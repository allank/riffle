package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/allank/riffle/internal/indexer"
	"github.com/allank/riffle/internal/mcpserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeIndexer struct {
	results  []indexer.QueryResult
	status   indexer.Status
	queryErr error
}

func (f *fakeIndexer) Query(_ context.Context, _ string, _ int, _ float64) ([]indexer.QueryResult, error) {
	return f.results, f.queryErr
}

func (f *fakeIndexer) Status() indexer.Status {
	return f.status
}

func newTestServer(t *testing.T, idx mcpserver.Indexer, mode string) *mcpserver.Server {
	t.Helper()
	srv := mcpserver.New("127.0.0.1:0", "/vault", idx, func() string { return mode })
	require.NoError(t, srv.Start())
	t.Cleanup(func() { srv.Shutdown(context.Background()) })
	return srv
}

func postMCP(t *testing.T, addr, body string) map[string]any {
	t.Helper()
	resp, err := http.Post("http://"+addr+"/mcp", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(t, &fakeIndexer{}, "events")

	resp, err := http.Get("http://" + srv.Addr() + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, true, body["ok"])
	assert.Equal(t, "/vault", body["watching"])
	assert.Equal(t, "events", body["mode"])
}

func TestHealthPollingMode(t *testing.T) {
	srv := newTestServer(t, &fakeIndexer{}, "polling")

	resp, err := http.Get("http://" + srv.Addr() + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "polling", body["mode"])
}

func TestMCPInitialize(t *testing.T) {
	srv := newTestServer(t, &fakeIndexer{}, "events")
	result := postMCP(t, srv.Addr(), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`)

	assert.Equal(t, "2.0", result["jsonrpc"])
	assert.Equal(t, float64(1), result["id"])
	rpcResult := result["result"].(map[string]any)
	assert.Equal(t, "2024-11-05", rpcResult["protocolVersion"])
	serverInfo := rpcResult["serverInfo"].(map[string]any)
	assert.Equal(t, "riffle", serverInfo["name"])
}

func TestMCPNotificationsInitialized(t *testing.T) {
	srv := newTestServer(t, &fakeIndexer{}, "events")

	resp, err := http.Post("http://"+srv.Addr()+"/mcp", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)
}

func TestMCPToolsList(t *testing.T) {
	srv := newTestServer(t, &fakeIndexer{}, "events")
	result := postMCP(t, srv.Addr(), `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)

	rpcResult := result["result"].(map[string]any)
	tools := rpcResult["tools"].([]any)
	assert.Len(t, tools, 2)

	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.(map[string]any)["name"].(string)
	}
	assert.ElementsMatch(t, []string{"riffle_query", "riffle_status"}, names)
}

func TestMCPQueryTool(t *testing.T) {
	fi := &fakeIndexer{
		results: []indexer.QueryResult{
			{Path: "security/oauth2", Score: 0.91},
			{Path: "projects/auth", Score: 0.87},
		},
	}
	srv := newTestServer(t, fi, "events")

	result := postMCP(t, srv.Addr(), `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"riffle_query","arguments":{"q":"OAuth token","top":5}}}`)

	assert.Nil(t, result["error"])
	rpcResult := result["result"].(map[string]any)
	content := rpcResult["content"].([]any)
	require.Len(t, content, 1)
	item := content[0].(map[string]any)
	assert.Equal(t, "text", item["type"])

	var queryResults []indexer.QueryResult
	require.NoError(t, json.Unmarshal([]byte(item["text"].(string)), &queryResults))
	require.Len(t, queryResults, 2)
	assert.Equal(t, "security/oauth2", queryResults[0].Path)
	assert.InDelta(t, 0.91, queryResults[0].Score, 0.001)
}

func TestMCPQueryMissingQ(t *testing.T) {
	srv := newTestServer(t, &fakeIndexer{}, "events")
	result := postMCP(t, srv.Addr(), `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"riffle_query","arguments":{}}}`)

	assert.NotNil(t, result["error"])
	rpcErr := result["error"].(map[string]any)
	assert.Contains(t, rpcErr["message"], "q is required")
}

func TestMCPStatusTool(t *testing.T) {
	fi := &fakeIndexer{
		status: indexer.Status{
			Index: "/vault/.riffle/index.bin",
			Dirs:  42,
			Size:  "1.2MB",
			Ext:   []string{".md"},
			Model: "all-MiniLM-L6-v2",
			Built: "2026-05-02T10:00:00Z",
		},
	}
	srv := newTestServer(t, fi, "polling")

	result := postMCP(t, srv.Addr(), `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"riffle_status","arguments":{}}}`)

	assert.Nil(t, result["error"])
	rpcResult := result["result"].(map[string]any)
	content := rpcResult["content"].([]any)
	item := content[0].(map[string]any)

	var st map[string]any
	require.NoError(t, json.Unmarshal([]byte(item["text"].(string)), &st))
	assert.Equal(t, float64(42), st["dirs"])
	assert.Equal(t, "polling", st["mode"]) // mode comes from modeFunc, not Status struct
}

func TestMCPUnknownTool(t *testing.T) {
	srv := newTestServer(t, &fakeIndexer{}, "events")
	result := postMCP(t, srv.Addr(), `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"no_such_tool","arguments":{}}}`)

	assert.NotNil(t, result["error"])
}

func TestMCPUnknownMethod(t *testing.T) {
	srv := newTestServer(t, &fakeIndexer{}, "events")
	result := postMCP(t, srv.Addr(), `{"jsonrpc":"2.0","id":7,"method":"no/such/method","params":{}}`)

	assert.NotNil(t, result["error"])
	rpcErr := result["error"].(map[string]any)
	assert.Equal(t, float64(-32601), rpcErr["code"])
}

func TestHealthWrongMethod(t *testing.T) {
	srv := newTestServer(t, &fakeIndexer{}, "events")

	resp, err := http.Post("http://"+srv.Addr()+"/health", "application/json", bytes.NewReader(nil))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 405, resp.StatusCode)
}
