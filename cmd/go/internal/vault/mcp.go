// MCP server mode for the local vault. Implements the subset of
// the Model Context Protocol an AI agent needs to discover and
// invoke vault-mediated HTTP calls:
//
//   - initialize
//   - tools/list
//   - tools/call
//   - notifications/initialized
//   - ping
//
// Each provider-tagged alias is exposed as one tool named
// "<alias>_request" with inputs {path, method, body, query} and
// outputs {status, headers, body}. The agent never sees the
// underlying credential — the server resolves the alias, looks
// up the secret in the keyring, and forwards through the same
// provider injection logic as `vault proxy`.

package vault

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// mcpRequest mirrors a JSON-RPC 2.0 request frame. ID is decoded
// as json.RawMessage so we can echo it back unchanged regardless
// of whether the client sent a string, number, or null.
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// runMCP implements `aql vault mcp`. It reads JSON-RPC frames
// from stdin (one per line; clients that use the Content-Length
// HTTP-style framing are not supported by this minimal server)
// and writes responses to stdout. Diagnostic output goes to
// stderr to keep the stdout protocol stream clean.
func runMCP(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", "mcp", "agent identity attributed to forwarded requests")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `aql vault unlock`")
		return 1
	}
	srv := &mcpServer{
		homeDir: homeDir,
		agent:   *agent,
		stderr:  stderr,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
	srv.serve(stdin, stdout)
	return 0
}

type mcpServer struct {
	homeDir string
	agent   string
	stderr  io.Writer
	client  *http.Client
}

// serve runs the line-delimited JSON-RPC loop until stdin closes.
// Each request is handled synchronously; a misbehaving handler
// cannot deadlock the loop because we always emit a response
// (or, for notifications, swallow silently per the spec).
func (s *mcpServer) serve(stdin io.Reader, stdout io.Writer) {
	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 1<<16), 1<<22)
	enc := json.NewEncoder(stdout)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req mcpRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(s.stderr, "vault mcp: bad request: %s\n", err)
			continue
		}
		resp := s.dispatch(&req)
		if resp == nil {
			continue // notification; no reply per JSON-RPC 2.0
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintf(s.stderr, "vault mcp: encoding response: %s\n", err)
			return
		}
	}
}

// dispatch routes one request to the right handler. Returns nil
// for notifications (no ID) so the loop can skip writing a reply.
func (s *mcpServer) dispatch(req *mcpRequest) *mcpResponse {
	isNotification := len(req.ID) == 0
	switch req.Method {
	case "initialize":
		return ok(req, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]string{
				"name":    "aql-vault",
				"version": "1",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		})
	case "ping":
		return ok(req, map[string]any{})
	case "notifications/initialized":
		// Client signal that handshake is complete; per spec we
		// return no response for notifications.
		return nil
	case "tools/list":
		tools, err := s.listTools()
		if err != nil {
			return fail(req, -32603, err.Error())
		}
		return ok(req, map[string]any{"tools": tools})
	case "tools/call":
		return s.callTool(req)
	default:
		if isNotification {
			return nil
		}
		return fail(req, -32601, "method not found: "+req.Method)
	}
}

// listTools returns one MCP tool per non-locked alias. Each tool
// is named "<alias>_request" and accepts a small input schema.
func (s *mcpServer) listTools() ([]map[string]any, error) {
	st, err := LoadStore(s.homeDir)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, errors.New("vault not initialized")
	}
	out := make([]map[string]any, 0, len(st.Aliases))
	for _, a := range st.SortedAliases() {
		prov := LookupProvider(a.Provider)
		if prov.BaseURL == "" {
			// Aliases without a provider preset cannot be brokered;
			// skip rather than expose a half-working tool.
			continue
		}
		out = append(out, map[string]any{
			"name":        a.Name + "_request",
			"description": fmt.Sprintf("Issue an HTTP request to %s via the %q vault alias. The real credential is not exposed.", prov.BaseURL, a.Name),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"method": map[string]any{"type": "string", "default": "GET"},
					"path":   map[string]any{"type": "string", "default": "/"},
					"body":   map[string]any{"type": "string"},
					"query":  map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				},
				"required": []string{"path"},
			},
		})
	}
	return out, nil
}

// callTool handles tools/call by looking up the alias indicated
// by the tool name suffix, applying provider injection, and
// returning the upstream response as MCP "content" entries. The
// response body is returned as text (MCP's typical surface for
// model-visible payloads); binary responses are base64-encoded
// upstream of this layer if needed.
func (s *mcpServer) callTool(req *mcpRequest) *mcpResponse {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return fail(req, -32602, "invalid params: "+err.Error())
	}
	alias := strings.TrimSuffix(params.Name, "_request")
	if alias == params.Name {
		return fail(req, -32602, "tool name must end with _request")
	}

	st, err := requireStore(s.homeDir)
	if err != nil {
		return fail(req, -32603, err.Error())
	}
	if st.Locked {
		return fail(req, -32603, "vault is locked")
	}
	a, _ := st.FindAlias(alias)
	if a == nil {
		return fail(req, -32602, "unknown alias: "+alias)
	}
	prov := LookupProvider(a.Provider)
	if prov.BaseURL == "" {
		return fail(req, -32603, "alias has no provider preset")
	}

	method := stringArg(params.Arguments, "method", "GET")
	path := stringArg(params.Arguments, "path", "/")
	body := stringArg(params.Arguments, "body", "")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := prov.BaseURL + path
	if q, ok := params.Arguments["query"].(map[string]any); ok && len(q) > 0 {
		sep := "?"
		for k, v := range q {
			url += sep + k + "=" + fmt.Sprint(v)
			sep = "&"
		}
	}

	kr, err := openKeyring(st, s.homeDir, nil, io.Discard, "")
	if err != nil {
		return fail(req, -32603, "keyring unavailable: "+err.Error())
	}
	secret, err := kr.Get(alias)
	if err != nil {
		return fail(req, -32603, "secret lookup failed")
	}

	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	httpReq, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fail(req, -32603, "building request: "+err.Error())
	}
	if err := prov.InjectAuth(httpReq, secret); err != nil {
		return fail(req, -32603, "credential injection failed")
	}
	resp, err := s.client.Do(httpReq)
	if err != nil {
		return fail(req, -32603, "upstream: "+err.Error())
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	_ = appendAudit(s.homeDir, AuditEvent{
		Action:  "mcp.request",
		Actor:   "mcp",
		Agent:   s.agent,
		Alias:   alias,
		Method:  method,
		Path:    path,
		Status:  resp.StatusCode,
		Outcome: "ok",
	})

	return ok(req, map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, string(respBody)),
			},
		},
		"isError": resp.StatusCode >= 400,
	})
}

func stringArg(args map[string]any, key, def string) string {
	if args == nil {
		return def
	}
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}

func ok(req *mcpRequest, result any) *mcpResponse {
	return &mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func fail(req *mcpRequest, code int, msg string) *mcpResponse {
	return &mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: code, Message: msg}}
}
