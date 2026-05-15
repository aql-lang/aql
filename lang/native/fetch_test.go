package native

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aql-lang/aql/lang/engine"
)

func TestFetchFunc(t *testing.T) {
	r, err := engine.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	Register(r)
	fn := r.Lookup("fetch")
	if fn == nil {
		t.Fatal("expected word 'fetch' to be registered")
	}
	if len(fn.Signatures) != 3 {
		t.Errorf("expected 3 signatures, got %d", len(fn.Signatures))
	}
}

func TestFetchStringHandler(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "test-value")
		w.WriteHeader(200)
		w.Write([]byte(`{"hello":"world"}`))
	}))
	defer ts.Close()

	result, err := fetchStringHandler(
		[]engine.Value{engine.NewString(ts.URL)},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	resp := result[0]
	if !resp.VType.Equal(TFetchResponse) {
		t.Errorf("expected type %s, got %s", TFetchResponse, resp.VType)
	}

	m := engine.AsMap(resp)

	okVal, _ := m.Get("ok")
	okb, _ := engine.AsBoolean(okVal)
	if !okb {
		t.Error("expected ok to be true")
	}

	statusVal, _ := m.Get("status")
	stati, _ := engine.AsInteger(statusVal)
	if stati != 200 {
		t.Errorf("expected status 200, got %d", stati)
	}

	bodyVal, _ := m.Get("body")
	bodys, _ := engine.AsString(bodyVal)
	if bodys != `{"hello":"world"}` {
		t.Errorf("expected body '{\"hello\":\"world\"}', got %q", bodys)
	}

	urlVal, _ := m.Get("url")
	urls, _ := engine.AsString(urlVal)
	if urls != ts.URL {
		t.Errorf("expected url %q, got %q", ts.URL, urls)
	}

	headersVal, _ := m.Get("headers")
	hm := engine.AsMap(headersVal)
	xCustom, ok := hm.Get("x-custom")
	if !ok {
		t.Error("expected x-custom header in response")
	} else {
		xcs, _ := engine.AsString(xCustom)
		if xcs != "test-value" {
			t.Errorf("expected x-custom 'test-value', got %q", xcs)
		}
	}
}

func TestFetchMapHandler(t *testing.T) {
	var receivedMethod string
	var receivedBody string
	var receivedHeader string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedHeader = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(201)
		w.Write([]byte("created"))
	}))
	defer ts.Close()

	headers := engine.NewOrderedMap()
	headers.Set("Authorization", engine.NewString("Bearer token123"))

	reqMap := engine.NewOrderedMap()
	reqMap.Set("url", engine.NewString(ts.URL))
	reqMap.Set("method", engine.NewString("POST"))
	reqMap.Set("headers", engine.NewMap(headers))
	reqMap.Set("body", engine.NewString(`{"name":"test"}`))

	result, err := fetchMapHandler(
		[]engine.Value{engine.NewMap(reqMap)},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if receivedMethod != "POST" {
		t.Errorf("expected POST, got %q", receivedMethod)
	}
	if receivedHeader != "Bearer token123" {
		t.Errorf("expected 'Bearer token123', got %q", receivedHeader)
	}
	if receivedBody != `{"name":"test"}` {
		t.Errorf("expected body '{\"name\":\"test\"}', got %q", receivedBody)
	}

	resp := engine.AsMap(result[0])
	okVal, _ := resp.Get("ok")
	okb, _ := engine.AsBoolean(okVal)
	if !okb {
		t.Error("expected ok to be true for 201")
	}
	statusVal, _ := resp.Get("status")
	stati, _ := engine.AsInteger(statusVal)
	if stati != 201 {
		t.Errorf("expected status 201, got %d", stati)
	}
	bodyVal, _ := resp.Get("body")
	bodys, _ := engine.AsString(bodyVal)
	if bodys != "created" {
		t.Errorf("expected body 'created', got %q", bodys)
	}
}

