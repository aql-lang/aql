package modules

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
	tabnasabnf "github.com/tabnas/abnf/go"
	tabnas "github.com/tabnas/parser/go"
)

// The aql:parse module — the Parse namespace, a builder DSL for defining a
// CUSTOM parser and exposing it as a new `parse <name>` kind. Where
// aql:parselang ships built-in decoders (json, csv, yaml, …) and lets an AQL
// fn register as a parser, aql:parse lets a user CONSTRUCT a parser from
// grammar: an ABNF grammar, a declarative rule map ("grammar as Map
// subtypes"), custom lex matchers, and semantic actions that build custom
// data types. It is built on github.com/tabnas/parser/go (the engine under
// the jsonic layer) and github.com/tabnas/abnf/go (ABNF → grammar).
//
// The module is REGISTER-ONLY: there is no standalone run word. Building a
// grammar culminates in `Parse.register <name> <grammar>`, which registers a
// `parse <name>` kind via aql:parselang's host-parser framework
// (RegisterHostParser). The user then runs it with the ordinary `parse`
// macro:
//
//	import "aql:parse"
//	import "aql:parselang"
//	def g Parse.grammar                              # mint a builder, bind it
//	Parse.action g '@op:o:INC' ([nd:Any] => [1])     # custom data via a mark
//	Parse.abnf g "op = \"inc\" / \"dec\"" {start:'op'}
//	Parse.register op g                              # → a `parse op` kind
//	parse op 'inc'                                   # → 1
//
// The Grammar carrier is a shared pointer (mutated in place, like Store /
// Array), so it is bound to a `def` and threaded through each builder word as
// the first argument; the builder words return nothing. Building is deferred:
// tokens, declarative rules and ABNF installs are replayed at Parse.register
// (when every mark action is known), and custom matchers are applied last.

// parseGrammar is the host-backed builder a Parse.grammar call mints. It
// carries the tabnas engine under construction plus the AQL-fn-backed
// callbacks (mark actions) and the deferred ABNF installs. Pointer-backed so
// the chainable words mutate one instance.
type parseGrammar struct {
	j           *tabnas.Tabnas        // the engine under construction
	markActions tabnasabnf.ActionsMap // ABNF mark/phase ref -> AQL-fn-backed AltActions
	inlineSeq   int                   // counter for generated declarative action refs

	// steps are the deferred grammar-building operations (tokens, declarative
	// rules, ABNF installs) in call order. They run at Parse.register, when
	// every mark action is known. matchers are applied LAST, after every
	// grammar step, so an ABNF install (which can rebuild the lexer) cannot
	// drop a custom lex matcher.
	steps    []func() error
	matchers []pendingMatcher

	// registered marks the builder as consumed by Parse.register. The carrier
	// is a shared pointer and the registered parser handler closes over g/g.j,
	// so the builder is SINGLE-USE: any further builder word or a second
	// register on a consumed grammar raises rather than mutating a finalized
	// parser. Build a fresh Parse.grammar for a second parser.
	registered bool

	// firstErr captures the first error raised by an AQL-fn callback (a
	// matcher or action) during a parse; the parser handler surfaces it as a
	// parse error. Callbacks never panic (ADR-005) — they record here instead.
	firstErr error
	// r is the active registry for the current parse, set by the parser
	// handler for the duration of one j.Parse call so callbacks can dispatch
	// AQL fns re-entrantly. Single-threaded parse → safe; nil-guarded.
	r *native.Registry
}

type pendingMatcher struct {
	name string
	prio int
	fn   native.Value
}

// setErr records the first callback error (first wins).
func (g *parseGrammar) setErr(err error) {
	if g.firstErr == nil {
		g.firstErr = err
	}
}

// ensureOpen errors when the builder has already been registered (single-use).
func (g *parseGrammar) ensureOpen(word string, r *native.Registry) error {
	if g.registered {
		return r.AqlErrorHint("parse_grammar_done",
			word+": this grammar was already registered as a parse kind", word,
			"a Parse.grammar builder is single-use — build a fresh one for another parser")
	}
	return nil
}

