package native

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/policy"
)

// checkFetchPolicy consults the registry's host policy (if any)
// before fetch issues an outbound request. The check sequence runs
// global.network → network install → network.connect{host, port};
// any denial returns the policy error so no HTTP request is built.
//
// When r is nil or no policy is installed the function is a no-op
// (the historical "no permissions configured" default).
//
// Errors are returned in *policy.Denied shape when the policy
// refuses; URL-parse failures are returned as ordinary errors so
// callers can distinguish "bad URL" from "policy denied".
func checkFetchPolicy(r *Registry, urlStr string) error {
	if r == nil {
		return nil
	}
	pol := HostPolicy(r)
	if pol == nil {
		return nil
	}
	// Resolve host/port from the URL before invoking the rule check
	// so where-predicates can match on host: and port: fields.
	host, port := hostPortFromURL(urlStr)
	args := policy.Args{
		"url":  urlStr,
		"host": host,
		"port": port,
	}
	// Per design: the network scope's "connect" op is what fetch
	// performs. global.network is consulted by the wrapper sequence
	// via Check; install:false on the network scope produces
	// capability_not_installed.
	if !pol.Installed("network") {
		return &policy.Denied{
			Code:    policy.CodeCapabilityNotInstalled,
			Scope:   "network",
			Op:      "connect",
			Profile: pol.Name(),
			Blame:   "network.install=false",
			Args:    args,
		}
	}
	return pol.Check("network", "connect", args)
}

// hostPortFromURL extracts (host, port) from a URL. port is
// inferred from the scheme when absent (80 for http, 443 for https).
// Parse errors yield ("", 0) — the policy then matches on the
// surviving args only.
func hostPortFromURL(rawURL string) (string, int) {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return "", 0
	}
	host := u.Hostname()
	portStr := u.Port()
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			return host, p
		}
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return host, 80
	case "https":
		return host, 443
	case "ws":
		return host, 80
	case "wss":
		return host, 443
	}
	return host, 0
}

// Object/Fetch / Object/Fetch/Request / Object/Fetch/Response are
// owned by the lang/go/native package — they're consumed only by the
// fetch handler and tests in this package. Registration goes
// through eng.Builtin.RegisterExternalBuiltin in the var
// initialisers below so that any other package-level var
// referencing TFetch* (signature slices in natives.go) sees a
// non-nil pointer at slice-init time. FixedIDs come from the
// documented lang/go/native/fetch range (3000-3999) — see
// eng.TypeTable.RegisterExternalBuiltin for the allocation policy.
var (
	TFetchFunction = registerFetchType("Ideal/Fetch", 3000)
	TFetchRequest  = registerFetchType("Ideal/Fetch/Request", 3001)
	TFetchResponse = registerFetchType("Ideal/Fetch/Response", 3002)
)

func registerFetchType(path string, fixedID int) *eng.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin(path, fixedID, nil)
	if err != nil {
		// lint:allow-panic — init-time builtin registration; see
		// registerTimerType in engine/native_misc.go for rationale.
		panic(fmt.Sprintf("fetch: register %s: %v", path, err))
	}
	return t
}

const defaultFetchTimeout = 30 * time.Second

// The "fetch" word is registered via the consolidated Natives slice in
// natives.go. Handlers cover [string], [map], and [string, map] forms.
//
// All entry points consult the registry's policy before issuing the
// outbound request (or refuse if the network capability is
// uninstalled). The check sequence is:
//  1. global.network hard cap
//  2. network capability install gate
//  3. network.connect{host, port} per-host rule
//
// If any check denies, the HTTP request is never built and no
// outbound packet is sent.
//
// fetchStringHandler handles fetch with a single URL string argument.
// Performs a GET request to the given URL.
func fetchStringHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	reqOM := NewOrderedMap()
	reqOM.Set("url", args[0])
	return doFetch(reqOM, r)
}

