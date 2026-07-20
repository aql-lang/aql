package modules

import (
	stdfmt "fmt"

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
// starting from the canonical defaults so omitted keys keep their standard
// values. Every key and enum value is validated — an unknown rule key, node
// kind, attach class, or strategy name is an error, not a silent no-op.
func valueToRules(v native.Value) (formatter.Rules, error) {
	ru := formatter.DefaultRules()
	m, err := native.RequireConcreteMap(v, "fmt format-with")
	if err != nil {
		return ru, err
	}
	for _, key := range m.Keys() {
		val, _ := m.Get(key)
		switch key {
		case "width":
			n, nerr := intRuleField(val, "width")
			if nerr != nil {
				return ru, nerr
			}
			ru.Width = n
		case "indent":
			n, nerr := intRuleField(val, "indent")
			if nerr != nil {
				return ru, nerr
			}
			ru.Indent = n
		case "fn-word":
			s, serr := val.AsConcreteString()
			if serr != nil {
				return ru, stdfmt.Errorf("fmt rules: fn-word must be a String: %w", serr)
			}
			ru.FnWord = s
		case "statement-starts":
			words, lerr := stringListField(val, "statement-starts")
			if lerr != nil {
				return ru, lerr
			}
			ru.StmtStartWords = words
		case "attach":
			if aerr := readAttach(val, &ru); aerr != nil {
				return ru, aerr
			}
		case "attach-dot-suffix":
			b, berr := val.AsConcreteBoolean()
			if berr != nil {
				return ru, stdfmt.Errorf("fmt rules: attach-dot-suffix must be a Boolean: %w", berr)
			}
			ru.AttachDotSuffix = b
		case "brackets":
			if berr := readBrackets(val, &ru); berr != nil {
				return ru, berr
			}
		case "strategies":
			names, lerr := stringListField(val, "strategies")
			if lerr != nil {
				return ru, lerr
			}
			known := map[string]bool{}
			for _, s := range formatter.StrategyNames() {
				known[s] = true
			}
			for _, s := range names {
				if !known[s] {
					return ru, stdfmt.Errorf("fmt rules: unknown strategy %q (known: %v)", s, formatter.StrategyNames())
				}
			}
			ru.Strategies = names
		default:
			return ru, stdfmt.Errorf("fmt rules: unknown rule key %q", key)
		}
	}
	return ru, nil
}

// maxRuleMagnitude bounds width/indent values from an AQL rule table.
// Layout dimensions beyond ±4096 columns are nonsensical and a huge indent
// would otherwise force absurd padding allocations (the processor also
// clamps defensively — this boundary rejects loudly instead of clamping
// silently).
const maxRuleMagnitude = 4096

// intRuleField reads a bounded integer rule-table field (width / indent).
func intRuleField(v native.Value, field string) (int, error) {
	n, err := v.AsConcreteInteger()
	if err != nil {
		return 0, stdfmt.Errorf("fmt rules: %s must be an Integer: %w", field, err)
	}
	if n < -maxRuleMagnitude || n > maxRuleMagnitude {
		return 0, stdfmt.Errorf("fmt rules: %s %d out of range (|%s| must be <= %d)", field, n, field, maxRuleMagnitude)
	}
	return int(n), nil
}

// stringListField reads a rule-table field that must be a List of Strings.
func stringListField(v native.Value, field string) ([]string, error) {
	lst, err := native.RequireConcreteList(v, "fmt rules")
	if err != nil {
		return nil, stdfmt.Errorf("fmt rules: %s must be a List: %w", field, err)
	}
	out := make([]string, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		s, serr := lst.Get(i).AsConcreteString()
		if serr != nil {
			return nil, stdfmt.Errorf("fmt rules: %s[%d] must be a String: %w", field, i, serr)
		}
		out[i] = s
	}
	return out, nil
}

// readAttach reads the attach-class map ({comma:'prev' colon:'both' …})
// into the Rules' AttachPrev/AttachNext kind sets. Setting the key REPLACES
// the whole policy: kinds absent from the map attach nothing.
func readAttach(v native.Value, ru *formatter.Rules) error {
	m, err := native.RequireConcreteMap(v, "fmt rules")
	if err != nil {
		return stdfmt.Errorf("fmt rules: attach must be a Map: %w", err)
	}
	ru.AttachPrev = nil
	ru.AttachNext = nil
	for _, name := range m.Keys() {
		kind, ok := formatter.KindByName(name)
		if !ok {
			return stdfmt.Errorf("fmt rules: attach: unknown node kind %q", name)
		}
		cv, _ := m.Get(name)
		cls, cerr := cv.AsConcreteString()
		if cerr != nil {
			return stdfmt.Errorf("fmt rules: attach.%s must be a String class: %w", name, cerr)
		}
		switch cls {
		case "prev":
			ru.AttachPrev = append(ru.AttachPrev, kind)
		case "next":
			ru.AttachNext = append(ru.AttachNext, kind)
		case "both":
			ru.AttachPrev = append(ru.AttachPrev, kind)
			ru.AttachNext = append(ru.AttachNext, kind)
		case "none":
			// explicit no-attachment: named for clarity, adds nothing.
		default:
			return stdfmt.Errorf("fmt rules: attach.%s: unknown class %q (want prev/next/both/none)", name, cls)
		}
	}
	return nil
}

// readBrackets reads the container-bracket map
// ({list:{open:'[' close:']'} …}) into the Rules' bracket fields. Each
// container and each of open/close is optional — omitted ones keep their
// canonical glyphs.
func readBrackets(v native.Value, ru *formatter.Rules) error {
	m, err := native.RequireConcreteMap(v, "fmt rules")
	if err != nil {
		return stdfmt.Errorf("fmt rules: brackets must be a Map: %w", err)
	}
	for _, name := range m.Keys() {
		var open, close *string
		switch name {
		case "list":
			open, close = &ru.ListOpen, &ru.ListClose
		case "map":
			open, close = &ru.MapOpen, &ru.MapClose
		case "paren":
			open, close = &ru.ParenOpen, &ru.ParenClose
		default:
			return stdfmt.Errorf("fmt rules: brackets: unknown container %q (want list/map/paren)", name)
		}
		pv, _ := m.Get(name)
		pm, perr := native.RequireConcreteMap(pv, "fmt rules")
		if perr != nil {
			return stdfmt.Errorf("fmt rules: brackets.%s must be a Map: %w", name, perr)
		}
		for _, side := range pm.Keys() {
			sv, _ := pm.Get(side)
			s, serr := sv.AsConcreteString()
			if serr != nil {
				return stdfmt.Errorf("fmt rules: brackets.%s.%s must be a String: %w", name, side, serr)
			}
			switch side {
			case "open":
				*open = s
			case "close":
				*close = s
			default:
				return stdfmt.Errorf("fmt rules: brackets.%s: unknown side %q (want open/close)", name, side)
			}
		}
	}
	return nil
}