// applyMatchers installs the pending custom lex matchers onto g.j and re-sorts
// Config.CustomMatchers by priority. The tabnas lexer assumes the slice is
// priority-sorted as it interleaves custom matchers with the built-in
// match/fixed/text bands, so an out-of-order matcher (a lower priority added
// after a higher one) would otherwise run too late to claim its tokens — the
// same reason eng/go/parser/grammar.go::addMatcher sorts after appending.
func (g *parseGrammar) applyMatchers() {
	cfg := g.j.Config()
	for _, mt := range g.matchers {
		cfg.CustomMatchers = append(cfg.CustomMatchers, &tabnas.MatcherEntry{
			Name:     mt.name,
			Priority: mt.prio,
			Match:    g.wrapMatcher(mt.fn),
		})
	}
	sort.SliceStable(cfg.CustomMatchers, func(i, k int) bool {
		return cfg.CustomMatchers[i].Priority < cfg.CustomMatchers[k].Priority
	})
}

// composeAltActions collapses a list of mark actions into one AltAction (run
// in order); a single action passes through unchanged.
func composeAltActions(acts []tabnasabnf.ActionFn) tabnas.AltAction {
	if len(acts) == 1 {
		return acts[0]
	}
	return func(rule *tabnas.Rule, ctx *tabnas.Context) {
		for _, a := range acts {
			a(rule, ctx)
		}
	}
}

// parseTypeInitErr records a type-registration failure (ADR-005: recorded,
// not panicked) for BuildParseModule to surface.
var parseTypeInitErr error

// TParseGrammar is the carrier type for a Parse.grammar builder: it wraps the
// *tabnas.Tabnas engine + callbacks in an ExtensionPayload the kernel never
// inspects.
var TParseGrammar = registerParseGrammarType()

func registerParseGrammarType() *native.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin("Ideal/ParseGrammar", 5005, nil)
	if err != nil {
		parseTypeInitErr = fmt.Errorf("parse: register Ideal/ParseGrammar: %w", err)
	}
	return t
}

// asParseGrammar unwraps a Grammar carrier, or errors with a clear message.
func asParseGrammar(v native.Value, word string, r *native.Registry) (*parseGrammar, error) {
	ep, ok := v.Data.(eng.ExtensionPayload)
	if ok {
		if g, ok := ep.Body.(*parseGrammar); ok {
			return g, nil
		}
	}
	return nil, r.AqlErrorHint("parse_bad_grammar",
		word+": expected a Parse grammar (from Parse.grammar)", word,
		"build one with `Parse.grammar` first, then chain Parse.abnf / Parse.rule / …")
}

