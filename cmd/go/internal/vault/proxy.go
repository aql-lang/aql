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
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// proxyProtocol is the credential-broker wire protocol version. It
	// is advertised on every response via headerProtocol; a client that
	// sends headerProtocol declaring a newer protocol than this broker
	// supports is refused, so a stale agent fails loudly instead of
	// silently misbehaving.
	proxyProtocol  = 1
	headerProtocol = "X-Boru-Vault-Protocol"
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
	// sess is the broker's authenticated session, opened once at startup
	// so a request does not re-derive the KEK (one scrypt) per call. Its
	// unsealed data keys live in memory for the broker's lifetime; nil
	// falls back to a per-request authenticate. Read-only after Serve
	// starts, so concurrent request goroutines share it safely.
	sess *Session
}

// NewProxy constructs a Proxy. defaultPass is forwarded to the
// file keyring when the store's backend is "file"; it is ignored
// for OS keychain backends.
func NewProxy(listen, homeDir, defaultPass string, stdout, stderr io.Writer) *Proxy {
	// Bound the time to receive response *headers*, not the whole
	// exchange: a dead or stalled upstream still fails fast, but a
	// long-lived response body — an SSE token stream, a large download —
	// is not guillotined mid-stream the way a client-wide Timeout would
	// (that cut every stream over 60s and, because io.Copy's error is
	// discarded, surfaced it to the caller as a clean EOF logged "ok").
	// The transport is built explicitly rather than cloning
	// http.DefaultTransport, whose *http.Transport type assertion would
	// panic if another component swapped the global RoundTripper (the
	// repo forbids panics outside init-time registration).
	return &Proxy{
		listen:      listen,
		homeDir:     homeDir,
		defaultPass: defaultPass,
		stdout:      stdout,
		stderr:      stderr,
		client:      newBrokerClient(60 * time.Second),
	}
}

// noRedirect stops a broker HTTP client from following upstream
// redirects. A credential broker must never resend an injected secret to
// a host the capability did not authorize, and net/http copies custom
// auth headers (x-api-key, a header:<name> preset) across a redirect
// even when it changes host — only Authorization is stripped cross-host.
// Returning ErrUseLastResponse hands the 3xx back to the caller
// unfollowed, so the secret only ever reaches the preset's own host.
func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// newBrokerClient builds the HTTP client both the proxy and the MCP
// server use to reach upstreams: a transport whose response-header
// timeout bounds time-to-first-byte (not the whole stream), env-proxy
// support matching the standard default, and the no-follow redirect
// policy above. Built explicitly so there is no panic-prone assertion on
// the global default transport.
func newBrokerClient(headerTimeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: headerTimeout,
	}
	return &http.Client{Transport: tr, CheckRedirect: noRedirect}
}

