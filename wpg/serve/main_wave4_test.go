// Wave-4 coverage for the web REPL server: the index write-error arm
// (a client that disconnects mid-response) and the eval handler's
// result rendering over multiple stack values.
package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// failingWriter is a ResponseWriter whose body writes always fail, as
// they would when the client hangs up before the response is sent.
type failingWriter struct {
	header http.Header
	code   int
}

func (w *failingWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingWriter) WriteHeader(code int) { w.code = code }

func (w *failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("client went away")
}

func TestW4IndexWriteErrorLogged(t *testing.T) {
	mux := testMux(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := &failingWriter{}
	// Must not panic; the handler logs the write failure and returns.
	mux.ServeHTTP(w, req)
	if ct := w.Header().Get("Content-Type"); ct == "" {
		t.Error("Content-Type header not set before the failed write")
	}
}

func TestW4EvalMultiValueResult(t *testing.T) {
	mux := testMux(t)
	code, resp := postEval(t, mux, `{"code":"1 2 3"}`)
	if code != 200 {
		t.Fatalf("eval: status %d", code)
	}
	if resp.Error != "" {
		t.Fatalf("eval error: %s", resp.Error)
	}
	if len(resp.Result) != 3 || resp.Result[0] != "1" || resp.Result[2] != "3" {
		t.Errorf("result = %v, want [1 2 3]", resp.Result)
	}
}
