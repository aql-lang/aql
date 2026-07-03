package eng

import (
	"regexp"
	"testing"
)

// micronTestOrder is the no-extras token order the standalone-grammar
// tests build with — the same order MicronGrammarWith computes when no
// user shapes are registered.
var micronTestOrder = []string{"#EMAILON", "#URLON", "#PATHON"}

// TestMicronMergedGrammarDispatch pins the merged literal grammar's
// type dispatch: each builtin leaf contributes its own tabnas grammar,
// the merge combines them, and the shape decides the type — Emailon,
// then Urlon, then the Pathon catch-all.
func TestMicronMergedGrammarDispatch(t *testing.T) {
	cases := []struct {
		src  string
		kind *Type
	}{
		{"alice@example.com", TEmailon},
		{"bob@x.io", TEmailon},
		{"https://x.com/a?q=1", TUrlon},
		{"http://h.example:8080/p", TUrlon},
		{"a/b", TPathon},
		{"/abs/path", TPathon},
		{"9000", TPathon},         // any whitespace-free span is a valid single-segment path
		{"a@b@c", TPathon},        // two @s — not an Emailon shape, falls to the catch-all
		{"Alice<a@b.c>", TPathon}, // display-name form — excluded from the Emailon gate
		{"", TPathon},             // the empty relative path (handled before the lexer)
	}
	for _, c := range cases {
		v, err := MicronFromString(c.src)
		if err != nil {
			t.Errorf("MicronFromString(%q): unexpected error: %v", c.src, err)
			continue
		}
		if !v.Parent.Equal(c.kind) {
			t.Errorf("MicronFromString(%q) = %s, want %s", c.src, v.Parent.Name, c.kind.Name)
		}
	}
}

// TestMicronMergedGrammarValues pins that the merged grammar's actions
// run the SAME constructors `make` uses — the parsed value is
// content-equal to the make-built one.
func TestMicronMergedGrammarValues(t *testing.T) {
	v, err := MicronFromString("alice@example.com")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	made, err := makeEmailon(NewString("alice@example.com"))
	if err != nil {
		t.Fatalf("make: %v", err)
	}
	if !ValuesEqual(v, made[0]) {
		t.Fatalf("grammar value %s != constructor value %s", v.String(), made[0].String())
	}
	if got := v.String(); got != "alice@example.com" {
		t.Fatalf("render = %q", got)
	}
}

// TestMicronLeafGrammarsIndependent pins that each leaf's grammar
// stands alone — it accepts its own literal shape and REJECTS the
// other kinds' (the negative half of the merge contract: the merged
// dispatch comes from combining single-shape grammars, not from one
// grammar that happened to accept everything).
func TestMicronLeafGrammarsIndependent(t *testing.T) {
	em := micronEmailonGrammar(micronTestOrder)
	if _, err := em.Parse("alice@example.com"); err != nil {
		t.Errorf("emailon grammar rejected its own literal: %v", err)
	}
	if _, err := em.Parse("a/b"); err == nil {
		t.Error("emailon grammar accepted a path literal")
	}
	if _, err := em.Parse("https://x.com/a"); err == nil {
		t.Error("emailon grammar accepted a URL literal")
	}

	ur := micronUrlonGrammar(micronTestOrder)
	if _, err := ur.Parse("https://x.com/a"); err != nil {
		t.Errorf("urlon grammar rejected its own literal: %v", err)
	}
	if _, err := ur.Parse("alice@example.com"); err == nil {
		t.Error("urlon grammar accepted an email literal")
	}

	pt := micronPathonGrammar(micronTestOrder)
	if _, err := pt.Parse("a/b"); err != nil {
		t.Errorf("pathon grammar rejected its own literal: %v", err)
	}
}

