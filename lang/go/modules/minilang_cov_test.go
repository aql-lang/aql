package modules

import (
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// mcovReg builds a registry with the module resolver wired.
func mcovReg(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)
	return r
}

func mcovRun(t *testing.T, r *native.Registry, src string) []native.Value {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := native.NewTop(r).Run(vals)
	if err != nil {
		t.Fatalf("run: %v\n--- src ---\n%s", err, src)
	}
	return out
}

func mcovErr(t *testing.T, r *native.Registry, src string) error {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, rerr := native.NewTop(r).Run(vals)
	return rerr
}

const mcovImp = `import "aql:minilang"  `

// TestMiniCovHexBytes drives the hb kind: grouping, size, and the loud
// odd-length / non-hex rejections.
func TestMiniCovHexBytes(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+`mini hb 'de_ad be_ef' size`)
	if n, err := out[0].AsConcreteInteger(); err != nil || n != 4 {
		t.Errorf("hb size = %v, want 4 (err %v)", out[0], err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`mini hb 'xyz'`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("non-hex source should be mini_parse_error, got %v", err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`mini hb 'abc'`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("odd-length source should be mini_parse_error, got %v", err)
	}
}

// TestMiniCovBinBytes drives the bb kind: value equivalence with hb plus
// the bit-count and bad-digit rejections.
func TestMiniCovBinBytes(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+`(mini bb '01001100') (mini hb '4c') eq`)
	if b, err := out[0].AsConcreteBoolean(); err != nil || !b {
		t.Errorf("bb '01001100' should equal hb '4c', got %v (err %v)", out[0], err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`mini bb '0100110'`); err == nil ||
		!strings.Contains(err.Error(), "not a multiple of 8") {
		t.Errorf("7 bits should be rejected, got %v", err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`mini bb '01001102'`); err == nil ||
		!strings.Contains(err.Error(), "not a binary digit") {
		t.Errorf("'2' should be rejected, got %v", err)
	}
}

// TestMiniCovRe drives both re paths — the expansion-time compile hook
// (mini re → run-re) and the plain transducer (MiniLang.lang_re) — plus
// opts, capture groups, and the no-match shape.
func TestMiniCovRe(t *testing.T) {
	r := mcovReg(t)

	// Hook path (mini re) and transducer path (lang_re) agree.
	hook := mcovRun(t, r, mcovImp+`('AbcD' mini re '[a-z]+').fst.m`)
	if s, _ := hook[0].AsConcreteString(); s != "bc" {
		t.Errorf("mini re fst.m = %v, want bc", hook[0])
	}
	trans := mcovRun(t, r, mcovImp+`('AbcD' MiniLang.lang_re '[a-z]+' {} end).fst.m`)
	if s, _ := trans[0].AsConcreteString(); s != "bc" {
		t.Errorf("lang_re fst.m = %v, want bc", trans[0])
	}

	// opts.limit caps matches on both paths.
	lim := mcovRun(t, r, mcovImp+`('a1b2c3' mini re '\\d' {limit:2}).n`)
	if n, _ := lim[0].AsConcreteInteger(); n != 2 {
		t.Errorf("limit:2 n = %v, want 2", lim[0])
	}

	// Unmatched capture groups render as none.
	grp := mcovRun(t, r, mcovImp+`('ab' mini re '(a)(x)?(b)').fst.g`)
	gl, glErr := native.AsList(grp[0])
	if glErr != nil || gl.Len() != 3 || !native.IsNone(gl.Get(1)) {
		t.Errorf("groups = %v, want ['a' none 'b'] (err %v)", grp[0], glErr)
	}

	// No match: ok=false, n=0, no fst.
	nm := mcovRun(t, r, mcovImp+`('abc' mini re 'z').ok`)
	if b, _ := nm[0].AsConcreteBoolean(); b {
		t.Errorf("no-match ok = %v, want false", nm[0])
	}
}

// TestMiniCovReNegatives pins the loud failures of both re paths: a
// malformed pattern (hook defers to the transducer's error) and a
// non-Integer limit.
func TestMiniCovReNegatives(t *testing.T) {
	if err := mcovErr(t, mcovReg(t), mcovImp+`'x' mini re '('`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("bad pattern via mini re should be mini_parse_error, got %v", err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`'x' MiniLang.lang_re '(' {} end`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("bad pattern via lang_re should be mini_parse_error, got %v", err)
	}
	// run-re path with a bad opts.limit.
	if err := mcovErr(t, mcovReg(t), mcovImp+`'x' mini re 'x' {limit:'z'}`); err == nil ||
		!strings.Contains(err.Error(), "opts.limit") {
		t.Errorf("bad limit (run-re) should name opts.limit, got %v", err)
	}
	// lang_re path with a bad opts.limit.
	if err := mcovErr(t, mcovReg(t), mcovImp+`'x' MiniLang.lang_re 'x' {limit:'z'} end`); err == nil ||
		!strings.Contains(err.Error(), "opts.limit") {
		t.Errorf("bad limit (lang_re) should name opts.limit, got %v", err)
	}
}

// TestMiniCovBf drives the brainfuck kind: filter form (stack input),
// generator form (opts.in), the `,`-at-EOF zero, and a loop program.
func TestMiniCovBf(t *testing.T) {
	r := mcovReg(t)
	gen := mcovRun(t, r, mcovImp+`mini bf ',+.' {in:'A'}`)
	if s, _ := gen[0].AsConcreteString(); s != "B" {
		t.Errorf("generator form = %v, want B", gen[0])
	}
	fil := mcovRun(t, r, mcovImp+`'A' mini bf ',+.'`)
	if s, _ := fil[0].AsConcreteString(); s != "B" {
		t.Errorf("filter form = %v, want B", fil[0])
	}
	eof := mcovRun(t, r, mcovImp+`(mini bf ',+.') size`)
	if n, _ := eof[0].AsConcreteInteger(); n != 1 {
		t.Errorf("EOF read should still emit one byte, got %v", eof[0])
	}
	loop := mcovRun(t, r, mcovImp+`mini bf '++++++++[>++++++++<-]>+.'`)
	if s, _ := loop[0].AsConcreteString(); s != "A" {
		t.Errorf("loop program = %v, want A (8*8+1=65)", loop[0])
	}
}

// TestMiniCovBfNegatives pins runBrainfuck's rejections: unbalanced
// brackets (both directions), the step budget, tape underrun, and the
// opts type errors.
func TestMiniCovBfNegatives(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"unbalanced close", `mini bf ']'`, "unbalanced ']'"},
		{"unbalanced open", `mini bf '['`, "unbalanced '['"},
		{"step budget", `mini bf '+[]' {steps:10}`, "step budget"},
		{"tape underrun", `mini bf '<'`, "pointer out of range"},
		{"bad steps opt", `mini bf '+.' {steps:'x'}`, "opts.steps"},
		{"bad in opt", `mini bf ',.' {in:5}`, "opts.in"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := mcovErr(t, mcovReg(t), mcovImp+c.src)
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// TestMiniCovMicronBuiltin drives the no-registrations micron fast path
// (Emailon / Pathon dispatch) and its parse failure.
func TestMiniCovMicronBuiltin(t *testing.T) {
	r := mcovReg(t)
	em := mcovRun(t, r, mcovImp+`mini m 'alice@example.com' typeof`)
	if em[0].String() != "Emailon" {
		t.Errorf("email literal typeof = %s, want Emailon", em[0].String())
	}
	pa := mcovRun(t, r, mcovImp+`mini micron 'a/b' typeof`)
	if pa[0].String() != "Pathon" {
		t.Errorf("path literal typeof = %s, want Pathon", pa[0].String())
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+`mini m 'a b'`); err == nil ||
		!strings.Contains(err.Error(), "mini_parse_error") {
		t.Errorf("whitespace source should be mini_parse_error, got %v", err)
	}
}

// TestMiniCovMicronUserKind drives the user-Micron literal machinery:
// token-only registration claims its span, the builder's instance carries
// properties, builtin leaves keep their spans, and a gate mismatch falls
// through to Pathon.
func TestMiniCovMicronUserKind(t *testing.T) {
	reg := mcovImp + `def Ticketon refine Micron {id:String}  ` +
		`MiniLang.micron Ticketon {options:{match:{token:{'#TK':'@/T-[0-9]+/'}}}} ([s:String] => [make Ticketon {id:s}])  end  `
	r := mcovReg(t)
	out := mcovRun(t, r, reg+`mini m 'T-123' typeof`)
	if out[0].String() != "Ticketon" {
		t.Errorf("registered kind typeof = %s, want Ticketon", out[0].String())
	}
	r2 := mcovReg(t)
	out2 := mcovRun(t, r2, reg+`(mini m 'T-123').id`)
	if s, _ := out2[0].AsConcreteString(); s != "T-123" {
		t.Errorf("builder instance .id = %v, want T-123", out2[0])
	}
	r3 := mcovReg(t)
	out3 := mcovRun(t, r3, reg+`mini m 'alice@example.com' typeof`)
	if out3[0].String() != "Emailon" {
		t.Errorf("builtin leaf should keep its span, got %s", out3[0].String())
	}
	r4 := mcovReg(t)
	out4 := mcovRun(t, r4, reg+`mini m 'T-nope' typeof`)
	if out4[0].String() != "Pathon" {
		t.Errorf("gate mismatch should fall to Pathon, got %s", out4[0].String())
	}
}

// TestMiniCovMicronRuledGrammar covers the RULED registration paths: a
// gated val alternate with a ref action (the userA arm) and a plain gated
// alternate defaulting to the matched span.
func TestMiniCovMicronRuledGrammar(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+`def Depton refine Micron {team:String}  `+
		`MiniLang.micron Depton {options:{match:{token:{'#DP':'@/D-[a-z]+/'}}} rule:{val:{open:[{s:'#DP' a:'@team'}]}} ref:{'@team': ([nd:Any] => ['ops'])}} ([t:String] => [make Depton {team:t}])  end  `+
		`(mini m 'D-x').team`)
	if s, _ := out[0].AsConcreteString(); s != "ops" {
		t.Errorf("ruled grammar with ref action: .team = %v, want ops", out[0])
	}

	r2 := mcovReg(t)
	out2 := mcovRun(t, r2, mcovImp+`def Depton refine Micron {team:String}  `+
		`MiniLang.micron Depton {options:{match:{token:{'#DP':'@/D-[a-z]+/'}}} rule:{val:{open:[{s:'#DP'}]}}} ([s:String] => [make Depton {team:s}])  end  `+
		`(mini m 'D-ops').team`)
	if s, _ := out2[0].AsConcreteString(); s != "D-ops" {
		t.Errorf("plain gated alternate should default to the span, got %v", out2[0])
	}
}

// TestMiniCovMicronBuilderForm covers the aql:parse builder form of
// MiniLang.micron — a Parse.grammar value consumed like Parse.parser
// consumes one.
func TestMiniCovMicronBuilderForm(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+`import "aql:parse"  def Baron refine Micron {v:String}  `+
		`def g (Parse.grammar)  Parse.spec g {options:{match:{token:{'#BX':'@/B[0-9]+/'}}}}  `+
		`MiniLang.micron Baron g ([s:String] => [make Baron {v:s}])  end  mini m 'B77' typeof`)
	if out[0].String() != "Baron" {
		t.Errorf("builder-form kind typeof = %s, want Baron", out[0].String())
	}
}

// TestMiniCovMicronBuilderNegatives pins the builder-form refusals:
// a consumed builder, a builder carrying its own tag, and a builder
// whose gate token collides with a builtin leaf (caught at finalize —
// the builder form bypasses the map-decidable check).
func TestMiniCovMicronBuilderNegatives(t *testing.T) {
	pre := mcovImp + `import "aql:parse"  import "aql:parselang"  def Baron refine Micron {v:String}  `
	fn := ` ([s:String] => [make Baron {v:s}])`

	if err := mcovErr(t, mcovReg(t), pre+
		`def g (Parse.grammar)  Parse.abnf g 'x = "a"' {start:'x'}  def used (Parse.parser g)  end  `+
		`MiniLang.micron Baron g`+fn); err == nil ||
		!strings.Contains(err.Error(), "parse_grammar_done") {
		t.Errorf("a consumed builder should be refused, got %v", err)
	}

	if err := mcovErr(t, mcovReg(t), pre+
		`def g (Parse.grammar)  Parse.spec g {options:{tag:'x' match:{token:{'#BX':'@/B[0-9]+/'}}}}  `+
		`MiniLang.micron Baron g`+fn); err == nil ||
		!strings.Contains(err.Error(), "must not set options.tag") {
		t.Errorf("a tagged builder should be refused, got %v", err)
	}

	if err := mcovErr(t, mcovReg(t), pre+
		`def g (Parse.grammar)  Parse.spec g {options:{match:{token:{'#EMAILON':'@/x/'}}}}  `+
		`MiniLang.micron Baron g`+fn); err == nil ||
		!strings.Contains(err.Error(), "collides with Emailon") {
		t.Errorf("a builtin-token builder should collide at finalize, got %v", err)
	}
}

// TestMiniCovMicronBuilderContract pins the post-parse Build rules: a
// builder that returns the wrong shape and a ref action that raises
// during the parse are both loud.
func TestMiniCovMicronBuilderContract(t *testing.T) {
	// Wrong instance back from the builder.
	if err := mcovErr(t, mcovReg(t), mcovImp+`def Ticketon refine Micron {id:String}  `+
		`MiniLang.micron Ticketon {options:{match:{token:{'#TK':'@/T-[0-9]+/'}}}} ([s:String] => [42])  end  `+
		`mini m 'T-1'`); err == nil ||
		!strings.Contains(err.Error(), "must return one Ticketon instance") {
		t.Errorf("a non-conforming builder result should be loud, got %v", err)
	}

	// A ref action raising mid-parse surfaces via the claimed-span rule.
	if err := mcovErr(t, mcovReg(t), mcovImp+`def Depton refine Micron {team:String}  `+
		`MiniLang.micron Depton {options:{match:{token:{'#DP':'@/D-[a-z]+/'}}} rule:{val:{open:[{s:'#DP' a:'@team'}]}} ref:{'@team': ([nd:Any] => [raise 'cbboom'])}} ([t:String] => [make Depton {team:t}])  end  `+
		`mini m 'D-x'`); err == nil || !strings.Contains(err.Error(), "cbboom") {
		t.Errorf("a raising grammar callback should surface loudly, got %v", err)
	}

	// The empty literal keeps riding the builtin fast path even with
	// registrations present.
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+`def Ticketon refine Micron {id:String}  `+
		`MiniLang.micron Ticketon {options:{match:{token:{'#TK':'@/T-[0-9]+/'}}}} ([s:String] => [make Ticketon {id:s}])  end  `+
		`mini m '' typeof`)
	if out[0].String() != "Pathon" {
		t.Errorf("empty literal should stay the empty relative path, got %s", out[0].String())
	}
}

// TestMiniCovMicronNegatives pins MiniLang.micron's refusals.
func TestMiniCovMicronNegatives(t *testing.T) {
	mk := `def Baron refine Micron {v:Integer}  `
	fn := ` ([s:String] => [make Baron {v:1}])`
	cases := []struct{ name, src, want string }{
		{"builtin kind refused", mcovImp +
			`MiniLang.micron Emailon {options:{match:{token:{'#T':'@/x/'}}}} ([s:String] => [s])`,
			"builtin Micron"},
		{"non-Micron kind refused", mcovImp +
			`MiniLang.micron Integer {options:{match:{token:{'#T':'@/x/'}}}} ([s:String] => [s])`,
			"expected a Micron kind"},
		{"retired string form", mcovImp + mk +
			`MiniLang.micron Baron 'B[0-9]+'` + fn, "expected a grammar"},
		{"no match token", mcovImp + mk +
			`MiniLang.micron Baron {options:{}}` + fn, "at least one match token"},
		{"options.tag forbidden", mcovImp + mk +
			`MiniLang.micron Baron {options:{tag:'x' match:{token:{'#T':'@/x/'}}}}` + fn,
			"must not set options.tag"},
		{"builtin token collision", mcovImp + mk +
			`MiniLang.micron Baron {options:{match:{token:{'#PATHON':'@/x/'}}}}` + fn,
			"collides with a builtin leaf"},
		{"duplicate registration", mcovImp + mk +
			`MiniLang.micron Baron {options:{match:{token:{'#B1':'@/b/'}}}}` + fn + ` end ` +
			`MiniLang.micron Baron {options:{match:{token:{'#B2':'@/c/'}}}}` + fn,
			"already registered"},
		{"cross-kind token collision", mcovImp + mk + `def Caron refine Micron {v:Integer}  ` +
			`MiniLang.micron Baron {options:{match:{token:{'#TK':'@/b/'}}}}` + fn + ` end ` +
			`MiniLang.micron Caron {options:{match:{token:{'#TK':'@/c/'}}}} ([s:String] => [make Caron {v:1}])`,
			"collides with Baron"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := mcovErr(t, mcovReg(t), c.src)
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// TestMiniCovRegister drives MiniLang.register: a generator kind (named
// params via opts), a filter kind (stack subject + minted member type),
// and MiniLang.kinds listing them.
func TestMiniCovRegister(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+
		`MiniLang.register poly (fn [[src:String opts:Map] [Integer] [((opts.x pow 2) add (3 mul opts.y))]]) end  `+
		`mini poly 'x^2 + 3*y' {x:10, y:2}`)
	if n, _ := out[0].AsConcreteInteger(); n != 106 {
		t.Errorf("generator kind = %v, want 106", out[0])
	}

	r2 := mcovReg(t)
	out2 := mcovRun(t, r2, mcovImp+
		`MiniLang.register count (fn [[src:String opts:Map subject:String] [Integer] [def m (subject mini re src)  m.n]]) end  `+
		`"banana" mini count 'an'`)
	if n, _ := out2[0].AsConcreteInteger(); n != 2 {
		t.Errorf("filter kind = %v, want 2", out2[0])
	}
	// A filter-shaped kind mints its member type export.
	mt := mcovRun(t, r2, `MiniLang.Count`)
	if mt[0].String() != "Count" {
		t.Errorf("MiniLang.Count = %s, want the minted member type", mt[0].String())
	}
	// kinds lists the new kind after the builtins.
	kinds := mcovRun(t, r2, `MiniLang.kinds`)
	if !strings.Contains(kinds[0].String(), "count") || !strings.Contains(kinds[0].String(), "re") {
		t.Errorf("kinds = %s, want re … count", kinds[0].String())
	}
}

// TestMiniCovRegisterNegatives pins the register contract.
func TestMiniCovRegisterNegatives(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"collision with builtin", mcovImp +
			`MiniLang.register re (fn [[s:String o:Map] [Integer] [1]])`, "mini_kind_exists"},
		{"loop re-register collides", mcovImp +
			`for 2 [MiniLang.register cnt (fn [[src:String opts:Map subject:String] [Integer] [1]]) end]`,
			"mini_kind_exists"},
		{"missing standard prefix", mcovImp +
			`MiniLang.register bad (fn [[a:Integer] [Integer] [a]])`, "mini_bad_signature"},
		{"lang_ prefix rejected", mcovImp +
			`MiniLang.register lang_x (fn [[s:String o:Map] [Integer] [1]])`, "mini_bad_name"},
		{"capitalised name rejected", mcovImp +
			`MiniLang.register Bad (fn [[s:String o:Map] [Integer] [1]])`, "mini_bad_name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := mcovErr(t, mcovReg(t), c.src)
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// TestMiniCovRegisterCompiled drives MiniLang.register-compiled: the
// macro hook rewrites the call site; the contract failures are loud.
func TestMiniCovRegisterCompiled(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+`import "aql:string-util"  `+
		`MiniLang.register up (fn [[s:String o:Map] [String] [s]]) end  `+
		`MiniLang.register-compiled up (macro [[src opts] [ quote [ unquote (StringUtil.upper src) ] ]]) end  `+
		`mini up 'hi'`)
	if s, _ := out[0].AsConcreteString(); s != "HI" {
		t.Errorf("compiled kind = %v, want HI", out[0])
	}

	if err := mcovErr(t, mcovReg(t), mcovImp+
		`MiniLang.register-compiled ghost (macro [[s o] [ quote [ s ] ]])`); err == nil ||
		!strings.Contains(err.Error(), "mini_no_transducer") {
		t.Errorf("compiler without transducer should be mini_no_transducer, got %v", err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+
		`MiniLang.register lit (fn [[s:String o:Map] [String] [s]]) end  `+
		`MiniLang.register-compiled lit (fn [[s:String o:Map] [List] [ [s] ]])`); err == nil ||
		!strings.Contains(err.Error(), "mini_bad_compiler") {
		t.Errorf("non-macro compiler should be mini_bad_compiler, got %v", err)
	}
	if err := mcovErr(t, mcovReg(t), mcovImp+
		`MiniLang.register-compiled Bad (macro [[s o] [ quote [ s ] ]])`); err == nil ||
		!strings.Contains(err.Error(), "mini_bad_name") {
		t.Errorf("capitalised compiled name should be mini_bad_name, got %v", err)
	}
}

// mcovHostSpec builds a Go host kind with one stack input: it returns
// the subject's length plus the length of src.
func mcovHostSpec(name string) MiniLangSpec {
	return MiniLangSpec{
		Name:    name,
		Inputs:  []native.FnParam{{Type: native.TString}},
		Returns: []*native.Type{native.TInteger},
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
			src, err := args[0].AsConcreteString()
			if err != nil {
				return nil, err
			}
			subj, err := args[2].AsConcreteString()
			if err != nil {
				return nil, err
			}
			return []native.Value{native.NewInteger(int64(len(src) + len(subj)))}, nil
		},
	}
}

// TestMiniCovHostRegister drives RegisterHostMiniLang / installHostMiniLang:
// pre-import and post-import registration both dispatch, and the contract
// failures are loud.
func TestMiniCovHostRegister(t *testing.T) {
	// Pre-import registration.
	r := mcovReg(t)
	if err := RegisterHostMiniLang(r, mcovHostSpec("hlen")); err != nil {
		t.Fatalf("RegisterHostMiniLang: %v", err)
	}
	out := mcovRun(t, r, mcovImp+`'hello' mini hlen 'xy'`)
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Errorf("host kind = %v, want 7 (2+5)", out[0])
	}

	// Post-import registration injects into the live module.
	r2 := mcovReg(t)
	mcovRun(t, r2, mcovImp+`0 drop`)
	if err := RegisterHostMiniLang(r2, mcovHostSpec("hlen2")); err != nil {
		t.Fatalf("post-import RegisterHostMiniLang: %v", err)
	}
	out2 := mcovRun(t, r2, `'abc' mini hlen2 'q'`)
	if n, _ := out2[0].AsConcreteInteger(); n != 4 {
		t.Errorf("post-import host kind = %v, want 4", out2[0])
	}

	// Contract refusals.
	r3 := mcovReg(t)
	if err := RegisterHostMiniLang(r3, MiniLangSpec{Name: "", Handler: mcovHostSpec("x").Handler}); err == nil {
		t.Error("empty name must be refused")
	}
	if err := RegisterHostMiniLang(r3, MiniLangSpec{Name: "lang_x", Handler: mcovHostSpec("x").Handler}); err == nil {
		t.Error("lang_ prefix must be refused")
	}
	if err := RegisterHostMiniLang(r3, MiniLangSpec{Name: "Xup", Handler: mcovHostSpec("x").Handler}); err == nil {
		t.Error("capitalised name must be refused")
	}
	if err := RegisterHostMiniLang(r3, MiniLangSpec{Name: "nohandler"}); err == nil {
		t.Error("nil handler must be refused")
	}
	if err := RegisterHostMiniLang(r3, mcovHostSpec("dup")); err != nil {
		t.Fatalf("first dup register: %v", err)
	}
	if err := RegisterHostMiniLang(r3, mcovHostSpec("dup")); err == nil {
		t.Error("duplicate host kind must be refused")
	}

	// Collision with a built-in kind: pre-import defers to the import.
	r4 := mcovReg(t)
	if err := RegisterHostMiniLang(r4, mcovHostSpec("re")); err != nil {
		t.Fatalf("pre-import collision should defer to build: %v", err)
	}
	if err := mcovErr(t, r4, mcovImp+`1`); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Errorf("import should fail on the host/builtin collision, got %v", err)
	}
	// Post-import: the live-inject path catches it immediately.
	r5 := mcovReg(t)
	mcovRun(t, r5, mcovImp+`0 drop`)
	if err := RegisterHostMiniLang(r5, mcovHostSpec("bf")); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Errorf("post-import builtin collision should refuse, got %v", err)
	}
}

