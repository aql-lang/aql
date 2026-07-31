package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
)

// upFormat is a trivial host Format that uppercases its content. It
// exercises the two format surfaces post-freeze: native.RegisterFormat for
// the `read` side, and NewFormatParserFn — the bridge's value-form
// successor — for the `parse` side.
type upFormat struct{}

func (upFormat) Decode(content string) ([]native.Value, error) {
	return []native.Value{native.NewString(strings.ToUpper(content))}, nil
}
func (upFormat) Encode(native.Value) (string, error) { return "", nil }

// TestFormatParserFnBothSurfaces wires one Format onto both surfaces the
// post-freeze way: native.RegisterFormat for `read` (name + extension), and
// a NewFormatParserFn VALUE bound to a name for `parse` — which needs no
// boru:parselang import at all (the value form is import-free).
func TestFormatParserFnBothSurfaces(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)

	// read side: format installed under name, extension mapped.
	if err := native.RegisterFormat(r, "up", upFormat{}, ".up"); err != nil {
		t.Fatalf("RegisterFormat: %v", err)
	}
	if _, ok := native.HostFormats(r)["up"]; !ok {
		t.Error("read registry missing 'up' format")
	}
	if exts := native.HostExtensions(r); exts == nil || exts["up"] != "up" {
		t.Errorf("extension .up not mapped: %v", native.HostExtensions(r))
	}

	// parse side: the format's decoder as a bound ParseLang value.
	fnv, err := NewFormatParserFn("up", upFormat{})
	if err != nil {
		t.Fatalf("NewFormatParserFn: %v", err)
	}
	native.InstallDef(r, "up", fnv)
	tokens, err := parser.Parse(`parse up 'hi'`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := native.NewTop(r).Run(tokens)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("parse up = %v, want one value", out)
	}
	if s, _ := native.AsString(out[0]); s != "HI" {
		t.Errorf("parse up 'hi' = %q, want HI", s)
	}
}

// TestModuleBodyInheritsHostFormats: a host-registered format + extension on
// the parent registry must propagate into a module body's own registry, so an
// IO.read inside the module decodes via the custom format. Without inheritance
// the read would silently fall back to text (the raw lowercase string).
func TestModuleBodyInheritsHostFormats(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)

	// Custom read format (uppercases) + extension on the PARENT only.
	if err := native.RegisterFormat(r, "uc", upFormat{}, ".uc"); err != nil {
		t.Fatalf("RegisterFormat: %v", err)
	}
	mem := capabilities.NewMem()
	mem.Files["x.uc"] = []byte("hi")
	native.SetHostFileOps(r, mem)

	src := `import module [ import "boru:io" export "R" {v: (IO.read (make Pathon "x.uc"))} ] R.v`
	tokens, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := native.NewTop(r).Run(tokens)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("out = %v, want one value", out)
	}
	if s, _ := native.AsString(out[0]); s != "HI" {
		t.Errorf("module IO.read .uc = %q, want HI (custom format inherited; 'hi' = text fallback)", s)
	}
}

// (TestRegisterFormatParserCollision died with the bridge: there is no kind
// name to collide — the parse side is a def-scoped value, and the read side
// keeps native.RegisterFormat's own collision contract, pinned in native.)
