package native

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

func TestFetchFunc(t *testing.T) {
	fn := fetchFunc()
	if fn.Name != "fetch" {
		t.Errorf("expected name 'fetch', got %q", fn.Name)
	}
	if !fn.ForwardPrecedence {
		t.Error("expected ForwardPrecedence to be true")
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
	if !resp.VType.Equal(engine.TFetchResponse) {
		t.Errorf("expected type %s, got %s", engine.TFetchResponse, resp.VType)
	}

	m := resp.AsMap()

	okVal, _ := m.Get("ok")
	if !okVal.AsBoolean() {
		t.Error("expected ok to be true")
	}

	statusVal, _ := m.Get("status")
	if statusVal.AsInteger() != 200 {
		t.Errorf("expected status 200, got %d", statusVal.AsInteger())
	}

	bodyVal, _ := m.Get("body")
	if bodyVal.AsString() != `{"hello":"world"}` {
		t.Errorf("expected body '{\"hello\":\"world\"}', got %q", bodyVal.AsString())
	}

	urlVal, _ := m.Get("url")
	if urlVal.AsString() != ts.URL {
		t.Errorf("expected url %q, got %q", ts.URL, urlVal.AsString())
	}

	headersVal, _ := m.Get("headers")
	hm := headersVal.AsMap()
	xCustom, ok := hm.Get("x-custom")
	if !ok {
		t.Error("expected x-custom header in response")
	} else if xCustom.AsString() != "test-value" {
		t.Errorf("expected x-custom 'test-value', got %q", xCustom.AsString())
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

	resp := result[0].AsMap()
	okVal, _ := resp.Get("ok")
	if !okVal.AsBoolean() {
		t.Error("expected ok to be true for 201")
	}
	statusVal, _ := resp.Get("status")
	if statusVal.AsInteger() != 201 {
		t.Errorf("expected status 201, got %d", statusVal.AsInteger())
	}
	bodyVal, _ := resp.Get("body")
	if bodyVal.AsString() != "created" {
		t.Errorf("expected body 'created', got %q", bodyVal.AsString())
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

	resp := result[0].AsMap()
	statusVal, _ := resp.Get("status")
	if statusVal.AsInteger() != 200 {
		t.Errorf("expected status 200, got %d", statusVal.AsInteger())
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

	resp := result[0].AsMap()
	okVal, _ := resp.Get("ok")
	if okVal.AsBoolean() {
		t.Error("expected ok to be false for 404")
	}
	statusVal, _ := resp.Get("status")
	if statusVal.AsInteger() != 404 {
		t.Errorf("expected status 404, got %d", statusVal.AsInteger())
	}
	bodyVal, _ := resp.Get("body")
	if bodyVal.AsString() != "not found" {
		t.Errorf("expected body 'not found', got %q", bodyVal.AsString())
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

	resp := result[0].AsMap()
	urlVal, _ := resp.Get("url")
	if urlVal.AsString() != final.URL {
		t.Errorf("expected final url %q, got %q", final.URL, urlVal.AsString())
	}
	bodyVal, _ := resp.Get("body")
	if bodyVal.AsString() != "final" {
		t.Errorf("expected body 'final', got %q", bodyVal.AsString())
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

	resp := result[0].AsMap()
	headersVal, _ := resp.Get("headers")
	hm := headersVal.AsMap()

	ct, ok := hm.Get("content-type")
	if !ok {
		t.Error("expected content-type header")
	} else if ct.AsString() != "application/json" {
		t.Errorf("expected 'application/json', got %q", ct.AsString())
	}

	xrid, ok := hm.Get("x-request-id")
	if !ok {
		t.Error("expected x-request-id header")
	} else if xrid.AsString() != "abc123" {
		t.Errorf("expected 'abc123', got %q", xrid.AsString())
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
	if !resp.VType.Equal(engine.TFetchResponse) {
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

	resp := result[0].AsMap()
	okVal, _ := resp.Get("ok")
	if okVal.AsBoolean() {
		t.Error("expected ok to be false for 500")
	}
	statusVal, _ := resp.Get("status")
	if statusVal.AsInteger() != 500 {
		t.Errorf("expected status 500, got %d", statusVal.AsInteger())
	}
}