// TestMiniCovHostCompileHook covers the spec.Compile wiring: the hook
// runs at the call site and its splice dispatches the standard call.
func TestMiniCovHostCompileHook(t *testing.T) {
	r := mcovReg(t)
	spec := mcovHostSpec("hck")
	spec.Compile = func(src string, opts native.Value, _ *native.Registry) ([]native.Value, error) {
		// Defer to the standard transducer call — the same shape
		// miniReCompileFor splices on a malformed pattern.
		return []native.Value{
			native.NewWord("MiniLang"), native.NewWord("dot"), native.NewWord("lang_hck"),
			native.NewString(src), opts, native.NewEnd(),
		}, nil
	}
	if err := RegisterHostMiniLang(r, spec); err != nil {
		t.Fatalf("RegisterHostMiniLang with Compile: %v", err)
	}
	out := mcovRun(t, r, mcovImp+`'hello' mini hck 'xy'`)
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Errorf("compile-hook kind = %v, want 7", out[0])
	}
}

// TestMiniCovOptHelpersDirect pins miniOptInt / miniOptString /
// miniDropGrouping / miniValidKindName / miniCompiledPattern directly.
func TestMiniCovOptHelpersDirect(t *testing.T) {
	// Non-map opts fall back to the default.
	if n, err := miniOptInt(native.NewInteger(3), "k", 42); err != nil || n != 42 {
		t.Errorf("miniOptInt(non-map) = %d,%v want 42,nil", n, err)
	}
	if s, err := miniOptString(native.NewInteger(3), "k"); err != nil || s != "" {
		t.Errorf("miniOptString(non-map) = %q,%v want \"\",nil", s, err)
	}
	m := native.NewOrderedMap()
	m.Set("i", native.NewInteger(7))
	m.Set("s", native.NewString("v"))
	m.Set("bad", native.NewBoolean(true))
	opts := native.NewMap(m)
	if n, err := miniOptInt(opts, "i", 0); err != nil || n != 7 {
		t.Errorf("miniOptInt present = %d,%v want 7,nil", n, err)
	}
	if n, err := miniOptInt(opts, "absent", 9); err != nil || n != 9 {
		t.Errorf("miniOptInt absent = %d,%v want 9,nil", n, err)
	}
	if _, err := miniOptInt(opts, "bad", 0); err == nil {
		t.Error("miniOptInt should refuse a non-Integer value")
	}
	if s, err := miniOptString(opts, "s"); err != nil || s != "v" {
		t.Errorf("miniOptString present = %q,%v want v,nil", s, err)
	}
	if s, err := miniOptString(opts, "absent"); err != nil || s != "" {
		t.Errorf("miniOptString absent = %q,%v", s, err)
	}
	if _, err := miniOptString(opts, "bad"); err == nil {
		t.Error("miniOptString should refuse a non-String value")
	}

	if got := miniDropGrouping("de_ad be\tef\r\n"); got != "deadbeef" {
		t.Errorf("miniDropGrouping = %q, want deadbeef", got)
	}

	for _, c := range []struct{ name, wantSub string }{
		{"", "must not be empty"},
		{"lang_x", "lang_ prefix"},
		{"Xyz", "lowercase"},
		{"ok", ""},
	} {
		got := miniValidKindName(c.name)
		if c.wantSub == "" && got != "" {
			t.Errorf("miniValidKindName(%q) = %q, want valid", c.name, got)
		}
		if c.wantSub != "" && !strings.Contains(got, c.wantSub) {
			t.Errorf("miniValidKindName(%q) = %q, want %q", c.name, got, c.wantSub)
		}
	}

	re1, err := miniCompiledPattern("cov-[a-z]+")
	if err != nil {
		t.Fatalf("miniCompiledPattern: %v", err)
	}
	re2, _ := miniCompiledPattern("cov-[a-z]+")
	if re1 != re2 {
		t.Error("miniCompiledPattern should memoize by source")
	}
	if _, err := miniCompiledPattern("("); err == nil {
		t.Error("miniCompiledPattern should surface a compile error")
	}
}