// BuildParseModule creates the "aql:parse" native module.
func BuildParseModule(parent *native.Registry) (native.ModuleDesc, error) {
	if parseTypeInitErr != nil {
		return native.ModuleDesc{}, parseTypeInitErr
	}
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()

	gT := TParseGrammar

	// ---- grammar — mint a fresh builder --------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-grammar",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{gT},
			BarrierPos: -1,
			Handler:    parseGrammarHandler,
		}},
	})
	exports.Set("grammar", wrapMiniFnDef("parse-grammar", [][]native.FnParam{{}},
		[]*native.Type{gT}, nil, subReg))

	// ---- abnf — install an ABNF grammar (deferred to register) ---------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-abnf",
		Signatures: []native.NativeSig{
			{
				Args:       []*native.Type{gT, native.TString, native.TMap},
				Returns:    []*native.Type{},
				BarrierPos: -1,
				Handler:    parseAbnfHandler,
			},
			{
				Args:       []*native.Type{gT, native.TString},
				Returns:    []*native.Type{},
				BarrierPos: -1,
				Handler:    parseAbnfHandler,
			},
		},
	})
	exports.Set("abnf", wrapMiniFnDef("parse-abnf", [][]native.FnParam{
		{{Type: gT}, {Type: native.TString}, {Type: native.TMap}},
		{{Type: gT}, {Type: native.TString}},
	}, []*native.Type{}, nil, subReg))

	// ---- rule — declarative grammar rule (the Map-subtype form) --------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-rule",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{gT, native.TAtom, native.TMap},
			QuoteArgs:  map[int]bool{1: true},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Handler:    parseRuleHandler,
		}},
	})
	exports.Set("rule", wrapMiniFnDef("parse-rule", [][]native.FnParam{
		{{Type: gT}, {Type: native.TAtom, Quote: true}, {Type: native.TMap}},
	}, []*native.Type{}, map[int]bool{1: true}, subReg))

	// ---- token — register a fixed lexer token --------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-token",
		Signatures: []native.NativeSig{{
			// name is a token identifier like "#PL" (special chars) → a String.
			Args:       []*native.Type{gT, native.TString, native.TString},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Handler:    parseTokenHandler,
		}},
	})
	exports.Set("token", wrapMiniFnDef("parse-token", [][]native.FnParam{
		{{Type: gT}, {Type: native.TString}, {Type: native.TString}},
	}, []*native.Type{}, nil, subReg))

	// ---- matcher — register an AQL-fn-backed custom lex matcher --------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-matcher",
		Signatures: []native.NativeSig{{
			Args:          []*native.Type{gT, native.TAtom, native.TInteger, native.TFunction},
			QuoteArgs:     map[int]bool{1: true},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // the fn is stored, not invoked on the tape
			Handler:       parseMatcherHandler,
		}},
	})
	exports.Set("matcher", wrapMiniFnDef("parse-matcher", [][]native.FnParam{
		{{Type: gT}, {Type: native.TAtom, Quote: true}, {Type: native.TInteger}, {Type: native.TFunction}},
	}, []*native.Type{}, map[int]bool{1: true}, subReg))

	// ---- action — attach an AQL-fn-backed semantic action (mark) -------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-action",
		Signatures: []native.NativeSig{{
			Args:          []*native.Type{gT, native.TString, native.TFunction},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // the fn is stored, not invoked on the tape
			Handler:       parseActionHandler,
		}},
	})
	exports.Set("action", wrapMiniFnDef("parse-action", [][]native.FnParam{
		{{Type: gT}, {Type: native.TString}, {Type: native.TFunction}},
	}, []*native.Type{}, nil, subReg))

	// ---- register — finalize + register as a `parse <name>` kind -------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-register",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TAtom, gT},
			QuoteArgs:  map[int]bool{0: true},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Handler:    parseRegisterHandlerFor(parent),
			// Check-mode hook: mark the kind as registered-at-runtime so a later
			// fn body that calls `parse <name>` resolves leniently instead of
			// raising parse_unknown_lang. The grammar (args[1]) is not concrete
			// under analysis, so the real parser is installed only by the Handler
			// at run time; the checker just needs the kind to resolve.
			ReturnsFn: parseRegisterDeferReturns,
		}},
	})
	exports.Set("register", wrapMiniFnDef("parse-register", [][]native.FnParam{
		{{Type: native.TAtom, Quote: true}, {Type: gT}},
	}, []*native.Type{}, map[int]bool{0: true}, subReg))

	// ---- type exports: the grammar-as-Map-subtypes --------------------
	// Parse.RuleSpec / Parse.AltSpec are Map-subtype (Options) type literals
	// mirroring the tabnas GrammarRuleSpec / GrammarAltSpec shape, so a
	// declarative grammar's structure is documented and inspectable as a
	// proper type (e.g. `Parse.AltSpec` renders `options{s:String …}`). The
	// `Parse.rule` converter accepts a plain map of these fields; the types
	// name the schema. Every field is optional.
	exports.Set("AltSpec", parseAltSpecType())
	exports.Set("RuleSpec", parseRuleSpecType())

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"Parse": exports},
	}, nil
}

// parseAltSpecType is the Options type literal for a single grammar
// alternate — the declarative GrammarAltSpec fields a user may set. Returned
// as a structural type-literal Value (a Map subtype), exported as
// Parse.AltSpec.
func parseAltSpecType() native.Value {
	f := native.NewOrderedMap()
	f.Set("s", native.NewTypeLiteral(native.TString)) // token spec, e.g. "#NR #CL"
	f.Set("p", native.NewTypeLiteral(native.TString)) // push rule
	f.Set("r", native.NewTypeLiteral(native.TString)) // replace rule
	f.Set("b", native.NewTypeLiteral(native.TAny))    // backtrack (Integer or rule ref)
	f.Set("a", native.NewTypeLiteral(native.TAny))    // action: a Function or a "@ref" String
	f.Set("g", native.NewTypeLiteral(native.TString)) // group tags
	f.Set("n", native.NewTypeLiteral(native.TMap))    // counter increments
	return native.NewOptionsType(f)
}