func TestFetchStringMapHandler(t *testing.T) {
	var receivedMethod string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	opts := engine.NewOrderedMap()
	opts.Set("method", engine.NewString("PUT"))

	result, err := fetchStringMapHandler(
		[]engine.Value{engine.NewString(ts.URL), engine.NewMap(opts)},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if receivedMethod != "PUT" {
		t.Errorf("expected PUT, got %q", receivedMethod)
	}

	resp := engine.AsMap(result[0])
	statusVal, _ := resp.Get("status")
	stati, _ := engine.AsInteger(statusVal)
	if stati != 200 {
		t.Errorf("expected status 200, got %d", stati)
	}
}

func TestFetchMissingURL(t *testing.T) {
	reqMap := engine.NewOrderedMap()
	reqMap.Set("method", engine.NewString("GET"))

	_, err := fetchMapHandler(
		[]engine.Value{engine.NewMap(reqMap)},
		nil, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error for missing url")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("expected error about url, got %q", err.Error())
	}
}

func TestFetchDefaultMethodIsGET(t *testing.T) {
	var receivedMethod string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer ts.Close()

	reqMap := engine.NewOrderedMap()
	reqMap.Set("url", engine.NewString(ts.URL))

	_, err := fetchMapHandler(
		[]engine.Value{engine.NewMap(reqMap)},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if receivedMethod != "GET" {
		t.Errorf("expected GET, got %q", receivedMethod)
	}
}

func TestFetchResponseNotOk(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("not found"))
	}))
	defer ts.Close()

	result, err := fetchStringHandler(
		[]engine.Value{engine.NewString(ts.URL)},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	resp := engine.AsMap(result[0])
	okVal, _ := resp.Get("ok")
	okb, _ := engine.AsBoolean(okVal)
	if okb {
		t.Error("expected ok to be false for 404")
	}
	statusVal, _ := resp.Get("status")
	stati, _ := engine.AsInteger(statusVal)
	if stati != 404 {
		t.Errorf("expected status 404, got %d", stati)
	}
	bodyVal, _ := resp.Get("body")
	bodys, _ := engine.AsString(bodyVal)
	if bodys != "not found" {
		t.Errorf("expected body 'not found', got %q", bodys)
	}
}

func TestFetchRedirect(t *testing.T) {
	// Final destination server.
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("final"))
	}))
	defer final.Close()

	// Redirect server.
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusMovedPermanently)
	}))
	defer redirect.Close()

	result, err := fetchStringHandler(
		[]engine.Value{engine.NewString(redirect.URL)},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	resp := engine.AsMap(result[0])
	urlVal, _ := resp.Get("url")
	urls, _ := engine.AsString(urlVal)
	if urls != final.URL {
		t.Errorf("expected final url %q, got %q", final.URL, urls)
	}
	bodyVal, _ := resp.Get("body")
	bodys, _ := engine.AsString(bodyVal)
	if bodys != "final" {
		t.Errorf("expected body 'final', got %q", bodys)
	}
}

func TestFetchTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	reqMap := engine.NewOrderedMap()
	reqMap.Set("url", engine.NewString(ts.URL))
	reqMap.Set("timeout", engine.NewInteger(1)) // 1ms timeout

	_, err := fetchMapHandler(
		[]engine.Value{engine.NewMap(reqMap)},
		nil, nil, nil,
	)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestFetchResponseHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "abc123")
		w.WriteHeader(200)
		w.Write([]byte("{}"))
	}))
	defer ts.Close()

	result, err := fetchStringHandler(
		[]engine.Value{engine.NewString(ts.URL)},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	resp := engine.AsMap(result[0])
	headersVal, _ := resp.Get("headers")
	hm := engine.AsMap(headersVal)

	ct, ok := hm.Get("content-type")
	if !ok {
		t.Error("expected content-type header")
	} else {
		cts, _ := engine.AsString(ct)
		if cts != "application/json" {
			t.Errorf("expected 'application/json', got %q", cts)
		}
	}

	xrid, ok := hm.Get("x-request-id")
	if !ok {
		t.Error("expected x-request-id header")
	} else {
		xrids, _ := engine.AsString(xrid)
		if xrids != "abc123" {
			t.Errorf("expected 'abc123', got %q", xrids)
		}
	}
}

func TestFetchResponseType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	result, err := fetchStringHandler(
		[]engine.Value{engine.NewString(ts.URL)},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	resp := result[0]
	// Object/Fetch/Response should match Object.
	if !resp.VType.Matches(engine.TObject) {
		t.Errorf("expected response to match Object, got %s", resp.VType)
	}
	if !resp.VType.Equal(TFetchResponse) {
		t.Errorf("expected type Object/Fetch/Response, got %s", resp.VType)
	}
}

func TestFetchServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer ts.Close()

	result, err := fetchStringHandler(
		[]engine.Value{engine.NewString(ts.URL)},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	resp := engine.AsMap(result[0])
	okVal, _ := resp.Get("ok")
	okb, _ := engine.AsBoolean(okVal)
	if okb {
		t.Error("expected ok to be false for 500")
	}
	statusVal, _ := resp.Get("status")
	stati, _ := engine.AsInteger(statusVal)
	if stati != 500 {
		t.Errorf("expected status 500, got %d", stati)
	}
}