// TestMiniCovShapeReturnsDirect covers the check-mode shape hooks for re
// and gex.
func TestMiniCovShapeReturnsDirect(t *testing.T) {
	if out := miniReShapeReturns(nil, nil); len(out) != 1 {
		t.Errorf("miniReShapeReturns returned %d values, want 1", len(out))
	}
	dummy := native.NewInteger(0)
	cases := []struct {
		name    string
		subject native.Value
	}{
		{"list subject", native.NewList([]native.Value{native.NewInteger(1)})},
		{"map subject", native.NewMap(native.NewOrderedMap())},
		{"scalar subject", native.NewString("x")},
		{"any literal subject", native.NewTypeLiteral(native.TAny)},
	}
	for _, c := range cases {
		out := miniGexShapeReturns([]native.Value{dummy, dummy, c.subject}, nil)
		if len(out) != 1 {
			t.Errorf("%s: returned %d values, want 1", c.name, len(out))
		}
	}
	// Arity guard.
	if out := miniGexShapeReturns([]native.Value{dummy}, nil); len(out) != 1 {
		t.Errorf("short args: returned %d values, want 1", len(out))
	}
}

// mcovFilterFn builds a Function value with the standard 3-param filter
// signature (src, opts, subject) for the check-mode register hooks.
func mcovFilterFn() native.Value {
	return native.NewFunction(native.FnDefInfo{
		Name: "covfn",
		Signatures: []native.FnSig{{
			Params: []native.FnParam{
				{Type: native.TString}, {Type: native.TMap}, {Type: native.TString},
			},
		}},
	})
}

