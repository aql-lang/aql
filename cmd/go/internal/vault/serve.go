// serve.go — `boru vault serve`: the vault wire protocol.
//
// A read-only HTTP API for secret provision, shaped after HashiCorp
// Vault's KV v2 surface so a client needs only an address and a token to
// fetch secrets — and so existing Vault client libraries (and the
// sekreto secret-provider library) can speak to a boru vault unchanged.
//
// Endpoints (all under /v1, JSON in HashiCorp response shapes):
//
//	GET  /v1/sys/health                   liveness: initialized/sealed
//	GET  /v1/auth/token/lookup-self       metadata for the presented token
//	GET  /v1/secret/data/<path>           read one secret (KV v2 shape)
//	LIST /v1/secret/metadata[/<ns>]       list readable keys (or GET ?list=true)
//
// A secret path is `<name>` (root namespace) or `<ns>/<name>` — the
// wire spelling of the stored alias `ns:name`. Clients authenticate
// with a capability bearer token (minted by `vault grant`) presented as
// the X-Vault-Token header (HashiCorp convention) or an Authorization
// Bearer credential. A capability bound to one alias reads exactly that
// alias; a namespace wildcard capability (`vault grant 'ns:*'` or
// `vault grant '*'` for root) reads every alias in its one namespace.
//
// The surface is deliberately read-only: provisioning hands secrets
// out; mutating the vault stays on the authenticated CLI. Every denial
// is a JSON {"errors":[...]} body that never carries a secret.
package vault

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// serveProtocol is the wire-protocol version, advertised on every
	// response via headerProtocol exactly like the proxy's: a client that
	// declares a newer protocol than this server speaks is refused loudly
	// instead of silently misbehaving.
	serveProtocol = 1
	// headerVaultToken is HashiCorp Vault's client-token header, accepted
	// so stock Vault clients work against `vault serve` unchanged.
	headerVaultToken = "X-Vault-Token"
	// serveWirePrefixData / serveWirePrefixList are the KV v2 route
	// prefixes (the fixed "secret" mount).
	serveWirePrefixData = "/v1/secret/data/"
	serveWirePrefixList = "/v1/secret/metadata"
)

// serveServer is the wire-protocol HTTP server. Like the Proxy it holds
// one broker session opened at startup (nil falls back to a per-request
// authenticate) that is read-only after Serve starts, so concurrent
// request goroutines share it safely.
type serveServer struct {
	listen  string
	homeDir string
	stdout  io.Writer
	stderr  io.Writer
	sess    *Session
}

