package modules

import (
	"github.com/aql-lang/aql/lang/go/formatter"
	"github.com/aql-lang/aql/lang/go/native"
)

// The declarative rule-table surface of aql:fmt — the completion of the
// Phase-3 "formatting rules expressed as AQL" direction
// (design/fmt-module-and-xslt.0.md). The formatter's layout decisions live
// in a RULE TABLE (formatter.Rules); the Go emitter is the generic
// processor that interprets it, exactly as an XSLT processor interprets a
// stylesheet. Two words expose the split to AQL:
//
//	Fmt.rules                     the canonical rule table, as AQL data
//	Fmt.format-with <rules> <src> format src under a (partial) rule table
//
// A partial table overrides only the TOP-LEVEL keys it names; everything
// else keeps the canonical value. `Fmt.format-with {} src` therefore equals
// `Fmt.format src`, and `Fmt.format-with (Fmt.rules) src` proves the AQL
// representation is authoritative (pinned by spec rows + Go tests). Two
// keys differ in their inner granularity: `attach` REPLACES the whole
// attachment policy (kinds absent from the map attach nothing — pinned by
// the `x ? y` detachment in TestFormatWithOverrides), while `brackets`
// MERGES per container/side (omitted glyphs stay canonical).
//
// Table shape (all keys optional):
//
//	{width: 72                    max line width
//	 indent: 2                    indent step
//	 fn-word: 'fn'                fn-definition trigger word
//	 statement-starts: ['def' …]  words opening a new group in bodies
//	 attach: {comma:'prev' …}     kind → 'prev' | 'next' | 'both' | 'none'
//	 attach-dot-suffix: true      a word ending '.' glues the next token
//	 brackets: {list:{open:'[' close:']'} …}   container brackets
//	 strategies: ['comment-only' 'inline' …]}  statement templates, in order

// fmtRulesNative implements Fmt.rules: the canonical layout rule table as
// an AQL Map — the stylesheet the default formatter interprets.
func fmtRulesNative() native.NativeFunc {
	return native.NativeFunc{
		Name: "rules",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Impl:       native.Go(fmtRulesHandler),
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
		}},
	}
}

func fmtRulesHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	return []native.Value{rulesToValue(formatter.DefaultRules())}, nil
}

// fmtFormatWithNative implements Fmt.format-with: format AQL source under
// a caller-supplied (partial) rule table.
func fmtFormatWithNative() native.NativeFunc {
	return native.NativeFunc{
		Name: "format-with",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TMap, native.TString},
			Impl:       native.Go(fmtFormatWithHandler),
			Returns:    []*native.Type{native.TString},
			BarrierPos: -1,
		}},
	}
}

func fmtFormatWithHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	ru, err := valueToRules(args[0])
	if err != nil {
		return nil, err
	}
	src, serr := args[1].AsConcreteString()
	if serr != nil {
		return nil, serr
	}
	return []native.Value{native.NewString(formatter.FormatRules(src, ru))}, nil
}

// rulesToValue renders a formatter.Rules table as the AQL Map shape
// documented above. It is the write side of the authority round-trip:
// valueToRules(rulesToValue(r)) must reproduce r's behaviour exactly for
// any r whose attach lists are duplicate-free (duplicate kinds in a
// Go-built list carry no extra meaning — the sets are what the processor
// reads — and the canonical table has none).
func rulesToValue(ru formatter.Rules) native.Value {
	m := native.NewOrderedMap()
	m.Set("width", native.NewInteger(int64(ru.Width)))
	m.Set("indent", native.NewInteger(int64(ru.Indent)))
	m.Set("fn-word", native.NewString(ru.FnWord))

	starts := make([]native.Value, len(ru.StmtStartWords))
	for i, w := range ru.StmtStartWords {
		starts[i] = native.NewString(w)
	}
	m.Set("statement-starts", native.NewList(starts))

	// attach classes: prev-only → 'prev', next-only → 'next', both → 'both'.
	// The class is derived from SET membership (the processor reads the
	// lists as sets), so a kind duplicated in a Go-built list renders the
	// same as if listed once.
	prev := map[formatter.NodeKind]bool{}
	for _, k := range ru.AttachPrev {
		prev[k] = true
	}
	next := map[formatter.NodeKind]bool{}
	for _, k := range ru.AttachNext {
		next[k] = true
	}
	attach := native.NewOrderedMap()
	for _, k := range ru.AttachPrev {
		cls := "prev"
		if next[k] {
			cls = "both"
		}
		attach.Set(formatter.NodeKindName(k), native.NewString(cls))
	}
	for _, k := range ru.AttachNext {
		if !prev[k] {
			attach.Set(formatter.NodeKindName(k), native.NewString("next"))
		}
	}
	m.Set("attach", native.NewMap(attach))
	m.Set("attach-dot-suffix", native.NewBoolean(ru.AttachDotSuffix))

	brackets := native.NewOrderedMap()
	pair := func(open, close string) native.Value {
		p := native.NewOrderedMap()
		p.Set("open", native.NewString(open))
		p.Set("close", native.NewString(close))
		return native.NewMap(p)
	}
	brackets.Set("list", pair(ru.ListOpen, ru.ListClose))
	brackets.Set("map", pair(ru.MapOpen, ru.MapClose))
	brackets.Set("paren", pair(ru.ParenOpen, ru.ParenClose))
	m.Set("brackets", native.NewMap(brackets))

	strategies := make([]native.Value, len(ru.Strategies))
	for i, s := range ru.Strategies {
		strategies[i] = native.NewString(s)
	}
	m.Set("strategies", native.NewList(strategies))
	return native.NewMap(m)
}

// valueToRules reads a (partial) AQL rule table into a formatter.Rules,
// starting from the canonical (stylesheet-defined) table so omitted keys
// keep their standard values. The reading and validation live beside the
// stylesheet loader in the formatter package (formatter.MergeRulesValue) —
// one reader for both the embedded fmt-rules.aql and this runtime path.
func valueToRules(v native.Value) (formatter.Rules, error) {
	return formatter.MergeRulesValue(formatter.DefaultRules(), v)
}
