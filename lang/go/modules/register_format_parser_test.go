package modules

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
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

// TestRegisterFormatParserCollision: the parse side rejects a collision with a
// built-in tabnas kind, mirroring RegisterHostParser's contract.
func TestRegisterFormatParserCollision(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterFormatParser(r, "ini", upFormat{}, ".myini"); err == nil {
		t.Error("expected collision error registering 'ini' (built-in kind)")
	}
}
