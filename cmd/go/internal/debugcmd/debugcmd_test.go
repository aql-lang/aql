package debugcmd

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/debugserve"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
)

// attachToTestServer stands up a debugserve over a registry on an
// httptest server and returns a Client plus the registry (so a test can
// seed defs the attach should observe).
func attachToTestServer(t *testing.T) (*debugserve.Client, *native.Registry, func()) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	ts := httptest.NewServer(debugserve.NewServer(reg, "").Handler())
	return debugserve.NewClient(ts.URL, ""), reg, ts.Close
}

func TestAttachVerbEval(t *testing.T) {
	c, reg, closeFn := attachToTestServer(t)
	defer closeFn()
	// Seed a def the remote eval should see.
	if _, err := native.NewTop(reg).Run(mustParse(t, "def my-base 100")); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := attachVerb(c, []string{"eval", "my-base", "5", "add"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("eval exit %d, stderr=%q", code, errOut.String())
	}
	if strings.TrimSpace(out.String()) != "105" {
		t.Errorf("eval base 5 add = %q, want 105", strings.TrimSpace(out.String()))
	}
}

func TestAttachVerbDefs(t *testing.T) {
	c, reg, closeFn := attachToTestServer(t)
	defer closeFn()
	if _, err := native.NewTop(reg).Run(mustParse(t, "def my-marker 7")); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := attachVerb(c, []string{"defs"}, &out, &errOut); code != 0 {
		t.Fatalf("defs exit %d", code)
	}
	if !strings.Contains(out.String(), "my-marker = 7") {
		t.Errorf("defs should list the seeded binding; got:\n%s", out.String())
	}
}

func TestAttachVerbWordsAndHeap(t *testing.T) {
	c, _, closeFn := attachToTestServer(t)
	defer closeFn()
	var out, errOut bytes.Buffer
	if code := attachVerb(c, []string{"words"}, &out, &errOut); code != 0 {
		t.Fatalf("words exit %d", code)
	}
	if !strings.Contains(out.String(), "add") {
		t.Error("words should include 'add'")
	}
	out.Reset()
	if code := attachVerb(c, []string{"heap"}, &out, &errOut); code != 0 {
		t.Fatalf("heap exit %d", code)
	}
	if !strings.Contains(out.String(), "alloc=") {
		t.Errorf("heap should print alloc=...; got %q", out.String())
	}
}

func TestAttachVerbEvents(t *testing.T) {
	c, _, closeFn := attachToTestServer(t)
	defer closeFn()
	if err := c.Emit("inv1", debugserve.Event{Label: "x", Value: "9"}); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := attachVerb(c, []string{"events", "inv1"}, &out, &errOut); code != 0 {
		t.Fatalf("events exit %d", code)
	}
	if !strings.Contains(out.String(), "x = 9") {
		t.Errorf("events should print the emitted event; got %q", out.String())
	}
}

func TestAttachVerbUnknown(t *testing.T) {
	c, _, closeFn := attachToTestServer(t)
	defer closeFn()
	var out, errOut bytes.Buffer
	if code := attachVerb(c, []string{"frobnicate"}, &out, &errOut); code == 0 {
		t.Error("an unknown verb must exit non-zero")
	}
}

func mustParse(t *testing.T, src string) []native.Value {
	t.Helper()
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return toks
}
