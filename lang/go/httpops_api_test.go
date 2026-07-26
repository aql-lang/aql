package lang_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go"
)

// stubRoundTripper serves a canned response and counts calls, so a test
// can prove fetch went through the installed transport and never
// touched the network.
type stubRoundTripper struct {
	hits int
	url  string
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.hits++
	s.url = req.URL.String()
	return &http.Response{
		StatusCode: 204,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

type stubHTTPOps struct{ rt http.RoundTripper }

func (s stubHTTPOps) Transport(_ lang.TLSProfile, _ lang.ClientIdentity) (http.RoundTripper, error) {
	return s.rt, nil
}

// SetHTTPOps redirects fetch through a host-supplied transport. The URL
// is unroutable, so a passing test also proves no real request was made.
func TestSetHTTPOps(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	rt := &stubRoundTripper{}
	a.SetHTTPOps(stubHTTPOps{rt: rt})

	if _, err = a.RunInterp(`import "aql:net"`); err != nil {
		t.Fatal(err)
	}
	res, err := a.RunInterp(
		`(convert Map (Net.fetch "https://stub.invalid/thing")).status`)
	if err != nil {
		t.Fatal(err)
	}
	if rt.hits != 1 {
		t.Fatalf("installed transport called %d times, want 1", rt.hits)
	}
	if rt.url != "https://stub.invalid/thing" {
		t.Errorf("transport saw %q", rt.url)
	}
	if len(res) != 1 || fmt.Sprint(res[0]) != "204" {
		t.Errorf("status = %v, want [204]", res)
	}
}
