package modules

import (
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

// w9NopParser is a do-nothing host parser handler (never invoked — the install
// collision fails before dispatch).
func w9NopParser([]native.Value, map[string]native.Value, []native.Value, *native.Registry) ([]native.Value, error) {
	return nil, nil
}

// TestW9ParseLangBuildDuplicate drives installHostParser's already-registered
// arm from the module BUILD loop (parselang.go:163). RegisterHostParser rejects
// both built-in names and duplicate host names up front, so the only way a
// colliding spec reaches the merged install loop is a corrupted host state —
// which parselang.go documents as this loop's single coverable install-error
// arm. We inject a duplicate directly into state.specs to exercise it.
func TestW9ParseLangBuildDuplicate(t *testing.T) {
	r := mcovReg(t)
	state := parseLangHostStateFor(r, true)
	dup := ParseLangSpec{Name: "w9dup", Handler: w9NopParser}
	state.specs = append(state.specs, dup, dup) // same name twice → loop collision
	if _, err := BuildParseLangModule(r); err == nil {
		t.Error("BuildParseLangModule: a duplicate host-parser name should error")
	}
}

// TestW9RegisterHostParserLiveDuplicate drives installHostParser's
// already-registered arm from the LIVE RegisterHostParser path
// (parselang.go:256). The name must pass the built-in and specs-duplicate
// guards yet still collide inside the live exports, so we inject a matching
// key into the built module's exports directly.
func TestW9RegisterHostParserLiveDuplicate(t *testing.T) {
	r := mcovReg(t)
	if _, err := BuildParseLangModule(r); err != nil {
		t.Fatalf("BuildParseLangModule: %v", err)
	}
	state := parseLangHostStateFor(r, false)
	if state == nil || state.live == nil {
		t.Fatal("expected a live parselang state after build")
	}
	// A key present in live exports but absent from state.specs: the name
	// passes RegisterHostParser's guards but collides at the live install.
	state.live.exports.Set("parse_w9host", native.NewInteger(1))
	spec := ParseLangSpec{Name: "w9host", Handler: w9NopParser}
	if err := RegisterHostParser(r, spec); err == nil {
		t.Error("RegisterHostParser: a live exports collision should error")
	}
}
