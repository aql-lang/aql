package vault

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Proxy is the local credential broker. It listens on a loopback
// HTTP address, authorizes incoming requests by capability token,
// rewrites them with the real secret value from the keyring, and
// forwards them to the upstream provider.
//
// Request shape:
//
//	<method> http://<listen>/<alias>/<path> ...
//	Authorization: Bearer <capability-id>
//
// On success the upstream provider's response is streamed back to
// the caller. On any policy denial the proxy returns a 4xx with a
// short reason; the real secret is never written to the response
// or to logs.
type Proxy struct {
	listen      string
	homeDir     string
	defaultPass string
	stdout      io.Writer
	stderr      io.Writer
	client      *http.Client
}

// NewProxy constructs a Proxy. defaultPass is forwarded to the
// file keyring when the store's backend is "file"; it is ignored
// for OS keychain backends.
func NewProxy(listen, homeDir, defaultPass string, stdout, stderr io.Writer) *Proxy {
	return &Proxy{
		listen:      listen,
		homeDir:     homeDir,
		defaultPass: defaultPass,
		stdout:      stdout,
		stderr:      stderr,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// runProxy implements `aql vault proxy`.
func runProxy(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault proxy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "127.0.0.1:8787", "address to listen on (loopback recommended)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if !isLoopback(*listen) {
		fmt.Fprintf(stderr, "warning: %s is not a loopback address; the proxy will accept connections from other hosts\n", *listen)
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

	p := NewProxy(*listen, homeDir, os.Getenv(EnvPassphrase), stdout, stderr)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := p.Serve(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

// isLoopback reports whether host:port resolves to a loopback IP
// without performing DNS. Unknown shapes are treated as
// non-loopback so the warning fires by default.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Serve runs the proxy until ctx is cancelled. The HTTP server is
// shut down gracefully with a 5-second drain on cancellation.
func (p *Proxy) Serve(ctx context.Context) error {
	srv := &http.Server{
		Addr:              p.listen,
		Handler:           p,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	fmt.Fprintf(p.stdout, "vault proxy listening on http://%s\n", p.listen)
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	}
}

// ServeHTTP is the single request handler. Errors are turned into
// 4xx responses with a short, secret-free body.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	alias, upstreamPath, ok := splitAliasPath(r.URL.Path)
	if !ok {
		writeDenied(w, http.StatusBadRequest, "path must be /<alias>/<upstream-path>")
		p.log(started, r, alias, http.StatusBadRequest, "bad-path")
		return
	}

	token, ok := extractToken(r.Header.Get("Authorization"))
	if !ok {
		writeDenied(w, http.StatusUnauthorized, "missing Authorization: Bearer <capability-id>")
		p.log(started, r, alias, http.StatusUnauthorized, "no-token")
		return
	}

	s, err := requireStore(p.homeDir)
	if err != nil {
		writeDenied(w, http.StatusServiceUnavailable, err.Error())
		p.log(started, r, alias, http.StatusServiceUnavailable, "no-store")
		return
	}
	if s.Locked {
		writeDenied(w, http.StatusServiceUnavailable, "vault is locked")
		p.log(started, r, alias, http.StatusServiceUnavailable, "locked")
		return
	}

	tok, _ := s.FindCapability(token)
	if tok == nil {
		writeDenied(w, http.StatusUnauthorized, "unknown capability")
		p.log(started, r, alias, http.StatusUnauthorized, "no-cap")
		return
	}
	if tok.Revoked {
		writeDenied(w, http.StatusForbidden, "capability revoked")
		p.log(started, r, alias, http.StatusForbidden, "revoked")
		return
	}
	if !capabilityActive(tok, time.Now()) {
		writeDenied(w, http.StatusForbidden, "capability expired")
		p.log(started, r, alias, http.StatusForbidden, "expired")
		return
	}
	if tok.Alias != alias {
		writeDenied(w, http.StatusForbidden, "capability bound to a different alias")
		p.log(started, r, alias, http.StatusForbidden, "alias-mismatch")
		return
	}
	if len(tok.Methods) > 0 && !contains(tok.Methods, r.Method) {
		writeDenied(w, http.StatusForbidden, "method not permitted by capability")
		p.log(started, r, alias, http.StatusForbidden, "method-deny")
		return
	}
	if tok.MaxCalls > 0 && tok.UsedCalls >= tok.MaxCalls {
		writeDenied(w, http.StatusTooManyRequests, "capability call quota exhausted")
		p.log(started, r, alias, http.StatusTooManyRequests, "calls-exhausted")
		return
	}
	if tok.MaxCostCents > 0 && tok.UsedCostCents >= tok.MaxCostCents {
		writeDenied(w, http.StatusPaymentRequired, "capability cost budget exhausted")
		p.log(started, r, alias, http.StatusPaymentRequired, "budget-exhausted")
		return
	}
	if tok.RequireApproval {
		// Approval flow is currently advisory: the proxy refuses
		// the request and emits an audit event so an operator can
		// inspect and (out of band) flip RequireApproval off, grant
		// a new capability, or proceed via a different channel. A
		// future revision could park requests in a queue with an
		// interactive `vault approve <id>` command.
		writeDenied(w, http.StatusForbidden, "capability requires human approval (advisory; see audit log)")
		p.log(started, r, alias, http.StatusForbidden, "approval-required")
		return
	}

	aliasMeta, _ := s.FindAlias(alias)
	if aliasMeta == nil {
		writeDenied(w, http.StatusNotFound, "alias not found")
		p.log(started, r, alias, http.StatusNotFound, "no-alias")
		return
	}
	provider := LookupProvider(aliasMeta.Provider)
	if provider.BaseURL == "" {
		writeDenied(w, http.StatusBadRequest,
			"alias has no provider preset; tag it with --provider on vault add, or use a built-in preset")
		p.log(started, r, alias, http.StatusBadRequest, "no-provider")
		return
	}
	upstreamHost := mustHost(provider.BaseURL)
	if len(tok.Hosts) > 0 && !contains(tok.Hosts, upstreamHost) {
		writeDenied(w, http.StatusForbidden, "upstream host not permitted by capability")
		p.log(started, r, alias, http.StatusForbidden, "host-deny")
		return
	}

	kr, err := openKeyring(s, p.homeDir, nil, io.Discard, "")
	if err != nil {
		writeDenied(w, http.StatusServiceUnavailable, "vault unavailable; set AQL_VAULT_PASSPHRASE for file backend")
		p.log(started, r, alias, http.StatusServiceUnavailable, "no-keyring")
		return
	}
	secret, err := kr.Get(alias)
	if err != nil {
		writeDenied(w, http.StatusInternalServerError, "secret lookup failed")
		p.log(started, r, alias, http.StatusInternalServerError, "no-secret")
		return
	}

	upstreamURL := provider.BaseURL + upstreamPath
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}
	out, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, r.Body)
	if err != nil {
		writeDenied(w, http.StatusBadGateway, "building upstream request failed")
		p.log(started, r, alias, http.StatusBadGateway, "build-fail")
		return
	}
	copyHeadersExceptHop(out.Header, r.Header)
	out.Header.Del("Authorization") // capability token must not leak upstream
	if err := provider.InjectAuth(out, secret); err != nil {
		writeDenied(w, http.StatusInternalServerError, "credential injection failed")
		p.log(started, r, alias, http.StatusInternalServerError, "inject-fail")
		return
	}

	resp, err := p.client.Do(out)
	if err != nil {
		writeDenied(w, http.StatusBadGateway, "upstream request failed")
		p.log(started, r, alias, http.StatusBadGateway, "upstream-fail")
		return
	}
	defer resp.Body.Close()
	// Persist the call against the capability *before* streaming
	// the body so a crash mid-stream cannot cause the quota to be
	// silently bypassed.
	costCents := parseCostHeader(resp.Header.Get("X-AQL-Vault-Cost-Cents"))
	p.recordUse(tok.ID, costCents)

	copyHeadersExceptHop(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	p.log(started, r, alias, resp.StatusCode, "ok")
}

// recordUse increments the call counter and cost meter on the
// named capability and persists the store. Errors are swallowed
// (recorded to stderr) because a bookkeeping failure must not
// affect the in-flight response, which has already been authorized.
func (p *Proxy) recordUse(capID string, costCents int) {
	s, err := LoadStore(p.homeDir)
	if err != nil || s == nil {
		return
	}
	c, idx := s.FindCapability(capID)
	if c == nil {
		return
	}
	s.Capabilities[idx].UsedCalls++
	s.Capabilities[idx].UsedCostCents += costCents
	if err := SaveStore(p.homeDir, s); err != nil {
		fmt.Fprintf(p.stderr, "vault proxy: persisting capability counters: %s\n", err)
	}
}

// parseCostHeader returns the integer cost in cents reported by
// the upstream, or 0 when the header is missing or malformed.
func parseCostHeader(v string) int {
	if v == "" {
		return 0
	}
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// splitAliasPath parses /<alias>/<rest> into ("alias", "/rest").
// A bare /<alias> (no trailing slash, no rest) is treated as
// alias + path "/".
func splitAliasPath(p string) (alias, rest string, ok bool) {
	if p == "" || p[0] != '/' {
		return "", "", false
	}
	p = p[1:]
	slash := strings.IndexByte(p, '/')
	if slash < 0 {
		if p == "" {
			return "", "", false
		}
		return p, "/", true
	}
	alias = p[:slash]
	if alias == "" {
		return "", "", false
	}
	return alias, p[slash:], true
}

// extractToken pulls the capability token from an HTTP
// Authorization header. Only the Bearer scheme is accepted.
func extractToken(h string) (string, bool) {
	if h == "" {
		return "", false
	}
	const bearer = "Bearer "
	if len(h) <= len(bearer) || !strings.EqualFold(h[:len(bearer)], bearer) {
		return "", false
	}
	return strings.TrimSpace(h[len(bearer):]), true
}

// capabilityActive reports whether c is neither revoked nor past
// its ExpiresAt timestamp.
func capabilityActive(c *Capability, now time.Time) bool {
	if c.Revoked {
		return false
	}
	if c.ExpiresAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, c.ExpiresAt)
	if err != nil {
		return true
	}
	return now.Before(t)
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func mustHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// hopByHopHeaders are stripped from both directions per RFC 7230.
var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Proxy-Connection":    {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

func copyHeadersExceptHop(dst, src http.Header) {
	for k, vs := range src {
		if _, hop := hopByHopHeaders[http.CanonicalHeaderKey(k)]; hop {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func writeDenied(w http.ResponseWriter, code int, reason string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, "vault proxy denied: "+reason+"\n")
}

// log writes a single redacted access line and appends a
// structured event to the audit log. The Authorization header
// (capability token) and the upstream secret are never written;
// only metadata about the request shape and outcome.
func (p *Proxy) log(started time.Time, r *http.Request, alias string, status int, tag string) {
	if p.stdout != nil {
		fmt.Fprintf(p.stdout, "%s %s %s alias=%s status=%d outcome=%s dur=%s\n",
			time.Now().UTC().Format(time.RFC3339), r.Method, r.URL.Path,
			alias, status, tag, time.Since(started).Truncate(time.Millisecond))
	}
	_ = appendAudit(p.homeDir, AuditEvent{
		Action:  "proxy.request",
		Actor:   "proxy",
		Alias:   alias,
		Method:  r.Method,
		Path:    r.URL.Path,
		Status:  status,
		Outcome: tag,
	})
}