// fetchStringMapHandler handles fetch with a URL string and an options map.
// The URL is merged into the options map as the "url" field.
func fetchStringMapHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	opts, _ := AsMap(args[1])
	if opts == nil {
		return nil, r.AqlError("fetch_error", "fetch: expected map for options, got nil", "fetch")
	}
	reqOM := NewOrderedMap()
	reqOM.Set("url", args[0])
	// Copy options into request map (url from first arg takes precedence).
	for _, key := range opts.Keys() {
		if key == "url" {
			continue
		}
		val, _ := opts.Get(key)
		reqOM.Set(key, val)
	}
	return doFetch(reqOM, r)
}

// fetchMapHandler handles fetch with a full request map.
// The map must contain a "url" field.
func fetchMapHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	m, _ := AsMap(args[0])
	if m == nil {
		return nil, r.AqlError("fetch_error", "fetch: expected map argument, got nil", "fetch")
	}
	return doFetch(m, r)
}

// doFetch performs a synchronous HTTP request from the given request map
// and returns a Map/Fetch/Response value.
//
// Consults the registry's policy (if any) before issuing the request:
// global.network hard cap, network scope install, and
// network.connect{host, port} per-host rule. Denial returns the
// policy error without building or sending any HTTP request.
//
// Request map fields:
//   - url     (string, required) — the URL to fetch
//   - method  (string, optional, default "GET") — HTTP method
//   - headers (map, optional) — request headers
//   - body    (string, optional) — request body
//   - timeout (integer, optional, default 30000) — timeout in milliseconds
func doFetch(reqOM ReadMap, r *Registry) ([]Value, error) {
	// Extract url (required).
	urlVal, ok := reqOM.Get("url")
	if !ok {
		return nil, fmt.Errorf("fetch: missing required \"url\" field")
	}
	urlStr, err := AsString(urlVal)
	if err != nil {
		return nil, fmt.Errorf("fetch: url: %w", err)
	}

	// Policy gate: consult host policy before opening any socket.
	if err := checkFetchPolicy(r, urlStr); err != nil {
		return nil, err
	}

	// Extract method (default GET).
	method := "GET"
	if mv, ok := reqOM.Get("method"); ok {
		mvStr, err := AsString(mv)
		if err != nil {
			return nil, fmt.Errorf("fetch: method: %w", err)
		}
		method = strings.ToUpper(mvStr)
	}

	// Extract body.
	var bodyReader io.Reader
	if bv, ok := reqOM.Get("body"); ok {
		bvStr, err := AsString(bv)
		if err != nil {
			return nil, fmt.Errorf("fetch: body: %w", err)
		}
		bodyReader = strings.NewReader(bvStr)
	}

	// Build http.Request.
	req, err := http.NewRequest(method, urlStr, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	// Set headers.
	if hv, ok := reqOM.Get("headers"); ok && hv.Parent.Matches(TMap) {
		hm, _ := AsMap(hv)
		for _, key := range hm.Keys() {
			val, _ := hm.Get(key)
			valStr, err := AsString(val)
			if err != nil {
				return nil, fmt.Errorf("fetch: header %q: %w", key, err)
			}
			req.Header.Set(key, valStr)
		}
	}

	// Timeout.
	timeout := defaultFetchTimeout
	if tv, ok := reqOM.Get("timeout"); ok {
		tvInt, err := AsInteger(tv)
		if err != nil {
			return nil, fmt.Errorf("fetch: timeout: %w", err)
		}
		timeout = time.Duration(tvInt) * time.Millisecond
	}

	// Execute request.
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	// Read response body.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetch: reading body: %w", err)
	}

	// Build response headers map with lowercase keys in sorted order.
	headersOM := NewOrderedMap()
	headerKeys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)
	for _, k := range headerKeys {
		headersOM.Set(strings.ToLower(k), NewString(strings.Join(resp.Header[k], ", ")))
	}

	// Build response map.
	respOM := NewOrderedMap()
	respOM.Set("ok", NewBoolean(resp.StatusCode >= 200 && resp.StatusCode <= 299))
	respOM.Set("status", NewInteger(int64(resp.StatusCode)))
	respOM.Set("headers", NewMap(headersOM))
	respOM.Set("body", NewString(string(bodyBytes)))
	respOM.Set("url", NewString(resp.Request.URL.String()))

	return []Value{{Parent: TFetchResponse, Data: MapPayload{M: respOM}}}, nil
}
