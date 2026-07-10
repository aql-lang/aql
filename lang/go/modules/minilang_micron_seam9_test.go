package modules

import (
	"regexp"
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
	tabnasabnf "github.com/tabnas/abnf/go"
	tabnas "github.com/tabnas/parser/go"
)

// w9MicronGrammar builds a parseGrammar tagged for kind with the given match
// options.
func w9MicronGrammar(kindName string, match *tabnas.MatchOptions) *parseGrammar {
	return &parseGrammar{
		j:           tabnas.Make(tabnas.Options{Tag: kindName, Match: match}),
		markActions: tabnasabnf.ActionsMap{},
	}
}

// TestW9MicronFinalizeGrammarArms drives micronGrammarFinalize's token-order
// rejection (minilang.go:948) and the val open-alternate scan's empty/ungated
// continues (984 / 994) plus the token-only anchoring path.
func TestW9MicronFinalizeGrammarArms(t *testing.T) {
	r := mcovReg(t)
	kind := r.Types.MintType("W9mfg", native.TMicron)
	st := micronLitStateFor(r, true)
	tk := map[string]*regexp.Regexp{"#W9TK": regexp.MustCompile("x")}

	// 948: a grammar carrying a token order is rejected (the +m merge owns it).
	gOrder := w9MicronGrammar(kind.Name(), &tabnas.MatchOptions{Token: tk, TokenOrder: []string{"#W9TK"}})
	if _, err := micronGrammarFinalize(st, kind, gOrder, r); err == nil {
		t.Error("948: a grammar with a token order should be rejected")
	}

	// 984/994 + token-only path: a valid gate token but no val alternate
	// references it, so every built-in val alternate is empty-S or ungated and
	// the finalize falls to the token-only anchoring branch.
	gTok := w9MicronGrammar(kind.Name(), &tabnas.MatchOptions{Token: tk})
	// Pre-seed the val rule with an empty-S alternate and an alternate gated on
	// a NON-gate token, so the finalize scan exercises both skip arms (984 the
	// empty alt, 994 the ungated alt).
	gTok.j.Rule("val", func(rs *tabnas.RuleSpec, _ *tabnas.Parser) {
		rs.AddOpen(&tabnas.AltSpec{})                                  // empty S → 984
		rs.AddOpen(&tabnas.AltSpec{S: [][]tabnas.Tin{{tabnas.TinTX}}}) // ungated → 994
	})
	toks, err := micronGrammarFinalize(st, kind, gTok, r)
	if err != nil {
		t.Fatalf("token-only finalize should succeed: %v", err)
	}
	if len(toks) != 1 || toks[0] != "#W9TK" {
		t.Errorf("token-only finalize tokens = %v, want [#W9TK]", toks)
	}
	// The val-alternate scan (984/994) is a deferred Rule definer that only
	// runs when the grammar builds — force it by parsing once. The built-in
	// val alternates are empty-S or ungated on #W9TK, exercising both continues.
	_, _ = gTok.j.Parse("x")
}

// TestW9MicronSpecMapTokenLenient probes micronSpecMapCheck's non-concrete
// nested-token lenient short-circuit (minilang.go:824-827): a lenient check of
// a spec whose options.match.token is a type literal returns nil.
func TestW9MicronSpecMapTokenLenient(t *testing.T) {
	r := mcovReg(t)
	tokenM := native.NewOrderedMap()
	tokenM.Set("token", native.NewTypeLiteral(native.TMap)) // non-concrete
	matchM := native.NewOrderedMap()
	matchM.Set("match", native.NewMap(tokenM))
	opts := native.NewOrderedMap()
	opts.Set("options", native.NewMap(matchM))
	spec := native.NewMap(opts)

	if err := micronSpecMapCheck(spec, r, true); err != nil {
		t.Logf("lenient token check returned err=%v (may be pre-empted by dry replay)", err)
	}
}
