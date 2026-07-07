package modules

import (
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
	tabnasabnf "github.com/tabnas/abnf/go"
	tabnas "github.com/tabnas/parser/go"
)

// TestW8MicronSpecMapCheckBadSection drives micronSpecMapCheck's applySpecMap
// error propagation (minilang.go:784): an unknown grammar section.
func TestW8MicronSpecMapCheckBadSection(t *testing.T) {
	r := mcovReg(t)
	spec := native.NewOrderedMap()
	spec.Set("bogus", native.NewInteger(1))
	if err := micronSpecMapCheck(native.NewMap(spec), r, false); err == nil {
		t.Error("micronSpecMapCheck: an unknown section should error")
	}
}

// TestW8MicronGrammarForArms drives micronGrammarFor's not-a-grammar (866) and
// tag-already-set (872) arms.
func TestW8MicronGrammarForArms(t *testing.T) {
	r := mcovReg(t)
	kind := r.Types.MintType("W8mk", native.TMicron)

	// 866: an ExtensionPayload whose Body is not a *parseGrammar.
	notG := eng.NewExtension(native.TIdeal, "not-a-grammar")
	if _, err := micronGrammarFor(kind, notG, r); err == nil {
		t.Error("micronGrammarFor: a non-grammar extension should error")
	}

	// 872: a grammar carrier whose engine already carries a tag.
	tagged := &parseGrammar{j: tabnas.Make(tabnas.Options{Tag: "x"}), markActions: tabnasabnf.ActionsMap{}}
	taggedV := eng.NewExtension(native.TIdeal, tagged)
	if _, err := micronGrammarFor(kind, taggedV, r); err == nil {
		t.Error("micronGrammarFor: a pre-tagged grammar should error")
	}
}

// TestW8MicronFinalizeNoMatchToken drives micronGrammarFinalize's no-match-
// token arm (minilang.go:952): a tag-correct grammar with no match tokens.
func TestW8MicronFinalizeNoMatchToken(t *testing.T) {
	r := mcovReg(t)
	kind := r.Types.MintType("W8mf", native.TMicron)
	st := micronLitStateFor(r, true)
	// Tag matches the kind so the tag guard passes; no match tokens declared.
	g := &parseGrammar{
		j:           tabnas.Make(tabnas.Options{Tag: kind.Name}),
		markActions: tabnasabnf.ActionsMap{},
	}
	if _, err := micronGrammarFinalize(st, kind, g, r); err == nil {
		t.Error("micronGrammarFinalize: a grammar with no match token should error")
	}
}

// TestW8MicronHandlerSrcError drives miniMicronHandlerFor's src refusal
// (minilang.go:1143).
func TestW8MicronHandlerSrcError(t *testing.T) {
	r := mcovReg(t)
	h := miniMicronHandlerFor(r)
	if _, err := h([]native.Value{native.NewTypeLiteral(native.TString)}, nil, nil, r); err == nil {
		t.Error("micron handler: a non-concrete src should error")
	}
}

// TestW8MicronHandlerEmptyState drives miniMicronHandlerFor's empty-registry
// fast path (minilang.go:1152): a live-but-empty state falls back to the
// builtin merged grammar.
func TestW8MicronHandlerEmptyState(t *testing.T) {
	r := mcovReg(t)
	micronLitStateFor(r, true) // create an empty state (no specs)
	h := miniMicronHandlerFor(r)
	out, err := h([]native.Value{native.NewString("a@b.com")}, nil, nil, r)
	if err != nil {
		t.Fatalf("micron handler with an empty state should use the builtin grammar: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("expected one result, got %v", out)
	}
}