// parseRuleSpecType is the Options type literal for a rule — its open / close
// alternate lists (each element an AltSpec map). Exported as Parse.RuleSpec.
func parseRuleSpecType() native.Value {
	f := native.NewOrderedMap()
	f.Set("open", native.NewTypeLiteral(native.TList))
	f.Set("close", native.NewTypeLiteral(native.TList))
	return native.NewOptionsType(f)
}

// ---- handlers --------------------------------------------------------

func parseGrammarHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	g := &parseGrammar{
		j:           tabnas.Make(), // bare engine — the user's grammar is the whole grammar
		markActions: tabnasabnf.ActionsMap{},
	}
	return []native.Value{eng.NewExtension(TParseGrammar, g)}, nil
}

// parseAbnfHandler — args[0]=grammar, args[1]=src, args[2]=opts (optional).
func parseAbnfHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.abnf", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.abnf", r); err != nil {
		return nil, err
	}
	src, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("parse_bad_abnf", fmt.Sprintf("Parse.abnf: src: %v", err), "Parse.abnf")
	}
	var opts native.Value
	if len(args) > 2 {
		opts = args[2]
	}
	o := abnfOptsFrom(opts)
	g.steps = append(g.steps, func() error {
		var acts tabnasabnf.ActionsMap
		if len(g.markActions) > 0 {
			acts = g.markActions
		}
		if _, ierr := tabnasabnf.Install(g.j, src, o, acts); ierr != nil {
			return fmt.Errorf("abnf: %s", native.FirstCleanLine(ierr.Error()))
		}
		return nil
	})
	return nil, nil
}

// parseRuleHandler — args[0]=grammar, args[1]=name (atom), args[2]=spec map.
func parseRuleHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.rule", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.rule", r); err != nil {
		return nil, err
	}
	name, err := args[1].AsConcreteAtom()
	if err != nil {
		return nil, r.AqlError("parse_bad_rule", fmt.Sprintf("Parse.rule: name: %v", err), "Parse.rule")
	}
	gs, err := ruleMapToSpec(g, name, args[2], r)
	if err != nil {
		return nil, err
	}
	g.steps = append(g.steps, func() error {
		if gerr := g.j.Grammar(gs); gerr != nil {
			return fmt.Errorf("rule %q: %v", name, gerr)
		}
		return nil
	})
	return nil, nil
}

// parseTokenHandler — args[0]=grammar, args[1]=name (atom), args[2]=literal.
func parseTokenHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.token", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.token", r); err != nil {
		return nil, err
	}
	name, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("parse_bad_token", fmt.Sprintf("Parse.token: name: %v", err), "Parse.token")
	}
	lit, err := args[2].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("parse_bad_token", fmt.Sprintf("Parse.token: literal: %v", err), "Parse.token")
	}
	g.steps = append(g.steps, func() error {
		g.j.Token(name, lit)
		return nil
	})
	return nil, nil
}

// parseMatcherHandler — args[0]=grammar, args[1]=name, args[2]=priority, args[3]=fn.
func parseMatcherHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.matcher", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.matcher", r); err != nil {
		return nil, err
	}
	name, err := args[1].AsConcreteAtom()
	if err != nil {
		return nil, r.AqlError("parse_bad_matcher", fmt.Sprintf("Parse.matcher: name: %v", err), "Parse.matcher")
	}
	prio, err := args[2].AsConcreteInteger()
	if err != nil {
		return nil, r.AqlError("parse_bad_matcher", fmt.Sprintf("Parse.matcher: priority: %v", err), "Parse.matcher")
	}
	g.matchers = append(g.matchers, pendingMatcher{name: name, prio: int(prio), fn: args[3]})
	return nil, nil
}

// parseActionHandler — args[0]=grammar, args[1]=ref, args[2]=fn.
func parseActionHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.action", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.action", r); err != nil {
		return nil, err
	}
	ref, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("parse_bad_action", fmt.Sprintf("Parse.action: ref: %v", err), "Parse.action")
	}
	if !strings.HasPrefix(ref, "@") {
		return nil, r.AqlErrorHint("parse_bad_action",
			fmt.Sprintf("Parse.action: ref %q must start with '@'", ref), "Parse.action",
			"use @rule:phase (bo/ao/bc/ac) or @rule:o|c:MARK")
	}
	g.markActions[ref] = append(g.markActions[ref], g.wrapAction(args[2]))
	return nil, nil
}