// TestMicronMergeCommutative pins the tabnas merge contract for the
// micron grammars: merging in either direction dispatches identically
// (the shared micronTokenOrder carries the lexer precedence through
// the commutative option merge).
func TestMicronMergeCommutative(t *testing.T) {
	ab, err := micronEmailonGrammar(micronTestOrder).Merge(micronUrlonGrammar(micronTestOrder))
	if err != nil {
		t.Fatalf("merge e~u: %v", err)
	}
	abc, err := ab.Merge(micronPathonGrammar(micronTestOrder))
	if err != nil {
		t.Fatalf("merge (e~u)~p: %v", err)
	}
	cb, err := micronPathonGrammar(micronTestOrder).Merge(micronUrlonGrammar(micronTestOrder))
	if err != nil {
		t.Fatalf("merge p~u: %v", err)
	}
	cba, err := cb.Merge(micronEmailonGrammar(micronTestOrder))
	if err != nil {
		t.Fatalf("merge (p~u)~e: %v", err)
	}
	for _, m := range []interface {
		Parse(string) (any, error)
	}{abc, cba} {
		for src, kind := range map[string]*Type{
			"alice@example.com": TEmailon,
			"https://x.com/a":   TUrlon,
			"just/a/path":       TPathon,
		} {
			node, err := m.Parse(src)
			if err != nil {
				t.Errorf("merged parse %q: %v", src, err)
				continue
			}
			v, ok := node.(Value)
			if !ok || !v.Parent.Equal(kind) {
				t.Errorf("merged parse %q → %v, want %s", src, node, kind.Name)
			}
		}
	}
}

// TestMicronGrammarWithExtras pins the opt-in hook: an extra shape
// merges BETWEEN the builtin leaves and the Pathon catch-all — it
// claims spans Pathon would otherwise catch, never spans the builtin
// leaves own, and a builder error is loud (the pattern claimed the
// span), unlike the builtin RFC-edge Pathon fallback.
func TestMicronGrammarWithExtras(t *testing.T) {
	buildCalls := 0
	ticket := MicronLiteralSpec{
		Tag:     "Ticketon",
		Token:   "#U0TICKETON",
		Pattern: regexp.MustCompile(`\AT-[0-9]+\z`),
		Build: func(s string) (Value, error) {
			buildCalls++
			fields := NewOrderedMap()
			fields.Set("id", NewString(s))
			return NewValueRaw(TMicron, MicronPayload{Fields: fields}), nil
		},
	}
	m, err := MicronGrammarWith(ticket)
	if err != nil {
		t.Fatalf("MicronGrammarWith: %v", err)
	}

	node, err := m.Parse("T-123")
	if err != nil {
		t.Fatalf("parse T-123: %v", err)
	}
	v, ok := node.(Value)
	if !ok || !IsMicronValue(v) {
		t.Fatalf("T-123 → %v, want the extra's Micron value", node)
	}
	if got, _ := MicronProperty(v, "id"); !ValuesEqual(got, NewString("T-123")) {
		t.Fatalf("T-123 id property = %v", got)
	}
	if buildCalls != 1 {
		t.Fatalf("builder ran %d times, want 1", buildCalls)
	}

	// Builtin leaves keep their spans — the extra sits AFTER them.
	for src, kind := range map[string]*Type{
		"alice@example.com": TEmailon,
		"https://x.com/a":   TUrlon,
		"plain/path":        TPathon,
		"T-notanumber":      TPathon, // gate mismatch → catch-all
	} {
		node, err := m.Parse(src)
		if err != nil {
			t.Errorf("parse %q: %v", src, err)
			continue
		}
		v, ok := node.(Value)
		if !ok || !v.Parent.Equal(kind) {
			t.Errorf("parse %q → %v, want %s", src, node, kind.Name)
		}
	}

	// No extras ≡ the builtin merged grammar.
	base, err := MicronGrammarWith()
	if err != nil {
		t.Fatalf("MicronGrammarWith(): %v", err)
	}
	if _, err := base.Parse("T-123"); err != nil {
		t.Fatalf("base grammar should catch T-123 as a Pathon: %v", err)
	}
}
