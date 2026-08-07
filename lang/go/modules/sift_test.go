package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// boru:sift is implemented in boru (sift.boru, embedded + run by BuildSiftModule).
// Its behaviour is pinned by the executable spec suite in
// lang/spec/module-sift.tsv; these Go tests only assert that the boru module
// LOADS and exposes the Sift namespace, plus a couple of end-to-end smokes.

func siftRun(t *testing.T, src string) ([]native.Value, error) {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return native.NewTop(r).Run(values)
}

// TestSiftModuleLoads asserts the boru-implemented module builds and exports
// the seven Sift words.
func TestSiftModuleLoads(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)
	desc, err := BuildSiftModule(r)
	if err != nil {
		t.Fatalf("BuildSiftModule: %v", err)
	}
	sift, ok := desc.Exports["Sift"]
	if !ok {
		t.Fatal("expected a 'Sift' export")
	}
	for _, name := range []string{"parse", "define", "kinds", "families", "spec", "detect", "check"} {
		if _, ok := sift.Get(name); !ok {
			t.Errorf("missing Sift export %q", name)
		}
	}
}

// TestSiftSmoke exercises the namespace end-to-end through import.
func TestSiftSmoke(t *testing.T) {
	cases := []struct{ src, want string }{
		{`import "boru:sift"  Sift.families`, "[kv blocks columns dsv fixed pattern]"},
		{`import "boru:sift"  Sift.kinds`, "[kv blocks columns dsv fixed pattern]"},
		{"import \"boru:sift\"  Sift.parse kv/q {sep:':'} \"a: 1\\nb: 2\"", "{a:'1' b:'2'}"},
	}
	for _, tc := range cases {
		res, err := siftRun(t, tc.src)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.src, err)
			continue
		}
		if len(res) != 1 {
			t.Errorf("%q: expected 1 result, got %d", tc.src, len(res))
			continue
		}
		if got := res[0].String(); got != tc.want {
			t.Errorf("%q = %s, want %s", tc.src, got, tc.want)
		}
	}
}