// parseRegisterHandlerFor builds the terminal register word. It finalizes the
// builder (applies the deferred ABNF installs with the now-complete actions)
// and registers a `parse <name>` kind via the aql:parselang host framework.
// parseRegisterDeferReturns is parse-register's check-mode hook: it records the
// kind atom (args[0]) as a deferred parse kind so `parse <name>` resolves during
// analysis (the built grammar in args[1] is not concrete under the checker, so the
// real parser installs only at run time via the Handler). Returns no value, like
// the Handler.
func parseRegisterDeferReturns(args []native.Value, r *native.Registry) []native.Value {
	if len(args) >= 1 && native.IsConcrete(args[0]) {
		if name, err := args[0].AsConcreteAtom(); err == nil {
			native.MarkParseKindDeferred(r, name)
		}
	}
	return nil
}

func parseRegisterHandlerFor(_ *native.Registry) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		name, err := args[0].AsConcreteAtom()
		if err != nil {
			return nil, r.AqlError("parse_bad_name", fmt.Sprintf("Parse.register: %v", err), "Parse.register")
		}
		g, err := asParseGrammar(args[1], "Parse.register", r)
		if err != nil {
			return nil, err
		}
		if err := g.ensureOpen("Parse.register", r); err != nil {
			return nil, err
		}

		// Replay the deferred grammar steps (tokens, rules, ABNF installs) now
		// that every mark action is known, then apply custom matchers LAST
		// (priority-sorted) so a grammar install cannot drop or reorder them.
		for _, step := range g.steps {
			if serr := step(); serr != nil {
				return nil, r.AqlErrorHint("parse_bad_grammar",
					fmt.Sprintf("Parse.register %q: %s", name, serr.Error()),
					"Parse.register", "check the grammar is well-formed")
			}
		}
		g.applyMatchers()

		spec := ParseLangSpec{
			Name:    name,
			Returns: []*native.Type{native.TAny},
			Handler: g.parseHandler(name),
		}
		if rerr := RegisterHostParser(r, spec); rerr != nil {
			return nil, r.AqlError("parse_register_error",
				fmt.Sprintf("Parse.register: %v", rerr), "Parse.register")
		}
		// Single-use: the finalized parser closes over g/g.j; further builder
		// words or a second register must not mutate it.
		g.registered = true
		return nil, nil
	}
}

// parseHandler builds the ParseLangSpec handler that runs the constructed
// parser over a source string and converts the result. It binds g.r for the
// duration of the parse (so callbacks dispatch AQL fns) and surfaces the
// first callback error as a parse error.
func (g *parseGrammar) parseHandler(kind string) native.Handler {
	target := "parse_" + kind
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := args[0].AsConcreteString() // resolved by the framework
		if err != nil {
			return nil, r.AqlError("parse_error", kind+": src: "+err.Error(), target)
		}
		g.firstErr = nil
		g.r = r
		out, perr := g.j.Parse(src)
		g.r = nil
		if g.firstErr != nil {
			return nil, r.AqlErrorHint("parse_syntax_error",
				kind+": "+native.FirstCleanLine(g.firstErr.Error()), target,
				"a grammar action/matcher raised this error")
		}
		if perr != nil {
			return nil, r.AqlErrorHint("parse_syntax_error",
				kind+": "+native.FirstCleanLine(perr.Error()), target,
				"check that the source is well-formed "+kind)
		}
		return []native.Value{native.AnyToValue(out)}, nil
	}
}

// wrapAction wraps an AQL fn as a tabnas semantic action (an AltAction): it
// hands the fn the rule's current node (as an AQL Value), and stores the fn's
// returned Value back on the node — which, because nodes may now BE AQL
// Values (see the native.AnyToValue pass-through), is how an action emits a
// custom-typed value into the parse result.
func (g *parseGrammar) wrapAction(fn native.Value) tabnas.AltAction {
	return func(rule *tabnas.Rule, _ *tabnas.Context) {
		if g.firstErr != nil || g.r == nil {
			return
		}
		node := native.AnyToValue(rule.Node)
		res, err := callParseFn(g.r, fn, []native.Value{node})
		if err != nil {
			g.setErr(err)
			return
		}
		if len(res) > 0 {
			rule.Node = res[len(res)-1]
		}
	}
}