// TestMiniCovRegisterReturnsDirect drives miniRegisterReturns' arms: nil
// registry, the compile-pass growth note, the pure-check install, and the
// non-concrete guards.
func TestMiniCovRegisterReturnsDirect(t *testing.T) {
	owner := mcovReg(t)
	exports := native.NewOrderedMap()
	eng.RegisterModuleExportGrowth(owner, exports)
	idents := map[string]registerIdent{}
	var minted []string
	mint := func(kind string) { minted = append(minted, kind) }
	hook := miniRegisterReturns(exports, idents, mint, owner)

	args := []native.Value{native.NewAtom("covk"), mcovFilterFn()}

	// nil registry → no-op.
	if out := hook(args, nil); out != nil {
		t.Errorf("nil registry should return nil, got %v", out)
	}

	done := owner.Check.Begin()
	defer done()

	// Compile pass: notes growth, installs nothing.
	owner.Check.Compiling = true
	hook(args, owner)
	if _, ok := exports.Get("lang_covk"); ok {
		t.Error("compile pass must not install the kind")
	}
	owner.Check.Compiling = false

	// Non-concrete name / non-fn value guards.
	hook([]native.Value{native.NewTypeLiteral(native.TAtom), mcovFilterFn()}, owner)
	hook([]native.Value{native.NewAtom("covk"), native.NewInteger(1)}, owner)
	if _, ok := exports.Get("lang_covk"); ok {
		t.Error("guarded args must not install the kind")
	}

	// Pure check: installs and mints the filter member type.
	hook(args, owner)
	if _, ok := exports.Get("lang_covk"); !ok {
		t.Error("pure check should install lang_covk")
	}
	if len(minted) != 1 || minted[0] != "covk" {
		t.Errorf("filter-shaped kind should mint its member type, got %v", minted)
	}
}

