package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// coverDenominator declines (nil) on a registry with no parser and on a source
// that fails to parse.
func TestCoverDenominatorDeclines(t *testing.T) {
	noParse, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// DefaultRegistry has no ParseFunc set until SetParseFunc.
	if d := coverDenominator(noParse, "def x 1"); d != nil {
		t.Fatalf("nil ParseFunc must decline, got %v", d)
	}

	withParse, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	withParse.SetParseFunc(parser.Parse)
	if d := coverDenominator(withParse, "((( not valid boru"); d != nil {
		t.Fatalf("unparseable source must decline, got %v", d)
	}
	// A parseable source yields a non-empty denominator.
	if d := coverDenominator(withParse, "def x 1\ndef y (x add 1)"); len(d) == 0 {
		t.Fatalf("parseable source must yield rows")
	}
}

// Test.coverage on a source id whose registered source fails to parse raises
// test_cover_bad_source (exercises the coverDenominator-nil arm in the native).
func TestCoverageBadSource(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)
	r.RegisterCoverSource("bad", "((( unparseable")
	vals, perr := parser.Parse(`import "boru:test"  Test.coverage "bad"`)
	if perr != nil {
		t.Fatal(perr)
	}
	_, rerr := native.NewTop(r).Run(vals)
	if rerr == nil || !contains(rerr.Error(), "test_cover_bad_source") {
		t.Fatalf("want test_cover_bad_source, got %v", rerr)
	}
}

// The arg-guard error arms: invoking the handlers directly with a non-concrete
// (type-literal) arg reaches the RequireConcreteList / AsConcreteString guards
// that normal sig dispatch cannot land (a type literal matches no forward sig).
func TestCoverHandlerArgGuards(t *testing.T) {
	parent, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handler := func(name string) native.Handler {
		for _, nf := range coverNatives(parent) {
			if nf.Name == name {
				return nf.Signatures[0].Impl.(*native.GoImpl).Handler
			}
		}
		t.Fatalf("native %q not found", name)
		return nil
	}
	// test-cover with a List type literal → RequireConcreteList guard.
	if _, e := handler("test-cover")([]native.Value{native.NewTypeLiteral(native.TList)}, nil, nil, parent); e == nil {
		t.Fatalf("test-cover of a non-concrete body must error")
	}
	// test-coverage with a String type literal → AsConcreteString guard.
	if _, e := handler("test-coverage")([]native.Value{native.NewTypeLiteral(native.TString)}, nil, nil, parent); e == nil {
		t.Fatalf("test-coverage of a non-concrete id must error")
	}
}