// wrapMatcher wraps an AQL fn as a tabnas custom lex matcher. The fn receives
// the unconsumed source as a String and returns either None (no match) or a
// Map {src:String tin:String? val:Any?}: src is the matched prefix, tin the
// emitted token name (default "#TX" — a text token that flows to the value
// rule), val an optional token value. A malformed return records firstErr.
func (g *parseGrammar) wrapMatcher(fn native.Value) tabnas.LexMatcher {
	return func(lex *tabnas.Lex, _ *tabnas.Rule) *tabnas.Token {
		if g.firstErr != nil || g.r == nil {
			return nil
		}
		cur := lex.Cursor()
		if cur.SI >= len(lex.Src) {
			return nil
		}
		rest := lex.Src[cur.SI:]
		res, err := callParseFn(g.r, fn, []native.Value{native.NewString(rest)})
		if err != nil {
			g.setErr(err)
			return nil
		}
		if len(res) == 0 {
			return nil
		}
		top := res[len(res)-1]
		if !native.IsConcrete(top) || top.Parent.ConformsTo(native.TNone) {
			return nil // no match
		}
		m, _ := native.AsMap(top)
		if m == nil {
			g.setErr(fmt.Errorf("matcher must return None or a {src:String …} map"))
			return nil
		}
		matched, hasSrc := native.MapFieldString(m, "src")
		if !hasSrc || matched == "" {
			g.setErr(fmt.Errorf("matcher result must have a non-empty 'src' String"))
			return nil
		}
		if !strings.HasPrefix(rest, matched) {
			g.setErr(fmt.Errorf("matcher 'src' %q is not a prefix of the unconsumed source", matched))
			return nil
		}
		tinName := "#TX"
		if s, ok := native.MapFieldString(m, "tin"); ok && s != "" {
			tinName = s
		}
		tin := tabnas.TinTX
		if tinName != "#TX" {
			tin = g.j.Token(tinName)
		}
		var val any = matched
		if vv, ok := m.Get("val"); ok {
			val = native.ValueToAny(vv)
		}
		tkn := lex.Token(tinName, tin, val, matched)
		cur.SI += len(matched)
		cur.CI += utf8.RuneCountInString(matched)
		return tkn
	}
}

// callParseFn invokes an AQL function value (a matcher or action callback)
// against args, handling both the compiled-closure and interpreter-FnDef
// shapes — the same seam filter's callback uses (native/filter.go).
func callParseFn(r *native.Registry, fn native.Value, args []native.Value) ([]native.Value, error) {
	if native.IsCompiledClosure(fn) {
		return native.InvokeBody(r, fn, args)
	}
	sig := native.MatchFnSig(fn, args)
	if sig == nil {
		return nil, fmt.Errorf("no matching callback signature")
	}
	var caps []native.CapturedBinding
	if fd, ok := fn.Data.(native.FnDefInfo); ok {
		caps = fd.Captured
	}
	return r.CallAQL(sig, args, caps)
}

// abnfOptsFrom reads the {start tag builtins marks} option map into the
// AbnfConvertOptions. Builtins defaults true (most ABNF grammars reference
// the core rules ALPHA/DIGIT/…); a nil/empty map keeps the defaults.
func abnfOptsFrom(opts native.Value) *tabnasabnf.AbnfConvertOptions {
	o := &tabnasabnf.AbnfConvertOptions{Builtins: true}
	m, _ := native.AsMap(opts)
	if m == nil {
		return o
	}
	if s, ok := native.MapFieldString(m, "start"); ok {
		o.Start = s
	}
	if s, ok := native.MapFieldString(m, "tag"); ok {
		o.Tag = s
	}
	if b, ok := native.MapFieldBoolean(m, "builtins"); ok {
		o.Builtins = b
	}
	if b, ok := native.MapFieldBoolean(m, "marks"); ok {
		o.Marks = b
	}
	return o
}

