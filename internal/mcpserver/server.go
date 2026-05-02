package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/allank/riffle/internal/indexer"
)

// Indexer is the interface the MCP server uses to serve queries.
// *indexer.Manager satisfies this interface.
type Indexer interface {
	Query(ctx context.Context, q string, top int, threshold float64) ([]indexer.QueryResult, error)
	Status() indexer.Status
}

// Server implements the MCP Streamable HTTP transport on a single TCP listener.
type Server struct {
	addr     string
	root     string
	idx      Indexer
	modeFunc func() string
	httpSrv  *http.Server
	listener net.Listener
}

// New creates a Server. addr may be "host:0" to let the OS pick a port;
// call Addr() after Start() to get the actual address.
func New(addr, root string, idx Indexer, modeFunc func() string) *Server {
	s := &Server{
		addr:     addr,
		root:     root,
		idx:      idx,
		modeFunc: modeFunc,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /mcp", s.handleMCP)
	s.httpSrv = &http.Server{Handler: mux}
	return s
}

// Start binds the listener and begins serving in a background goroutine.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	s.listener = ln
	go s.httpSrv.Serve(ln)
	return nil
}

// Addr returns the actual listen address (useful when addr was "host:0").
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

// --- HTTP handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"watching": s.root,
		"mode":     s.modeFunc(),
	})
}

// --- MCP JSON-RPC types ---

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Notifications have no id and expect no response body.
	if req.Method == "notifications/initialized" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var result any
	var rpcErr *jsonRPCError

	switch req.Method {
	case "initialize":
		result = s.handleInitialize()
	case "tools/list":
		result = s.handleToolsList()
	case "tools/call":
		result, rpcErr = s.handleToolsCall(r.Context(), req.Params)
	default:
		rpcErr = &jsonRPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   rpcErr,
	})
}

func (s *Server) handleInitialize() any {
	return map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "riffle", "version": "2.0"},
	}
}

func (s *Server) handleToolsList() any {
	return map[string]any{
		"tools": []any{
			map[string]any{
				"name":        "riffle_query",
				"description": "Find the most semantically relevant directories for a natural-language query.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"q":         map[string]any{"type": "string", "description": "Natural-language search query"},
						"top":       map[string]any{"type": "integer", "default": 5},
						"threshold": map[string]any{"type": "number", "default": 0.0, "description": "Minimum similarity score (0.0-1.0)"},
					},
					"required": []string{"q"},
				},
			},
			map[string]any{
				"name":        "riffle_status",
				"description": "Return health and statistics for the current index.",
				"inputSchema": map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type queryArgs struct {
	Q         string  `json:"q"`
	Top       int     `json:"top"`
	Threshold float64 `json:"threshold"`
}

func (s *Server) handleToolsCall(ctx context.Context, params json.RawMessage) (any, *jsonRPCError) {
	var p toolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &jsonRPCError{Code: -32602, Message: "invalid params"}
	}
	switch p.Name {
	case "riffle_query":
		return s.callQuery(ctx, p.Arguments)
	case "riffle_status":
		return s.callStatus()
	default:
		return nil, &jsonRPCError{Code: -32602, Message: fmt.Sprintf("unknown tool: %s", p.Name)}
	}
}

func (s *Server) callQuery(ctx context.Context, rawArgs json.RawMessage) (any, *jsonRPCError) {
	var args queryArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, &jsonRPCError{Code: -32602, Message: "invalid arguments"}
	}
	if args.Q == "" {
		return nil, &jsonRPCError{Code: -32602, Message: "q is required"}
	}
	if args.Top <= 0 {
		args.Top = 5
	}

	results, err := s.idx.Query(ctx, args.Q, args.Top, args.Threshold)
	if err != nil {
		return nil, &jsonRPCError{Code: -32603, Message: err.Error()}
	}

	text, _ := json.Marshal(results)
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(text)}},
	}, nil
}

func (s *Server) callStatus() (any, *jsonRPCError) {
	st := s.idx.Status()
	payload := map[string]any{
		"index": st.Index,
		"dirs":  st.Dirs,
		"size":  st.Size,
		"stale": st.Stale,
		"ext":   st.Ext,
		"model": st.Model,
		"built": st.Built,
		"mode":  s.modeFunc(),
	}
	text, _ := json.Marshal(payload)
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(text)}},
	}, nil
}