// TestMiniCovGrowthNotesDirect drives noteMiniRegisterGrowth and
// miniRegisterCompiledReturns' arms directly.
func TestMiniCovGrowthNotesDirect(t *testing.T) {
	owner := mcovReg(t)
	exports := native.NewOrderedMap()
	eng.RegisterModuleExportGrowth(owner, exports)

	// Short args: no-op.
	noteMiniRegisterGrowth(owner, exports, []native.Value{native.NewAtom("x")})
	// Non-concrete name poisons.
	noteMiniRegisterGrowth(owner, exports, []native.Value{
		native.NewTypeLiteral(native.TAtom), mcovFilterFn()})
	// Filter-shaped fn notes both keys.
	noteMiniRegisterGrowth(owner, exports, []native.Value{
		native.NewAtom("flt"), mcovFilterFn()})
	// Provably non-filter fn notes only lang_.
	genFn := native.NewFunction(native.FnDefInfo{
		Name: "gen",
		Signatures: []native.FnSig{{
			Params: []native.FnParam{{Type: native.TString}, {Type: native.TMap}},
		}},
	})
	noteMiniRegisterGrowth(owner, exports, []native.Value{native.NewAtom("gen"), genFn})

	hook := miniRegisterCompiledReturns(owner, exports)
	if out := hook([]native.Value{native.NewAtom("x")}, nil); out != nil {
		t.Errorf("nil registry should return nil, got %v", out)
	}
	if out := hook([]native.Value{native.NewAtom("x")}, owner); out != nil {
		t.Errorf("concrete name should return nil after noting, got %v", out)
	}
	if out := hook([]native.Value{native.NewTypeLiteral(native.TAtom)}, owner); out != nil {
		t.Errorf("non-concrete name should return nil after poisoning, got %v", out)
	}
}