// ruleMapToSpec converts a declarative rule map into a one-rule GrammarSpec.
// The rule map mirrors tabnas GrammarRuleSpec: {open:[alt…] close:[alt…]},
// each alt an AltSpec map {s p r b a g n}. An inline Function in `a` is wrapped
// as an action and referenced from the spec's Ref table.
func ruleMapToSpec(g *parseGrammar, name string, specV native.Value, r *native.Registry) (*tabnas.GrammarSpec, error) {
	m, _ := native.AsMap(specV)
	if m == nil {
		return nil, r.AqlError("parse_bad_rule",
			fmt.Sprintf("Parse.rule %q: spec must be a {open:[…] close:[…]} map", name), "Parse.rule")
	}
	gs := &tabnas.GrammarSpec{Ref: map[string]any{}}
	rs := &tabnas.GrammarRuleSpec{}

	build := func(key string) (any, error) {
		v, ok := m.Get(key)
		if !ok {
			return nil, nil
		}
		lst, lerr := native.AsList(v)
		if lerr != nil {
			return nil, r.AqlError("parse_bad_rule",
				fmt.Sprintf("Parse.rule %q: %s must be a list of alternates", name, key), "Parse.rule")
		}
		alts := make([]*tabnas.GrammarAltSpec, 0, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			alt, err := altMapToSpec(g, gs, lst.Get(i), name, r)
			if err != nil {
				return nil, err
			}
			alts = append(alts, alt)
		}
		return alts, nil
	}

	open, err := build("open")
	if err != nil {
		return nil, err
	}
	rs.Open = open
	closeAlts, err := build("close")
	if err != nil {
		return nil, err
	}
	rs.Close = closeAlts

	gs.Rule = map[string]*tabnas.GrammarRuleSpec{name: rs}
	return gs, nil
}

// altMapToSpec converts a single alternate map to a GrammarAltSpec.
func altMapToSpec(g *parseGrammar, gs *tabnas.GrammarSpec, altV native.Value, rule string, r *native.Registry) (*tabnas.GrammarAltSpec, error) {
	m, _ := native.AsMap(altV)
	if m == nil {
		return nil, r.AqlError("parse_bad_rule",
			fmt.Sprintf("Parse.rule %q: each alternate must be a map", rule), "Parse.rule")
	}
	alt := &tabnas.GrammarAltSpec{}
	if s, ok := native.MapFieldString(m, "s"); ok {
		alt.S = s
	}
	if s, ok := native.MapFieldString(m, "p"); ok {
		alt.P = s
	}
	if s, ok := native.MapFieldString(m, "r"); ok {
		alt.R = s
	}
	if s, ok := native.MapFieldString(m, "g"); ok {
		alt.G = s
	}
	if bv, ok := m.Get("b"); ok {
		switch {
		case bv.Parent.ConformsTo(native.TInteger) && native.IsConcrete(bv):
			n, _ := bv.AsConcreteInteger()
			alt.B = int(n)
		case bv.Parent.ConformsTo(native.TString) && native.IsConcrete(bv):
			s, _ := bv.AsConcreteString()
			alt.B = s
		}
	}
	if nv, ok := m.Get("n"); ok {
		nm, _ := native.AsMap(nv)
		if nm != nil {
			counters := map[string]int{}
			for _, k := range nm.Keys() {
				cv, _ := nm.Get(k)
				if cv.Parent.ConformsTo(native.TInteger) && native.IsConcrete(cv) {
					ci, _ := cv.AsConcreteInteger()
					counters[k] = int(ci)
				}
			}
			alt.N = counters
		}
	}
	if av, ok := m.Get("a"); ok {
		switch {
		case isCallableValue(av):
			ref := fmt.Sprintf("@__inline_%d", g.inlineSeq)
			g.inlineSeq++
			gs.Ref[ref] = g.wrapAction(av)
			alt.A = ref
		case av.Parent.ConformsTo(native.TString) && native.IsConcrete(av):
			s, _ := av.AsConcreteString()
			alt.A = s
			// A string ref (e.g. '@hit') names an action registered via
			// Parse.action; wire it into the spec's Ref map so tabnas.Grammar
			// can resolve it (only inline fns auto-populate Ref otherwise). An
			// unregistered ref is left alone so tabnas raises "unresolved ref".
			if acts, ok := g.markActions[s]; ok && len(acts) > 0 {
				gs.Ref[s] = composeAltActions(acts)
			}
		}
	}
	return alt, nil
}

// isCallableValue reports whether v is a function-like value (a Function or an
// FnDef) usable as an inline grammar action.
func isCallableValue(v native.Value) bool {
	if v.Parent == native.TFunction {
		return true
	}
	_, ok := v.Data.(native.FnDefInfo)
	return ok
}