// runServe implements `boru vault serve`.
func runServe(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "127.0.0.1:8200", "address to listen on (loopback recommended)")
	allowPublic := fs.Bool("allow-public", false, "permit binding to a non-loopback address (exposes secret provision to the network)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if !isLoopback(*listen) {
		if !*allowPublic {
			fmt.Fprintf(stderr, "error: %s is not a loopback address; refusing to expose secret provision to the network. Re-run with --allow-public to override.\n", *listen)
			return 1
		}
		fmt.Fprintf(stderr, "warning: %s is not a loopback address; the wire protocol will accept connections from other hosts\n", *listen)
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

	srv := &serveServer{listen: *listen, homeDir: homeDir, stdout: stdout, stderr: stderr}
	// Open the session once so each request reuses its unsealed data keys
	// instead of re-deriving the KEK per call; failure falls back to a
	// per-request authenticate (mirrors runProxy, including the rule that
	// a TEMPORARY password is never cached — re-authenticating per request
	// makes openSession re-check slotExpired, so service stops at expiry).
	if sess, serr := authenticate(s, homeDir, nil, io.Discard, ""); serr == nil {
		if sess.Slot != nil && sess.Slot.ExpiresAt != "" {
			fmt.Fprintf(stderr, "warning: serve is running under a TEMPORARY password (expires %s); it re-authenticates per request and stops serving once it expires\n", sess.Slot.ExpiresAt)
			sess.Close()
		} else {
			srv.sess = sess
			defer sess.Close()
			if sess.Slot != nil && sess.Scope == ScopeAdmin {
				fmt.Fprintln(stderr, "warning: serve is running under an ADMIN password; prefer a scoped read password (vault password add <name> --scope=read --namespaces=...)")
			}
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := srv.Serve(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

// Serve runs the wire-protocol server until ctx is cancelled, with the
// same graceful 5-second drain as the proxy.
func (v *serveServer) Serve(ctx context.Context) error {
	srv := &http.Server{
		Addr:              v.listen,
		Handler:           v,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	fmt.Fprintf(v.stdout, "vault wire protocol listening on http://%s (v%d)\n", v.listen, serveProtocol)
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

// ServeHTTP routes one wire-protocol request. Every error becomes a
// JSON {"errors":[...]} body with a secret-free reason.
func (v *serveServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	w.Header().Set(headerProtocol, strconv.Itoa(serveProtocol))
	// Every response either carries a secret or reveals vault state, and
	// the documented deployment puts a reverse proxy in front: a shared
	// cache must never store one caller's answer for the next.
	w.Header().Set("Cache-Control", "no-store")
	if h := r.Header.Get(headerProtocol); h != "" {
		if n, err := strconv.Atoi(h); err == nil && n > serveProtocol {
			writeWireError(w, http.StatusBadRequest, fmt.Sprintf(
				"client speaks vault wire protocol %d but this server supports up to %d; upgrade the server", n, serveProtocol))
			v.log(started, r, "", http.StatusBadRequest, "protocol-mismatch")
			return
		}
	}
	switch {
	case r.URL.Path == "/v1/sys/health":
		v.handleHealth(w, r, started)
	case r.URL.Path == "/v1/auth/token/lookup-self":
		v.handleLookupSelf(w, r, started)
	case strings.HasPrefix(r.URL.Path, serveWirePrefixData):
		v.handleRead(w, r, started)
	case r.URL.Path == serveWirePrefixList || strings.HasPrefix(r.URL.Path, serveWirePrefixList+"/"):
		v.handleList(w, r, started)
	default:
		writeWireError(w, http.StatusNotFound, "unsupported path")
		v.log(started, r, "", http.StatusNotFound, "bad-path")
	}
}

// handleHealth reports liveness without authentication, in HashiCorp's
// sys/health shape and status conventions: 200 initialized+unsealed,
// 503 sealed (a locked vault), 501 uninitialized.
func (v *serveServer) handleHealth(w http.ResponseWriter, r *http.Request, started time.Time) {
	if r.Method != http.MethodGet {
		writeWireError(w, http.StatusMethodNotAllowed, "unsupported method")
		v.log(started, r, "", http.StatusMethodNotAllowed, "bad-method")
		return
	}
	s, err := LoadStore(v.homeDir)
	if err != nil {
		writeWireError(w, http.StatusInternalServerError, "vault store unreadable")
		v.log(started, r, "", http.StatusInternalServerError, "bad-store")
		return
	}
	code := http.StatusOK
	initialized, sealed := true, false
	switch {
	case s == nil:
		code, initialized = http.StatusNotImplemented, false
	case s.Locked:
		code, sealed = http.StatusServiceUnavailable, true
	}
	writeWireJSON(w, code, map[string]any{
		"initialized": initialized,
		"sealed":      sealed,
		"standby":     false,
		"version":     fmt.Sprintf("boru-vault-wire/%d", serveProtocol),
	})
	// Access line only, no audit record: a service manager probing
	// liveness every few seconds would otherwise grow the audit log
	// forever with entries that record no secret-relevant act.
	v.logline(started, r, "", code, "health")
}

// authWire authenticates the request's client token into its capability
// and enforces the shared capability policy (revocation, expiry, method
// allowlist, quotas, approval). On any denial it writes the wire error
// and returns a nil store.
func (v *serveServer) authWire(w http.ResponseWriter, r *http.Request, started time.Time, alias string) (*Store, *Capability) {
	token := serveToken(r)
	if token == "" {
		writeWireError(w, http.StatusForbidden, "missing client token")
		v.log(started, r, alias, http.StatusForbidden, "no-token")
		return nil, nil
	}
	s, err := requireStore(v.homeDir)
	if err != nil {
		// The error text names the store path and the parse/IO detail —
		// operator information. This caller has not even authenticated
		// yet, so it gets the same fixed phrase the health endpoint uses.
		writeWireError(w, http.StatusServiceUnavailable, "vault store unreadable")
		v.log(started, r, alias, http.StatusServiceUnavailable, "no-store")
		return nil, nil
	}
	if s.Locked {
		writeWireError(w, http.StatusServiceUnavailable, "vault is sealed (locked)")
		v.log(started, r, alias, http.StatusServiceUnavailable, "locked")
		return nil, nil
	}
	tok, _ := s.FindCapabilityByToken(token)
	if tok == nil {
		writeWireError(w, http.StatusForbidden, "permission denied")
		v.log(started, r, alias, http.StatusForbidden, "no-cap")
		return nil, nil
	}
	if reason := capabilityDenial(tok, r.Method, "", time.Now()); reason != "" {
		writeWireError(w, denialStatus(reason), denialMessage(reason))
		v.log(started, r, alias, denialStatus(reason), reason)
		return nil, nil
	}
	return s, tok
}

// handleLookupSelf returns metadata about the presented token, the
// HashiCorp endpoint clients use to validate a token before first use.
// The token itself is never echoed; the capability's public ID plays
// the "accessor" role.
func (v *serveServer) handleLookupSelf(w http.ResponseWriter, r *http.Request, started time.Time) {
	if r.Method != http.MethodGet {
		writeWireError(w, http.StatusMethodNotAllowed, "unsupported method")
		v.log(started, r, "", http.StatusMethodNotAllowed, "bad-method")
		return
	}
	_, tok := v.authWire(w, r, started, "")
	if tok == nil {
		return
	}
	remaining := 0
	if tok.MaxCalls > 0 {
		remaining = tok.MaxCalls - tok.UsedCalls
	}
	v.writeWireData(w, r, started, tok.Alias, map[string]any{
		"accessor":     tok.ID,
		"display_name": tok.Agent,
		"policies":     []string{"default"},
		"meta": map[string]any{
			"alias":     tok.Alias,
			"namespace": tok.Namespace,
		},
		"expire_time": tok.ExpiresAt,
		"num_uses":    remaining,
	})
}

// handleRead serves one secret in the KV v2 read shape.
func (v *serveServer) handleRead(w http.ResponseWriter, r *http.Request, started time.Time) {
	if r.Method != http.MethodGet {
		writeWireError(w, http.StatusMethodNotAllowed, "unsupported method")
		v.log(started, r, "", http.StatusMethodNotAllowed, "bad-method")
		return
	}
	alias, ok := aliasFromWirePath(strings.TrimPrefix(r.URL.Path, serveWirePrefixData))
	if !ok {
		writeWireError(w, http.StatusBadRequest, "invalid secret path (use <name> or <namespace>/<name>)")
		v.log(started, r, "", http.StatusBadRequest, "bad-secret-path")
		return
	}
	s, tok := v.authWire(w, r, started, alias)
	if s == nil {
		return
	}
	if !capabilityCoversAlias(tok, alias) {
		writeWireError(w, http.StatusForbidden, "permission denied")
		v.log(started, r, alias, http.StatusForbidden, "alias-mismatch")
		return
	}
	a, _ := s.FindAlias(alias)
	if a == nil {
		// HashiCorp returns 404 with an empty errors list for a missing
		// KV path; matching it keeps stock clients' not-found detection
		// working (and tells an authorized caller nothing it could not
		// learn from a list).
		writeWireError(w, http.StatusNotFound)
		v.log(started, r, alias, http.StatusNotFound, "no-alias")
		return
	}
	if len(a.IPWhitelist) > 0 && !ipAllowed(clientIP(r.RemoteAddr), a.IPWhitelist) {
		writeWireError(w, http.StatusForbidden, "client IP not in the alias ip-whitelist")
		v.log(started, r, alias, http.StatusForbidden, "ip-denied")
		return
	}
	ns := valueNamespace(alias)
	sess := v.sess
	if sess == nil {
		var aerr error
		sess, aerr = authenticate(s, v.homeDir, nil, io.Discard, "")
		if aerr != nil {
			writeWireError(w, http.StatusServiceUnavailable, "vault unavailable; set BORU_VAULT_PASSPHRASE for file backend")
			v.log(started, r, alias, http.StatusServiceUnavailable, "no-keyring")
			return
		}
		defer sess.Close()
	}
	if err := requireScopeNS(sess, OpRead, ns); err != nil {
		writeWireError(w, http.StatusForbidden, "permission denied")
		v.log(started, r, alias, http.StatusForbidden, "scope-denied")
		return
	}
	secret, err := sess.getValue(alias, ns)
	if err != nil {
		writeWireError(w, http.StatusInternalServerError, "secret lookup failed")
		v.log(started, r, alias, http.StatusInternalServerError, "no-secret")
		return
	}
	// Debit the capability's call quota BEFORE the value is written, so a
	// crash mid-response cannot bypass MaxCalls (mirrors the proxy).
	v.recordUse(tok.ID)
	created := a.CreatedAt
	if a.UpdatedAt != "" {
		created = a.UpdatedAt
	}
	v.writeWireData(w, r, started, alias, map[string]any{
		"data": map[string]any{"value": secret},
		"metadata": map[string]any{
			"created_time":    created,
			"custom_metadata": map[string]any{"provider": a.Provider, "expires_at": a.ExpiresAt},
			"deletion_time":   "",
			"destroyed":       false,
			"version":         1,
		},
	})
}

// handleList enumerates the keys the presented token may read, in
// HashiCorp's LIST shape: at the root level, root-namespace names plus
// "<ns>/" folder entries; under /<ns>, that namespace's base names.
func (v *serveServer) handleList(w http.ResponseWriter, r *http.Request, started time.Time) {
	if r.Method != "LIST" && !(r.Method == http.MethodGet && r.URL.Query().Get("list") == "true") {
		writeWireError(w, http.StatusMethodNotAllowed, "unsupported method (use LIST, or GET with ?list=true)")
		v.log(started, r, "", http.StatusMethodNotAllowed, "bad-method")
		return
	}
	ns := strings.Trim(strings.TrimPrefix(r.URL.Path, serveWirePrefixList), "/")
	if ns != "" && !validNamespaceName(ns) {
		writeWireError(w, http.StatusBadRequest, "invalid list path (use /v1/secret/metadata or /v1/secret/metadata/<namespace>)")
		v.log(started, r, "", http.StatusBadRequest, "bad-list-path")
		return
	}
	s, tok := v.authWire(w, r, started, "")
	if s == nil {
		return
	}
	// Listing runs under the same session a read would, so every gate a
	// GET enforces also bounds what a LIST reveals: a broker password
	// scoped away from a namespace cannot enumerate its names, an alias
	// IP-whitelisted away from this client stays invisible, and a
	// temporary broker password past its expiry stops answering here too.
	sess := v.sess
	if sess == nil {
		var aerr error
		sess, aerr = authenticate(s, v.homeDir, nil, io.Discard, "")
		if aerr != nil {
			writeWireError(w, http.StatusServiceUnavailable, "vault unavailable; set BORU_VAULT_PASSPHRASE for file backend")
			v.log(started, r, "", http.StatusServiceUnavailable, "no-keyring")
			return
		}
		defer sess.Close()
	}
	keys := serveListKeys(s, tok, ns, sess, clientIP(r.RemoteAddr))
	if len(keys) == 0 {
		// Same convention as a missing secret: 404, empty errors.
		writeWireError(w, http.StatusNotFound)
		v.log(started, r, "", http.StatusNotFound, "no-keys")
		return
	}
	v.writeWireData(w, r, started, "", map[string]any{"keys": keys})
}

// recordUse debits the capability's call quota and persists the store.
// Errors are swallowed (recorded to stderr) because a bookkeeping
// failure must not affect the in-flight response, which has already
// been authorized (mirrors Proxy.recordUse).
func (v *serveServer) recordUse(capID string) {
	if err := recordCapabilityUse(v.homeDir, capID, 0); err != nil {
		fmt.Fprintf(v.stderr, "vault serve: persisting capability counters: %s\n", err)
	}
}

// serveListKeys returns the sorted wire-protocol list entries visible to
// capability c at level ns: readable root names and "<ns>/" folders when
// ns is "", or the readable base names inside namespace ns. "Readable"
// means readable in full: an alias the serving session's scope or the
// alias's own IP whitelist would refuse to GET is not listed either — a
// name a client could never read is not a name it gets to learn.
func serveListKeys(s *Store, c *Capability, ns string, sess *Session, client string) []string {
	seen := map[string]struct{}{}
	for i := range s.Aliases {
		a := &s.Aliases[i]
		if !capabilityCoversAlias(c, a.Name) {
			continue
		}
		if len(a.IPWhitelist) > 0 && !ipAllowed(client, a.IPWhitelist) {
			continue
		}
		if requireScopeNS(sess, OpRead, valueNamespace(a.Name)) != nil {
			continue
		}
		ans, base := splitAlias(a.Name)
		switch {
		case ns == "" && ans == "":
			seen[base] = struct{}{}
		case ns == "":
			seen[ans+"/"] = struct{}{}
		case ans == ns:
			seen[base] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// capabilityCoversAlias reports whether capability c authorizes reading
// alias over the wire protocol: c is bound to exactly that alias, or c
// is a namespace wildcard ("*" for root, "ns:*") whose one namespace is
// the alias's. validAlias rejects '*', so no stored alias can ever
// collide with a wildcard binding.
func capabilityCoversAlias(c *Capability, alias string) bool {
	if c.Alias == alias {
		return true
	}
	wns, base := splitAlias(c.Alias)
	return base == "*" && wns == valueNamespace(alias)
}

// aliasFromWirePath maps a KV wire path to a stored alias name:
// "name" (root namespace) or "ns/name" (exactly one namespace level).
// Anything deeper, empty, or outside the alias charset is rejected.
func aliasFromWirePath(p string) (string, bool) {
	segs := strings.Split(p, "/")
	switch len(segs) {
	case 1:
		if !validAliasSegment(segs[0]) {
			return "", false
		}
		return segs[0], true
	case 2:
		if !validAliasSegment(segs[0]) || !validAliasSegment(segs[1]) {
			return "", false
		}
		return segs[0] + ":" + segs[1], true
	default:
		return "", false
	}
}

// serveToken extracts the client token: the X-Vault-Token header
// (HashiCorp convention) first, else an Authorization Bearer credential.
func serveToken(r *http.Request) string {
	if t := strings.TrimSpace(r.Header.Get(headerVaultToken)); t != "" {
		return t
	}
	t, _ := extractToken(r.Header.Get("Authorization"))
	return t
}

// writeWireData wraps a successful payload in the HashiCorp response
// envelope (request_id + data) and logs the request as ok.
func (v *serveServer) writeWireData(w http.ResponseWriter, r *http.Request, started time.Time, alias string, data map[string]any) {
	reqID, err := randomID()
	if err != nil {
		writeWireError(w, http.StatusInternalServerError, "internal error")
		v.log(started, r, alias, http.StatusInternalServerError, "no-entropy")
		return
	}
	writeWireJSON(w, http.StatusOK, map[string]any{
		"request_id":     reqID,
		"lease_id":       "",
		"renewable":      false,
		"lease_duration": 0,
		"data":           data,
	})
	v.log(started, r, alias, http.StatusOK, "ok")
}

// writeWireError emits a HashiCorp-shaped {"errors":[...]} body. No
// messages means an empty (but present) list — the 404 convention.
func writeWireError(w http.ResponseWriter, code int, msgs ...string) {
	if msgs == nil {
		msgs = []string{}
	}
	writeWireJSON(w, code, map[string]any{"errors": msgs})
}

// writeWireJSON marshals and writes one JSON response body.
func writeWireJSON(w http.ResponseWriter, code int, body any) {
	data, err := jsonMarshal(body)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "encoding error\n")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(append(data, '\n'))
}

// logline writes one redacted access line. The client token and the
// secret value are never written.
func (v *serveServer) logline(started time.Time, r *http.Request, alias string, status int, tag string) {
	if v.stdout != nil {
		fmt.Fprintf(v.stdout, "%s %s %s alias=%s status=%d outcome=%s dur=%s\n",
			time.Now().UTC().Format(time.RFC3339), r.Method, r.URL.Path,
			alias, status, tag, time.Since(started).Truncate(time.Millisecond))
	}
}

// log writes the access line and a structured audit event — every
// outcome except the unaudited health probe goes through here.
func (v *serveServer) log(started time.Time, r *http.Request, alias string, status int, tag string) {
	v.logline(started, r, alias, status, tag)
	_ = appendAudit(v.homeDir, AuditEvent{
		Action:  "serve.request",
		Actor:   "serve",
		Alias:   alias,
		Method:  r.Method,
		Path:    r.URL.Path,
		Status:  status,
		Outcome: tag,
	})
}
