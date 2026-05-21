package native

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aql-lang/aql/eng/go"
)

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
	TFetchFunction = registerFetchType("Ideal/Object/Fetch", 3000)
	TFetchRequest  = registerFetchType("Ideal/Object/Fetch/Request", 3001)
	TFetchResponse = registerFetchType("Ideal/Object/Fetch/Response", 3002)
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
// fetchStringHandler handles fetch with a single URL string argument.
// Performs a GET request to the given URL.
func fetchStringHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	reqOM := NewOrderedMap()
	reqOM.Set("url", args[0])
	return doFetch(reqOM)
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
	return doFetch(reqOM)
}

// fetchMapHandler handles fetch with a full request map.
// The map must contain a "url" field.
func fetchMapHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	m, _ := AsMap(args[0])
	if m == nil {
		return nil, r.AqlError("fetch_error", "fetch: expected map argument, got nil", "fetch")
	}
	return doFetch(m)
}

// doFetch performs a synchronous HTTP request from the given request map
// and returns a Map/Fetch/Response value.
//
// Request map fields:
//   - url     (string, required) — the URL to fetch
//   - method  (string, optional, default "GET") — HTTP method
//   - headers (map, optional) — request headers
//   - body    (string, optional) — request body
//   - timeout (integer, optional, default 30000) — timeout in milliseconds
func doFetch(reqOM ReadMap) ([]Value, error) {
	// Extract url (required).
	urlVal, ok := reqOM.Get("url")
	if !ok {
		return nil, fmt.Errorf("fetch: missing required \"url\" field")
	}
	urlStr, err := AsString(urlVal)
	if err != nil {
		return nil, fmt.Errorf("fetch: url: %w", err)
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
	if hv, ok := reqOM.Get("headers"); ok && hv.VType.Matches(TMap) {
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

	return []Value{{VType: TFetchResponse, Data: MapPayload{M: respOM}}}, nil
}
