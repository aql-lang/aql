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
	neturl "net/url"
	"strings"
	"time"
)

const (
	// mcpProtocolVersion is the MCP spec revision this server implements
	// and reports from `initialize`.
	mcpProtocolVersion = "2024-11-05"
	// mcpServerVersion is this vault MCP server's own version, bumped
	// when its tool surface or behavior changes. It is "2" since the
	// server now gates tools on capabilities rather than exposing every
	// provider-tagged alias.
	mcpServerVersion = "2"
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
	// Open the session once (one scrypt) and reuse it across tool calls;
	// nil falls back to a per-call authenticate.
	if sess, serr := authenticate(s, homeDir, nil, io.Discard, ""); serr == nil {
		if sess.Slot != nil && sess.Slot.ExpiresAt != "" {
			// A TEMPORARY password must NOT be cached (see runProxy): leave
			// srv.sess nil so each tool call re-authenticates and openSession
			// re-checks slotExpired, stopping service once it expires.
			fmt.Fprintf(stderr, "warning: MCP server is running under a TEMPORARY password (expires %s); it re-authenticates per call and stops serving once it expires\n", sess.Slot.ExpiresAt)
			sess.Close()
		} else {
			srv.sess = sess
			defer sess.Close()
		}
	}
	srv.serve(stdin, stdout)
	return 0
}

type mcpServer struct {
	homeDir string
	agent   string
	stderr  io.Writer
	client  *http.Client
	// sess is opened once at startup; see Proxy.sess.
	sess *Session
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
			"protocolVersion": mcpProtocolVersion,
			"serverInfo": map[string]string{
				"name":    "aql-vault",
				"version": mcpServerVersion,
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

// mcpToolName derives the tool name for an alias. MCP clients
// commonly restrict tool names to [A-Za-z0-9_-], so the namespace
// separator ':' is mangled to '_'. The mangling can collide with a
// literal alias (`proj:key` vs `proj_key`); forEachAgentTool resolves
// collisions deterministically.
func mcpToolName(alias string) string {
	return strings.ReplaceAll(alias, ":", "_") + "_request"
}

// forEachAgentTool walks the aliases this server's agent may broker —
// provider-tagged, with a live capability — in sorted-name order,
// yielding each with its mangled tool name. When two aliases mangle to
// the same tool name, the first (sorted) wins and later ones are
// reported via onSkip (nil ok) and not yielded. fn returns false to
// stop early. listTools and callTool both route through this walk so
// what tools/list advertises is exactly what tools/call resolves.
func (s *mcpServer) forEachAgentTool(st *Store, now time.Time, onSkip func(alias, tool, winner string), fn func(a Alias, tool string) bool) {
	seen := map[string]string{} // tool name -> winning alias
	for _, a := range st.SortedAliases() {
		if LookupProvider(a.Provider).BaseURL == "" {
			// Aliases without a provider preset cannot be brokered;
			// skip rather than expose a half-working tool.
			continue
		}
		if c, _ := st.FindActiveCapability(a.Name, s.agent, now); c == nil {
			// Only expose tools this agent holds a live capability for,
			// so tools/list never advertises something tools/call would
			// then refuse.
			continue
		}
		tool := mcpToolName(a.Name)
		if winner, taken := seen[tool]; taken {
			if onSkip != nil {
				onSkip(a.Name, tool, winner)
			}
			continue
		}
		seen[tool] = a.Name
		if !fn(a, tool) {
			return
		}
	}
}

// listTools returns one MCP tool per brokerable alias granted to the
// agent (see forEachAgentTool).
func (s *mcpServer) listTools() ([]map[string]any, error) {
	st, err := LoadStore(s.homeDir)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, errors.New("vault not initialized")
	}
	out := make([]map[string]any, 0, len(st.Aliases))
	s.forEachAgentTool(st, time.Now(), func(alias, tool, winner string) {
		fmt.Fprintf(s.stderr, "vault mcp: skipping tool %s for alias %s: name collides with alias %s\n", tool, alias, winner)
	}, func(a Alias, tool string) bool {
		prov := LookupProvider(a.Provider)
		out = append(out, map[string]any{
			"name":        tool,
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
		return true
	})
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
	// Map the tool name back to its alias by the same walk tools/list
	// used to generate it (':' mangled to '_', first-wins collisions).
	// Falling back to the literal trimmed name keeps the specific
	// unknown-alias / no-provider / no-capability errors reachable for
	// tools that were never advertised.
	now := time.Now()
	s.forEachAgentTool(st, now, nil, func(a Alias, tool string) bool {
		if tool == params.Name {
			alias = a.Name
			return false
		}
		return true
	})
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

	// Authorize against a capability granted to this agent for the
	// alias. The MCP server enforces the same policy as the proxy
	// (TTL, method/host allowlists, call and cost quotas, approval),
	// via the shared capabilityDenial; the difference is only how the
	// capability is resolved — by agent identity here, by bearer token
	// in the proxy.
	capab, _ := st.FindActiveCapability(alias, s.agent, now)
	if capab == nil {
		_ = appendAudit(s.homeDir, AuditEvent{
			Action: "mcp.request", Actor: "mcp", Agent: s.agent, Alias: alias,
			Method: method, Path: path, Outcome: "no-cap",
		})
		return fail(req, -32603, fmt.Sprintf(
			"no capability for alias %q granted to agent %q; run `aql vault grant --agent=%s %s`",
			alias, s.agent, s.agent, alias))
	}
	if reason := capabilityDenial(capab, method, mustHost(prov.BaseURL), now); reason != "" {
		_ = appendAudit(s.homeDir, AuditEvent{
			Action: "mcp.request", Actor: "mcp", Agent: s.agent, Alias: alias,
			Method: method, Path: path, Outcome: reason,
		})
		return fail(req, -32603, "capability denied: "+denialMessage(reason))
	}

	url := prov.BaseURL + path
	if q, ok := params.Arguments["query"].(map[string]any); ok && len(q) > 0 {
		// Encode through url.Values so keys/values containing &, #, =, or
		// spaces are escaped rather than spliced raw into the query
		// string (which would malform the URL or let a value smuggle
		// extra parameters).
		vals := neturl.Values{}
		for k, v := range q {
			vals.Set(k, fmt.Sprint(v))
		}
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		url += sep + vals.Encode()
	}

	sess := s.sess
	if sess == nil {
		var aerr error
		sess, aerr = authenticate(st, s.homeDir, nil, io.Discard, "")
		if aerr != nil {
			return fail(req, -32603, "keyring unavailable: "+aerr.Error())
		}
		defer sess.Close()
	}
	secret, err := sess.getValue(alias, valueNamespace(alias))
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

	// Debit the capability the same way the proxy does, so call and
	// cost quotas are enforced across MCP invocations too.
	cost := parseCostHeader(resp.Header.Get("X-AQL-Vault-Cost-Cents"))
	if err := recordCapabilityUse(s.homeDir, capab.ID, cost); err != nil {
		fmt.Fprintf(s.stderr, "vault mcp: persisting capability counters: %s\n", err)
	}

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
