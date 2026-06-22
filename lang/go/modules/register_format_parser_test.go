package modules

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/native"
)

// upFormat is a trivial host Format that uppercases its content. It exercises
// the RegisterFormatParser bridge: one registration must reach BOTH `read`
// (by name + extension) and `parse` (as a kind).
type upFormat struct{}

func (upFormat) Decode(content string) ([]native.Value, error) {
	return []native.Value{native.NewString(strings.ToUpper(content))}, nil
}
func (upFormat) Encode(native.Value) (string, error) { return "", nil }

// TestRegisterFormatParserBothSurfaces registers one format via the bridge and
// proves it is reachable from `parse <name>` and from the read-side registry
// (HostFormats + the extension map).
func TestRegisterFormatParserBothSurfaces(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)

	if err := RegisterFormatParser(r, "up", upFormat{}, ".up"); err != nil {
		t.Fatalf("RegisterFormatParser: %v", err)
	}

	// read side: format installed under name, extension mapped.
	if _, ok := native.HostFormats(r)["up"]; !ok {
		t.Error("read registry missing 'up' format")
	}
	if exts := native.HostExtensions(r); exts == nil || exts["up"] != "up" {
		t.Errorf("extension .up not mapped: %v", native.HostExtensions(r))
	}

	// parse side: `parse up '<src>'` dispatches through the same Format.
	tokens, err := parser.Parse(`"aql:parselang" import end  parse up 'hi'`)
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

	src := `module [ "aql:io" import end export "R" {v: (IO.read "x.uc")} ] import end R.v`
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

// TestRegisterFormatParserCollision: the parse side rejects a collision with a
// built-in tabnas kind, mirroring RegisterHostParser's contract — and the
// rejected bridge is ATOMIC: the read-side registries must be left untouched
// (built-in 'ini' reader not replaced, '.myini' extension not added).
func TestRegisterFormatParserCollision(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	before := native.HostFormats(r)["ini"]
	if err := RegisterFormatParser(r, "ini", upFormat{}, ".myini"); err == nil {
		t.Error("expected collision error registering 'ini' (built-in kind)")
	}
	// Atomicity: read side must be unchanged after the failed registration.
	if got := native.HostFormats(r)["ini"]; got != before {
		t.Errorf("built-in 'ini' reader was replaced by a failed registration")
	}
	if _, isUp := native.HostFormats(r)["ini"].(upFormat); isUp {
		t.Errorf("'ini' format was overwritten with the custom upFormat")
	}
	if exts := native.HostExtensions(r); exts != nil {
		if _, mapped := exts["myini"]; mapped {
			t.Errorf("'.myini' extension was mapped by a failed registration")
		}
	}
}
