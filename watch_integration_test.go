//go:build integration

package main_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waitForHealth(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/health")
		if err == nil && resp.StatusCode == 200 {
			var h map[string]any
			if json.NewDecoder(resp.Body).Decode(&h) == nil && h["ok"] == true {
				resp.Body.Close()
				return
			}
			resp.Body.Close()
		} else if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become healthy within %s", addr, timeout)
}

func mcpCall(t *testing.T, addr, body string) map[string]any {
	t.Helper()
	resp, err := http.Post("http://"+addr+"/mcp", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result
}

func getStatusBuiltTime(t *testing.T, addr string) string {
	t.Helper()
	result := mcpCall(t, addr, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"riffle_status","arguments":{}}}`)
	rpcResult, ok := result["result"].(map[string]any)
	require.True(t, ok, "expected result field")
	content := rpcResult["content"].([]any)
	item := content[0].(map[string]any)
	var st map[string]any
	require.NoError(t, json.Unmarshal([]byte(item["text"].(string)), &st))
	built, ok := st["built"].(string)
	require.True(t, ok, "expected built field to be a string")
	return built
}

func TestWatchEndToEnd(t *testing.T) {
	skipIfNoModel(t)
	bin := buildBinary(t)
	vault := makeTestVault(t)

	env := append(os.Environ(),
		"RIFFLE_MODEL_PATH="+os.Getenv("RIFFLE_MODEL_PATH"),
		"RIFFLE_TOKENIZER_PATH="+os.Getenv("RIFFLE_TOKENIZER_PATH"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Use :0 so the OS picks a free port, avoiding conflicts in parallel runs.
	watchCmd := exec.CommandContext(ctx, bin, "watch", vault, "--listen", "127.0.0.1:0")
	watchCmd.Env = env

	var watchOut bytes.Buffer
	stdoutPipe, err := watchCmd.StdoutPipe()
	require.NoError(t, err)
	watchCmd.Stderr = &watchOut
	require.NoError(t, watchCmd.Start())
	defer watchCmd.Process.Signal(os.Interrupt)

	// Drain stdout, buffer it, and extract the listen address from the startup log line.
	// Startup format: watching path=<path> listen=<addr> mode=<mode>
	addrCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			watchOut.WriteString(line + "\n")
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "listen=") {
					select {
					case addrCh <- strings.TrimPrefix(field, "listen="):
					default:
					}
				}
			}
		}
	}()

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for riffle watch startup log")
	}

	// Wait for the server to be fully ready.
	waitForHealth(t, addr, 10*time.Second)

	// 1. /health returns ok=true, mode=events.
	resp, err := http.Get("http://" + addr + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	var health map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
	assert.Equal(t, true, health["ok"])
	assert.Equal(t, vault, health["watching"])
	assert.Equal(t, "events", health["mode"])

	// 2. riffle_status via MCP includes mode and dir count.
	statusResult := mcpCall(t, addr, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"riffle_status","arguments":{}}}`)
	assert.Nil(t, statusResult["error"])
	rpcResult := statusResult["result"].(map[string]any)
	content := rpcResult["content"].([]any)
	item := content[0].(map[string]any)
	var st map[string]any
	require.NoError(t, json.Unmarshal([]byte(item["text"].(string)), &st))
	assert.Equal(t, "events", st["mode"])
	assert.Greater(t, st["dirs"].(float64), float64(0))

	// 3. riffle_query returns results.
	queryResult := mcpCall(t, addr, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"riffle_query","arguments":{"q":"OAuth token authentication","top":5}}}`)
	assert.Nil(t, queryResult["error"])
	qContent := queryResult["result"].(map[string]any)["content"].([]any)
	qItem := qContent[0].(map[string]any)
	var queryResults []map[string]any
	require.NoError(t, json.Unmarshal([]byte(qItem["text"].(string)), &queryResults))
	assert.NotEmpty(t, queryResults)
	found := false
	for _, r := range queryResults {
		if strings.Contains(r["path"].(string), "oauth2") {
			found = true
		}
	}
	assert.True(t, found, "oauth2 directory should appear in OAuth query results, got: %v", queryResults)

	// 4. Record current built time, then modify a file and wait for re-index.
	builtBefore := getStatusBuiltTime(t, addr)

	require.NoError(t, os.WriteFile(
		filepath.Join(vault, "security/oauth2/notes.md"),
		[]byte(fmt.Sprintf("Updated OAuth content at %s", time.Now())),
		0644,
	))

	// Wait up to 10 seconds for re-index to complete (watcher debounce + index time).
	deadline := time.Now().Add(10 * time.Second)
	reindexed := false
	for time.Now().Before(deadline) {
		builtAfter := getStatusBuiltTime(t, addr)
		if builtAfter != builtBefore {
			reindexed = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assert.True(t, reindexed, "index should have been updated after file modification; output: %s", watchOut.String())

	// 5. Graceful shutdown via interrupt — process should exit cleanly.
	require.NoError(t, watchCmd.Process.Signal(os.Interrupt))
	done := make(chan error, 1)
	go func() { done <- watchCmd.Wait() }()
	select {
	case err := <-done:
		// On Unix, interrupt exits non-zero (signal), which is expected.
		_ = err
	case <-time.After(5 * time.Second):
		t.Fatal("riffle watch did not shut down within 5 seconds after interrupt")
	}
}
