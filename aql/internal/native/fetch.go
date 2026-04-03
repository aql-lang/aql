package native

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

const defaultFetchTimeout = 30 * time.Second

// fetchFunc returns the "fetch" native function definition.
// fetch is synchronous and blocks until a response (or failure).
// It mirrors the browser fetch API with three signatures:
//   - [string, map] — URL string plus options map (method, headers, body, timeout)
//   - [map]         — full request map containing at least a "url" field
//   - [string]      — simple GET by URL string
func fetchFunc() engine.NativeFunc {
	return engine.NativeFunc{
		Name:             "fetch",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TString, engine.TMap},
				Handler: fetchStringMapHandler,
			},
			{
				Args:    []engine.Type{engine.TMap},
				Handler: fetchMapHandler,
			},
			{
				Args:    []engine.Type{engine.TString},
				Handler: fetchStringHandler,
			},
		},
	}
}

// fetchStringHandler handles fetch with a single URL string argument.
// Performs a GET request to the given URL.
func fetchStringHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	reqOM := engine.NewOrderedMap()
	reqOM.Set("url", args[0])
	return doFetch(reqOM)
}

// fetchStringMapHandler handles fetch with a URL string and an options map.
// The URL is merged into the options map as the "url" field.
func fetchStringMapHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	opts := args[1].AsMap()
	if opts == nil {
		return nil, fmt.Errorf("fetch: expected map for options, got nil")
	}
	reqOM := engine.NewOrderedMap()
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
func fetchMapHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	m := args[0].AsMap()
	if m == nil {
		return nil, fmt.Errorf("fetch: expected map argument, got nil")
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
func doFetch(reqOM engine.ReadMap) ([]engine.Value, error) {
	// Extract url (required).
	urlVal, ok := reqOM.Get("url")
	if !ok {
		return nil, fmt.Errorf("fetch: missing required \"url\" field")
	}
	urlStr, err := urlVal.AsString()
	if err != nil {
		return nil, fmt.Errorf("fetch: url: %w", err)
	}

	// Extract method (default GET).
	method := "GET"
	if mv, ok := reqOM.Get("method"); ok {
		mvStr, err := mv.AsString()
		if err != nil {
			return nil, fmt.Errorf("fetch: method: %w", err)
		}
		method = strings.ToUpper(mvStr)
	}

	// Extract body.
	var bodyReader io.Reader
	if bv, ok := reqOM.Get("body"); ok {
		bvStr, err := bv.AsString()
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
	if hv, ok := reqOM.Get("headers"); ok && hv.VType.Matches(engine.TMap) {
		hm := hv.AsMap()
		for _, key := range hm.Keys() {
			val, _ := hm.Get(key)
			valStr, err := val.AsString()
			if err != nil {
				return nil, fmt.Errorf("fetch: header %q: %w", key, err)
			}
			req.Header.Set(key, valStr)
		}
	}

	// Timeout.
	timeout := defaultFetchTimeout
	if tv, ok := reqOM.Get("timeout"); ok {
		tvInt, err := tv.AsInteger()
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
	headersOM := engine.NewOrderedMap()
	headerKeys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)
	for _, k := range headerKeys {
		headersOM.Set(strings.ToLower(k), engine.NewString(strings.Join(resp.Header[k], ", ")))
	}

	// Build response map.
	respOM := engine.NewOrderedMap()
	respOM.Set("ok", engine.NewBoolean(resp.StatusCode >= 200 && resp.StatusCode <= 299))
	respOM.Set("status", engine.NewInteger(int64(resp.StatusCode)))
	respOM.Set("headers", engine.NewMap(headersOM))
	respOM.Set("body", engine.NewString(string(bodyBytes)))
	respOM.Set("url", engine.NewString(resp.Request.URL.String()))

	return []engine.Value{{VType: engine.TFetchResponse, Data: respOM}}, nil
}