// TestMiniCovRunReDirect pins run-re's carrier guards: a non-extension
// value and an extension wrapping the wrong body are both refused.
func TestMiniCovRunReDirect(t *testing.T) {
	r := mcovReg(t)
	tMini := r.Types.MintType("CovCompiled", native.TIdeal)
	args := []native.Value{native.NewInteger(5), native.NewMap(native.NewOrderedMap()), native.NewString("x")}
	if _, err := miniRunReHandler(args, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not a compiled pattern") {
		t.Errorf("a non-extension carrier should be refused, got %v", err)
	}
	args[0] = eng.NewExtension(tMini, "not-a-regexp")
	if _, err := miniRunReHandler(args, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not a compiled pattern") {
		t.Errorf("a wrong-body extension should be refused, got %v", err)
	}
}

// TestMiniCovStateAccessorsDirect pins the create=false arms of the two
// per-registry state accessors.
func TestMiniCovStateAccessorsDirect(t *testing.T) {
	r := mcovReg(t)
	if s := miniLangHostStateFor(r, false); s != nil {
		t.Errorf("no host state expected on a fresh registry, got %v", s)
	}
	if s := micronLitStateFor(r, false); s != nil {
		t.Errorf("no micron-lit state expected on a fresh registry, got %v", s)
	}
	// create=true mints once and then returns the same state.
	s1 := miniLangHostStateFor(r, true)
	s2 := miniLangHostStateFor(r, false)
	if s1 == nil || s1 != s2 {
		t.Error("host state should be minted once and shared")
	}
}

