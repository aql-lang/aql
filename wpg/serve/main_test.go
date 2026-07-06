package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

func testMux(t *testing.T) *http.ServeMux {
	t.Helper()
	instance, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	return newMux(instance)
}

func postEval(t *testing.T, mux http.Handler, body string) (int, evalResponse) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/eval", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var resp evalResponse
	if rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("bad JSON response %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, resp
}

func TestIndexServed(t *testing.T) {
	mux := testMux(t)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET /: status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET /: Content-Type %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "AQL REPL") {
		t.Error("GET /: index page missing the REPL shell")
	}
}

func TestEvalExpression(t *testing.T) {
	mux := testMux(t)
	code, resp := postEval(t, mux, `{"code":"1 add 2"}`)
	if code != 200 {
		t.Fatalf("eval: status %d", code)
	}
	if resp.Error != "" {
		t.Fatalf("eval error: %s", resp.Error)
	}
	if len(resp.Result) != 1 || resp.Result[0] != "3" {
		t.Errorf("eval result = %v, want [3]", resp.Result)
	}
}

func TestEvalStatePersistsAcrossRequests(t *testing.T) {
	mux := testMux(t)
	if code, resp := postEval(t, mux, `{"code":"def xw 41"}`); code != 200 || resp.Error != "" {
		t.Fatalf("def: status %d error %q", code, resp.Error)
	}
	_, resp := postEval(t, mux, `{"code":"xw add 1"}`)
	if len(resp.Result) != 1 || resp.Result[0] != "42" {
		t.Errorf("state result = %v, want [42]", resp.Result)
	}
}

func TestEvalCapturesPrintOutput(t *testing.T) {
	mux := testMux(t)
	_, resp := postEval(t, mux, `{"code":"print 'hi'"}`)
	if !strings.Contains(resp.Output, "hi") {
		t.Errorf("output = %q, want it to contain %q", resp.Output, "hi")
	}
}

func TestEvalRejections(t *testing.T) {
	mux := testMux(t)

	// Wrong method.
	req := httptest.NewRequest("GET", "/api/eval", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/eval: status %d, want 405", rec.Code)
	}

	// Malformed body.
	if _, resp := postEval(t, mux, `{oops`); resp.Error != "invalid request body" {
		t.Errorf("bad body error = %q, want invalid request body", resp.Error)
	}

	// AQL-level error surfaces in the error field, not as HTTP failure.
	code, resp := postEval(t, mux, `{"code":"definitely-not-a-word"}`)
	if code != 200 {
		t.Fatalf("aql error eval: status %d, want 200", code)
	}
	if resp.Error == "" {
		t.Error("aql error eval: empty error field, want undefined-word error")
	}
}