// runProxy implements `boru vault proxy`.
func runProxy(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault proxy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "127.0.0.1:8787", "address to listen on (loopback recommended)")
	allowPublic := fs.Bool("allow-public", false, "permit binding to a non-loopback address (exposes the credential broker to the network)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if !isLoopback(*listen) {
		if !*allowPublic {
			fmt.Fprintf(stderr, "error: %s is not a loopback address; refusing to expose the credential broker to the network. Re-run with --allow-public to override.\n", *listen)
			return 1
		}
		fmt.Fprintf(stderr, "warning: %s is not a loopback address; the proxy will accept connections from other hosts\n", *listen)
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `boru vault unlock`")
		return 1
	}

	p := NewProxy(*listen, homeDir, os.Getenv(EnvPassphrase), stdout, stderr)
	// Open the session once so each request reuses its unsealed data keys
	// instead of re-deriving the KEK per call. A failure here (e.g. no
	// passphrase yet) is non-fatal: the handler falls back to a
	// per-request authenticate.
	if sess, serr := authenticate(s, homeDir, nil, io.Discard, ""); serr == nil {
		if sess.Slot != nil && sess.Slot.ExpiresAt != "" {
			// A TEMPORARY password must NOT be cached: a frozen session would
			// keep serving past the advertised expiry. Leave p.sess nil so the
			// handler re-authenticates per request — openSession re-checks
			// slotExpired every time, so the broker stops serving at expiry.
			fmt.Fprintf(stderr, "warning: broker is running under a TEMPORARY password (expires %s); it re-authenticates per request and stops serving once it expires\n", sess.Slot.ExpiresAt)
			sess.Close()
		} else {
			p.sess = sess
			defer sess.Close()
			if sess.Slot != nil && sess.Scope == ScopeAdmin {
				fmt.Fprintln(stderr, "warning: broker is running under an ADMIN password; prefer a scoped read password (vault password add <name> --scope=read --namespaces=...)")
			}
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := p.Serve(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

// isLoopback reports whether host:port resolves to a loopback IP
// without performing DNS. Unknown shapes are treated as non-loopback so
// the warning fires by default — and so is an EMPTY host (":8200"),
// because net/http binds an empty host to every interface, which is
// exactly the exposure the guard exists to catch.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
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
	w.Header().Set(headerProtocol, strconv.Itoa(proxyProtocol))
	if v := r.Header.Get(headerProtocol); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > proxyProtocol {
			writeDenied(w, http.StatusBadRequest, fmt.Sprintf(
				"client speaks vault protocol %d but this broker supports up to %d; upgrade the broker", n, proxyProtocol))
			p.log(started, r, "", http.StatusBadRequest, "protocol-mismatch")
			return
		}
	}
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

	// Authenticate the bearer token against stored hashes, then confirm
	// it is bound to the alias in the request path. These two checks are
	// token-specific and stay here; the policy checks (state, method,
	// host, quotas, approval) are shared with the MCP server below.
	tok, _ := s.FindCapabilityByToken(token)
	if tok == nil {
		writeDenied(w, http.StatusUnauthorized, "unknown capability")
		p.log(started, r, alias, http.StatusUnauthorized, "no-cap")
		return
	}
	if tok.Alias != alias {
		writeDenied(w, http.StatusForbidden, "capability bound to a different alias")
		p.log(started, r, alias, http.StatusForbidden, "alias-mismatch")
		return
	}

	aliasMeta, _ := s.FindAlias(alias)
	if aliasMeta == nil {
		writeDenied(w, http.StatusNotFound, "alias not found")
		p.log(started, r, alias, http.StatusNotFound, "no-alias")
		return
	}
	// Per-alias source-IP allowlist: a key tagged with --ip-whitelist may
	// only be used from a listed client IP/CIDR. Checked here at the
	// broker (the host-side `vault get`/`exec` paths never see a client
	// IP); matters once the proxy is bound off-loopback.
	if len(aliasMeta.IPWhitelist) > 0 && !ipAllowed(clientIP(r.RemoteAddr), aliasMeta.IPWhitelist) {
		writeDenied(w, http.StatusForbidden, "client IP not in the alias ip-whitelist")
		p.log(started, r, alias, http.StatusForbidden, "ip-denied")
		return
	}
	provider := LookupProviderIn(s, aliasMeta.Provider)
	if provider.BaseURL == "" {
		writeDenied(w, http.StatusBadRequest,
			"alias has no provider preset; tag it with --provider on vault add, use a built-in preset, or define one with vault provider add")
		p.log(started, r, alias, http.StatusBadRequest, "no-provider")
		return
	}
	upstreamHost := mustHost(provider.BaseURL)

	if reason := capabilityDenial(tok, r.Method, upstreamHost, time.Now()); reason != "" {
		writeDenied(w, denialStatus(reason), denialMessage(reason))
		p.log(started, r, alias, denialStatus(reason), reason)
		return
	}

	sess := p.sess
	if sess == nil {
		var aerr error
		sess, aerr = authenticate(s, p.homeDir, nil, io.Discard, "")
		if aerr != nil {
			writeDenied(w, http.StatusServiceUnavailable, "vault unavailable; set BORU_VAULT_PASSPHRASE for file backend")
			p.log(started, r, alias, http.StatusServiceUnavailable, "no-keyring")
			return
		}
		defer sess.Close()
	}
	secret, err := sess.getValue(alias, valueNamespace(alias))
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
	costCents := parseCostHeader(resp.Header.Get("X-Boru-Vault-Cost-Cents"))
	p.recordUse(tok.ID, costCents)

	copyHeadersExceptHop(w.Header(), resp.Header)
	fw := flushingWriter(w)
	w.WriteHeader(resp.StatusCode)
	// Push the status line and headers to the client immediately, before
	// the first body chunk. An SSE upstream that opens the stream then
	// idles until its first event would otherwise leave the client
	// blocked awaiting headers, so the connection never appears
	// established.
	fw.flush()
	_, cerr := io.Copy(fw, resp.Body)
	outcome := "ok"
	if cerr != nil {
		// The stream broke after the status was already committed (upstream
		// reset, client hang-up, header-timeout mid-body). The response
		// can't be un-sent, but record the truncation honestly rather than
		// logging a clean "ok" that reads as a complete response.
		outcome = "stream-interrupted"
	}
	p.log(started, r, alias, resp.StatusCode, outcome)
}

// flushingWriter wraps w so every chunk copied from the upstream is
// flushed to the client as it arrives. Without this the response
// buffer would hold small writes until it fills or the handler
// returns, stalling exactly the traffic a credential broker for AI
// agents carries most: server-sent-event token streams. The returned
// wrapper is always usable — its flush is a no-op when w cannot flush —
// so callers never branch on flushability.
func flushingWriter(w http.ResponseWriter) *flushWriter {
	f, _ := w.(http.Flusher)
	return &flushWriter{w: w, f: f}
}

type flushWriter struct {
	w io.Writer
	f http.Flusher // nil when the underlying writer cannot flush
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if n > 0 {
		fw.flush()
	}
	return n, err
}

// flush pushes buffered bytes to the client, or does nothing when the
// underlying writer is not an http.Flusher.
func (fw *flushWriter) flush() {
	if fw.f != nil {
		fw.f.Flush()
	}
}

// recordUse increments the call counter and cost meter on the
// named capability and persists the store. Errors are swallowed
// (recorded to stderr) because a bookkeeping failure must not
// affect the in-flight response, which has already been authorized.
func (p *Proxy) recordUse(capID string, costCents int) {
	if err := recordCapabilityUse(p.homeDir, capID, costCents); err != nil {
		fmt.Fprintf(p.stderr, "vault proxy: persisting capability counters: %s\n", err)
	}
}

// capabilityDenial reports why c may not be used for a request with the
// given method against upstreamHost at now, as a short machine reason,
// or "" if the request is permitted. Quota counters are read, not
// mutated. Both the proxy and the MCP server gate on this so the two
// enforcement paths cannot drift. Pass upstreamHost "" to skip the host
// check (e.g. when the host is not yet resolved).
func capabilityDenial(c *Capability, method, upstreamHost string, now time.Time) string {
	switch {
	case c.Revoked:
		return "revoked"
	case !capabilityActive(c, now):
		return "expired"
	case len(c.Methods) > 0 && !contains(c.Methods, method):
		return "method-deny"
	case c.MaxCalls > 0 && c.UsedCalls >= c.MaxCalls:
		return "calls-exhausted"
	case c.MaxCostCents > 0 && c.UsedCostCents >= c.MaxCostCents:
		return "budget-exhausted"
	case c.RequireApproval:
		// Advisory: deny and audit so an operator can inspect and (out
		// of band) clear approval, grant a fresh capability, or proceed
		// another way.
		return "approval-required"
	case len(c.Hosts) > 0 && upstreamHost != "" && !contains(c.Hosts, upstreamHost):
		return "host-deny"
	}
	return ""
}

// denialStatus maps a capabilityDenial reason to the HTTP status the
// proxy returns for it.
func denialStatus(reason string) int {
	switch reason {
	case "calls-exhausted":
		return http.StatusTooManyRequests
	case "budget-exhausted":
		return http.StatusPaymentRequired
	default:
		return http.StatusForbidden
	}
}

// denialMessage maps a capabilityDenial reason to the human-readable
// body the proxy returns for it.
func denialMessage(reason string) string {
	switch reason {
	case "revoked":
		return "capability revoked"
	case "expired":
		return "capability expired"
	case "method-deny":
		return "method not permitted by capability"
	case "calls-exhausted":
		return "capability call quota exhausted"
	case "budget-exhausted":
		return "capability cost budget exhausted"
	case "approval-required":
		return "capability requires human approval (advisory; see audit log)"
	case "host-deny":
		return "upstream host not permitted by capability"
	default:
		return "capability denied"
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