// mcovTokenGrammarMap builds {options:{match:{token:{tok:pat}}}} as
// concrete ordered maps for the direct check-mode tests.
func mcovTokenGrammarMap(tok, pat string) native.Value {
	tokens := native.NewOrderedMap()
	tokens.Set(tok, native.NewString(pat))
	match := native.NewOrderedMap()
	match.Set("token", native.NewMap(tokens))
	options := native.NewOrderedMap()
	options.Set("match", native.NewMap(match))
	spec := native.NewOrderedMap()
	spec.Set("options", native.NewMap(options))
	return native.NewMap(spec)
}

// TestMiniCovMicronLitReturnsDirect drives MiniLang.micron's check-mode
// hook: a non-Micron kind and the retired String shape both surface
// diagnostics with the runtime message.
func TestMiniCovMicronLitReturnsDirect(t *testing.T) {
	r := mcovReg(t)
	done := r.Check.Begin()
	defer done()

	fn := mcovFilterFn()

	// Non-Micron kind.
	out := miniMicronLitReturns([]native.Value{
		native.NewTypeLiteral(native.TInteger), native.NewString("x"), fn}, r)
	if len(out) != 0 {
		t.Errorf("expected no returns, got %v", out)
	}
	if len(r.Check.Diagnostics) == 0 {
		t.Fatal("a non-Micron kind should add a diagnostic")
	}
	if r.Check.Diagnostics[0].Code != "micron_literal" {
		t.Errorf("diagnostic code = %s, want micron_literal", r.Check.Diagnostics[0].Code)
	}

	// A user Micron kind with the RETIRED string shape.
	userKind := r.Types.MintType("Covon", native.TMicron)
	kindV := native.NewTypeLiteral(userKind)
	before := len(r.Check.Diagnostics)
	miniMicronLitReturns([]native.Value{kindV, native.NewString("B[0-9]+"), fn}, r)
	if len(r.Check.Diagnostics) != before+1 {
		t.Fatalf("retired string shape should add one diagnostic, got %+v", r.Check.Diagnostics[before:])
	}
	if !strings.Contains(r.Check.Diagnostics[before].Detail, "expected a grammar") {
		t.Errorf("diagnostic %q should carry the grammar migration message", r.Check.Diagnostics[before].Detail)
	}

	// A Map shape with no gate token flags via micronSpecMapCheck.
	empty := native.NewOrderedMap()
	before2 := len(r.Check.Diagnostics)
	miniMicronLitReturns([]native.Value{kindV, native.NewMap(empty), fn}, r)
	if len(r.Check.Diagnostics) != before2+1 {
		t.Fatalf("token-less grammar map should add one diagnostic")
	}
	if !strings.Contains(r.Check.Diagnostics[before2].Detail, "match token") {
		t.Errorf("diagnostic %q should name the missing match token", r.Check.Diagnostics[before2].Detail)
	}

	// A well-formed grammar map dry-replays its steps cleanly.
	before3 := len(r.Check.Diagnostics)
	miniMicronLitReturns([]native.Value{kindV, mcovTokenGrammarMap("#TK", "@/T-[0-9]+/"), fn}, r)
	if len(r.Check.Diagnostics) != before3 {
		t.Errorf("a valid grammar map should add no diagnostics, got %+v", r.Check.Diagnostics[before3:])
	}

	// An invalid token regexp flags at the dry replay with the runtime
	// message.
	before4 := len(r.Check.Diagnostics)
	miniMicronLitReturns([]native.Value{kindV, mcovTokenGrammarMap("#TK", "@/T-[0-9+/"), fn}, r)
	if len(r.Check.Diagnostics) != before4+1 {
		t.Fatalf("an invalid token regexp should add one diagnostic, got %+v", r.Check.Diagnostics[before4:])
	}

	// A non-concrete options section is skipped leniently.
	lenientSpec := native.NewOrderedMap()
	lenientSpec.Set("options", native.NewTypeLiteral(native.TMap))
	before5 := len(r.Check.Diagnostics)
	miniMicronLitReturns([]native.Value{kindV, native.NewMap(lenientSpec), fn}, r)
	if len(r.Check.Diagnostics) != before5 {
		t.Errorf("a non-concrete options section must be skipped, got %+v", r.Check.Diagnostics[before5:])
	}
}
