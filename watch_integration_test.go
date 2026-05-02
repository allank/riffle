//go:build integration

package main_test

import (
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

const watchTestAddr = "127.0.0.1:17424"

func waitForHealth(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return
		}
		if resp != nil {
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
	return st["built"].(string)
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

	// Start riffle watch in background.
	watchCmd := exec.CommandContext(ctx, bin, "watch", vault, "--listen", watchTestAddr)
	watchCmd.Env = env
	var watchOut bytes.Buffer
	watchCmd.Stdout = &watchOut
	watchCmd.Stderr = &watchOut
	require.NoError(t, watchCmd.Start())
	defer watchCmd.Process.Signal(os.Interrupt)

	// Wait for the server to be ready.
	waitForHealth(t, watchTestAddr, 30*time.Second)

	// 1. /health returns ok=true, mode=events.
	resp, err := http.Get("http://" + watchTestAddr + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	var health map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
	assert.Equal(t, true, health["ok"])
	assert.Equal(t, vault, health["watching"])
	assert.Equal(t, "events", health["mode"])

	// 2. riffle_status via MCP includes mode and dir count.
	statusResult := mcpCall(t, watchTestAddr, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"riffle_status","arguments":{}}}`)
	assert.Nil(t, statusResult["error"])
	rpcResult := statusResult["result"].(map[string]any)
	content := rpcResult["content"].([]any)
	item := content[0].(map[string]any)
	var st map[string]any
	require.NoError(t, json.Unmarshal([]byte(item["text"].(string)), &st))
	assert.Equal(t, "events", st["mode"])
	assert.Greater(t, st["dirs"].(float64), float64(0))

	// 3. riffle_query returns results.
	queryResult := mcpCall(t, watchTestAddr, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"riffle_query","arguments":{"q":"OAuth token authentication","top":5}}}`)
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
	builtBefore := getStatusBuiltTime(t, watchTestAddr)

	require.NoError(t, os.WriteFile(
		filepath.Join(vault, "security/oauth2/notes.md"),
		[]byte(fmt.Sprintf("Updated OAuth content at %s", time.Now())),
		0644,
	))

	// Wait up to 10 seconds for re-index to complete (watcher debounce + index time).
	deadline := time.Now().Add(10 * time.Second)
	reindexed := false
	for time.Now().Before(deadline) {
		builtAfter := getStatusBuiltTime(t, watchTestAddr)
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
