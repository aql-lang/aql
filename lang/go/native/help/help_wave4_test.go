package help

import (
	"errors"
	"strings"
	"testing"
)

// Wave-4 coverage for help.go (Format / FormatDynamic and the example
// generator), help_render.go (module catalog + category rendering),
// help_categories.go (Categories), and overview.go (Overview).

// --- Format (static entry rendering) ---------------------------------------

func TestW4FormatFullEntry(t *testing.T) {
	e := &Entry{
		Word:    "w4-full",
		Summary: "does the full thing",
		Description: "A deliberately long description that must wrap: " +
			strings.Repeat("wrap ", 30),
		Notes:    []string{"first note", "second note"},
		Examples: []string{`w4-full 1 2   ;# 3`},
	}
	out := Format(e)
	for _, want := range []string{
		"w4-full — does the full thing",
		"Description:",
		"Notes:",
		"  - first note",
		"  - second note",
		"Examples:",
		"w4-full 1 2",
		"Docs: " + ReferenceURL,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Format missing %q in:\n%s", want, out)
		}
	}
	// The long description must actually have wrapped.
	if !strings.Contains(out, "\n  wrap") {
		t.Errorf("Format did not wrap the description:\n%s", out)
	}
}

func TestW4FormatMinimalEntry(t *testing.T) {
	out := Format(&Entry{Word: "w4-min"})
	if !strings.Contains(out, "w4-min — <not described>") {
		t.Errorf("minimal entry should show <not described>: %s", out)
	}
	if strings.Contains(out, "Notes:") || strings.Contains(out, "Description:") {
		t.Errorf("minimal entry should have no Notes/Description blocks: %s", out)
	}
}

// --- FormatDynamic ----------------------------------------------------------

func w4Sig(args []string, rets []string) SigInfo {
	return SigInfo{Args: args, Returns: rets}
}

func TestW4FormatDynamicForwardNonCommutative(t *testing.T) {
	// "sub" is in the non-commutative table: FormatDynamic must show the
	// mirror configurations, the order-matters note, and the swap-form
	// example alongside the forward form.
	info := FuncInfo{
		Name:        "sub",
		ForwardArgs: true,
		Sigs: []SigInfo{
			w4Sig([]string{"Scalar/Number/Integer", "Scalar/Number/Integer"}, []string{"Integer"}),
		},
		Entry: Lookup("sub"),
	}
	out := FormatDynamic(info)
	for _, want := range []string{
		"Precedence: forward",
		"sub x y  <=>  y sub x  <=>  y x sub",
		"Order matters: the swap form `x sub y`",
		"Signatures: (in match order)",
		"[Integer Integer]",
		"Docs: " + ReferenceURL,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatDynamic(sub) missing %q in:\n%s", want, out)
		}
	}
}

func TestW4FormatDynamicStackOnly(t *testing.T) {
	info := FuncInfo{
		Name:        "w4-stacky",
		ForwardArgs: false,
		Sigs:        []SigInfo{w4Sig([]string{"Integer", "Integer"}, []string{"Integer"})},
	}
	out := FormatDynamic(info)
	if !strings.Contains(out, "Precedence: stack") {
		t.Errorf("expected stack precedence header:\n%s", out)
	}
	// Stack layout: all args precede the word, deepest first.
	if !strings.Contains(out, "y x w4-stacky") {
		t.Errorf("expected stack layout example:\n%s", out)
	}
	// Stack-only words get the all-prefix example expression.
	if !strings.Contains(out, "3 2 w4-stacky") {
		t.Errorf("expected all-stack example expr:\n%s", out)
	}
}

func TestW4FormatDynamicModuleExport(t *testing.T) {
	info := FuncInfo{
		Name:        "ArrayUtil.w4-fake",
		ForwardArgs: true,
		Module:      "aql:array-util",
		Doc:         "a module export summary",
		Sigs:        []SigInfo{w4Sig([]string{"List"}, []string{"List"})},
	}
	out := FormatDynamic(info)
	if !strings.Contains(out, "ArrayUtil.w4-fake — a module export summary") {
		t.Errorf("module Doc should be the summary:\n%s", out)
	}
	if !strings.Contains(out, "Module: aql:array-util") {
		t.Errorf("expected Module provenance line:\n%s", out)
	}
	// With no Entry, the Doc doubles as the description.
	if !strings.Contains(out, "a module export summary\n") {
		t.Errorf("Doc should back the description:\n%s", out)
	}
}

func TestW4FormatDynamicNoSigs(t *testing.T) {
	out := FormatDynamic(FuncInfo{Name: "w4-none", ForwardArgs: true})
	if !strings.Contains(out, "(none)") {
		t.Errorf("no sigs should render (none):\n%s", out)
	}
	if !strings.Contains(out, "(no examples)") {
		t.Errorf("no sigs should render (no examples):\n%s", out)
	}
	if !strings.Contains(out, "<not described>") {
		t.Errorf("no entry should render <not described>:\n%s", out)
	}
}

func TestW4FormatDynamicHandAuthoredExamplesWin(t *testing.T) {
	info := FuncInfo{
		Name:        "w4-authored",
		ForwardArgs: true,
		Sigs:        []SigInfo{w4Sig([]string{"Integer"}, []string{"Integer"})},
		Entry: &Entry{
			Word:     "w4-authored",
			Summary:  "sum",
			Notes:    []string{"note-a"},
			Examples: []string{`w4-authored 9 ;# hand`},
		},
	}
	out := FormatDynamic(info)
	if !strings.Contains(out, "w4-authored 9 ;# hand") {
		t.Errorf("hand-authored example missing:\n%s", out)
	}
	// Auto-generated permutations must NOT appear alongside.
	if strings.Contains(out, ";# ...") {
		t.Errorf("auto examples should be replaced by authored ones:\n%s", out)
	}
	if !strings.Contains(out, "  - note-a") {
		t.Errorf("notes missing:\n%s", out)
	}
}

func TestW4FormatDynamicExoticTypesSkipExamples(t *testing.T) {
	// Argument types without representative literals must not fabricate
	// examples.
	info := FuncInfo{
		Name:        "w4-exotic",
		ForwardArgs: true,
		Sigs:        []SigInfo{w4Sig([]string{"Ideal/Function"}, []string{"Any"})},
	}
	out := FormatDynamic(info)
	if !strings.Contains(out, "(no examples)") {
		t.Errorf("exotic arg types should yield no examples:\n%s", out)
	}
}

func TestW4FormatDynamicWideSigClamped(t *testing.T) {
	// >3 args clamps the precedence examples at 3 vars.
	info := FuncInfo{
		Name:        "w4-wide",
		ForwardArgs: true,
		Sigs: []SigInfo{w4Sig(
			[]string{"Integer", "Integer", "Integer", "Integer"},
			[]string{"Integer"})},
	}
	out := FormatDynamic(info)
	if !strings.Contains(out, "w4-wide x y z") {
		t.Errorf("expected clamped 3-var precedence example:\n%s", out)
	}
	// Stack-only wide word takes the clamped stack rendering path.
	info.ForwardArgs = false
	out = FormatDynamic(info)
	if !strings.Contains(out, "z y x w4-wide") {
		t.Errorf("expected clamped stack precedence example:\n%s", out)
	}
}

func TestW4FormatDynamicZeroArgSig(t *testing.T) {
	// A 0-arg signature: no precedence example lines, no examples.
	info := FuncInfo{
		Name:        "w4-nullary",
		ForwardArgs: true,
		Sigs:        []SigInfo{w4Sig(nil, []string{"Integer"})},
	}
	out := FormatDynamic(info)
	if !strings.Contains(out, "(no examples)") {
		t.Errorf("nullary word should have no examples:\n%s", out)
	}
	info.ForwardArgs = false
	out = FormatDynamic(info)
	if !strings.Contains(out, "Precedence: stack") {
		t.Errorf("expected stack header for nullary stack word:\n%s", out)
	}
}

// --- example machinery ------------------------------------------------------

func TestW4ExampleExprs(t *testing.T) {
	// Non-commutative forward 2-arg word: forward + swap forms.
	sub := FuncInfo{
		Name:        "sub",
		ForwardArgs: true,
		Sigs:        []SigInfo{w4Sig([]string{"Integer", "Integer"}, []string{"Integer"})},
	}
	exprs := ExampleExprs(sub)
	if len(exprs) != 2 {
		t.Fatalf("ExampleExprs(sub) = %v, want 2 exprs", exprs)
	}
	if exprs[0] != "sub 2 3" || exprs[1] != "3 sub 2" {
		t.Errorf("ExampleExprs(sub) = %v, want [sub 2 3, 3 sub 2]", exprs)
	}

	// Commutative forward word: forward form only.
	add := FuncInfo{
		Name:        "w4-plus",
		ForwardArgs: true,
		Sigs:        []SigInfo{w4Sig([]string{"Integer", "Integer"}, []string{"Integer"})},
	}
	if exprs := ExampleExprs(add); len(exprs) != 1 || exprs[0] != "w4-plus 2 3" {
		t.Errorf("ExampleExprs(w4-plus) = %v, want [w4-plus 2 3]", exprs)
	}

	// Stack-only word: the all-prefix form.
	st := FuncInfo{
		Name: "w4-st",
		Sigs: []SigInfo{w4Sig([]string{"Integer", "Integer"}, []string{"Integer"})},
	}
	if exprs := ExampleExprs(st); len(exprs) != 1 || exprs[0] != "3 2 w4-st" {
		t.Errorf("ExampleExprs(w4-st) = %v, want [3 2 w4-st]", exprs)
	}

	// 0-arg and exotic sigs generate nothing; duplicate sigs dedupe.
	none := FuncInfo{
		Name:        "w4-none",
		ForwardArgs: true,
		Sigs: []SigInfo{
			w4Sig(nil, nil),
			w4Sig([]string{"Ideal/Function"}, nil),
		},
	}
	if exprs := ExampleExprs(none); len(exprs) != 0 {
		t.Errorf("ExampleExprs(w4-none) = %v, want none", exprs)
	}
	dup := FuncInfo{
		Name:        "w4-dup",
		ForwardArgs: true,
		Sigs: []SigInfo{
			w4Sig([]string{"Boolean"}, nil),
			w4Sig([]string{"Boolean"}, nil),
		},
	}
	// Shared counters advance across sigs, so the two sigs yield distinct
	// example values — assert dedupe only on identical exprs.
	exprs = ExampleExprs(dup)
	seen := map[string]bool{}
	for _, e := range exprs {
		if seen[e] {
			t.Errorf("ExampleExprs(w4-dup) returned duplicate %q", e)
		}
		seen[e] = true
	}
}

func TestW4GenerateDynamicExamples(t *testing.T) {
	info := FuncInfo{
		Name:        "w4-dyn",
		ForwardArgs: true,
		Sigs:        []SigInfo{w4Sig([]string{"Integer", "Integer"}, []string{"Integer"})},
	}
	// nil eval is a no-op.
	GenerateDynamicExamples(info, nil)
	if _, ok := dynamicExampleResults["w4-dyn 2 3"]; ok {
		t.Fatal("nil eval must not populate results")
	}
	// A failing eval stores nothing.
	GenerateDynamicExamples(info, func(string) (string, error) {
		return "", errors.New("boom")
	})
	if _, ok := dynamicExampleResults["w4-dyn 2 3"]; ok {
		t.Fatal("failing eval must not populate results")
	}
	// A successful eval stores the result; FormatDynamic then shows it.
	calls := 0
	GenerateDynamicExamples(info, func(expr string) (string, error) {
		calls++
		return "42", nil
	})
	if got := dynamicExampleResults["w4-dyn 2 3"]; got != "42" {
		t.Errorf("dynamic result = %q, want 42", got)
	}
	out := FormatDynamic(info)
	if !strings.Contains(out, "w4-dyn 2 3") || !strings.Contains(out, ";# 42") {
		t.Errorf("FormatDynamic should show the dynamic result:\n%s", out)
	}
	// Re-generating skips exprs already in the dynamic map.
	GenerateDynamicExamples(info, func(expr string) (string, error) {
		t.Errorf("eval re-invoked for cached expr %q", expr)
		return "", nil
	})
	// An expr already in the STATIC map is skipped too ("add 2 3" is
	// pre-computed by the generator).
	if _, ok := exampleResults["add 2 3"]; ok {
		addInfo := FuncInfo{
			Name:        "add",
			ForwardArgs: true,
			Sigs:        []SigInfo{w4Sig([]string{"Integer", "Integer"}, []string{"Integer"})},
		}
		GenerateDynamicExamples(addInfo, func(expr string) (string, error) {
			if expr == "add 2 3" {
				t.Errorf("eval invoked for statically pre-computed expr")
			}
			return "", errors.New("skip")
		})
	}
}

func TestW4ComputeBinaryResult(t *testing.T) {
	sigInt := w4Sig([]string{"Integer", "Integer"}, []string{"Integer"})
	sigFloat := w4Sig([]string{"Float", "Float"}, []string{"Float"})
	cases := []struct {
		name string
		sig  SigInfo
		a, b string
		want string
	}{
		{"add", sigInt, "2", "3", "5"},
		{"sub", sigInt, "2", "3", "1"}, // b - a
		{"mul", sigInt, "2", "3", "6"}, //
		{"div", sigInt, "2", "6", "3"}, // b / a
		{"div", sigInt, "0", "6", "error"},
		{"mod", sigInt, "2", "7", "1"}, // b mod a
		{"mod", sigInt, "0", "7", "error"},
		{"pow", sigInt, "2", "3", "..."},
		{"min", sigInt, "2", "3", "2"},
		{"max", sigInt, "2", "3", "3"},
		{"w4-unknown", sigInt, "2", "3", "..."},
		{"add", sigFloat, "2.5", "3.25", "5.75"},
		{"add", w4Sig([]string{"String", "String"}, []string{"String"}), "'a'", "'b'", "'ba'"},
		// two non-String scalars: add does not concatenate (Boolean
		// arithmetic is a defined error), so the heuristic yields "...".
		{"add", w4Sig([]string{"Boolean", "Boolean"}, []string{"Boolean"}), "true", "false", "..."},
		{"mul", sigInt, "'a'", "'b'", "..."},
	}
	for _, c := range cases {
		if got := computeBinaryResult(c.name, c.sig, c.a, c.b); got != c.want {
			t.Errorf("computeBinaryResult(%s, %s, %s) = %q, want %q",
				c.name, c.a, c.b, got, c.want)
		}
	}
}

func TestW4ComputeUnaryResult(t *testing.T) {
	sigInt := w4Sig([]string{"Integer"}, []string{"Integer"})
	cases := []struct {
		name string
		a    string
		want string
	}{
		{"abs", "-4", "4"},
		{"abs", "4", "4"},
		{"negate", "4", "-4"},
		{"sign", "9", "1"},
		{"sign", "-9", "-1"},
		{"sign", "0", "0"},
		{"w4-unary-unknown", "4", "..."},
		{"abs", "'x'", "..."},
	}
	for _, c := range cases {
		if got := computeUnaryResult(c.name, sigInt, c.a); got != c.want {
			t.Errorf("computeUnaryResult(%s, %s) = %q, want %q", c.name, c.a, got, c.want)
		}
	}
}

func TestW4ComputeExampleResultArity(t *testing.T) {
	sig := w4Sig(nil, nil)
	if got := computeExampleResult("anything", sig, nil); got != "..." {
		t.Errorf("0-arg computeExampleResult = %q, want ...", got)
	}
	if got := computeExampleResult("abs", w4Sig([]string{"Integer"}, []string{"Integer"}),
		[]string{"-2"}); got != "2" {
		t.Errorf("1-arg computeExampleResult = %q, want 2", got)
	}
}

func TestW4FormatResult(t *testing.T) {
	intSig := w4Sig(nil, []string{"Integer"})
	floatSig := w4Sig(nil, []string{"Scalar/Number/Float"})
	if got := formatResult(7, intSig); got != "7" {
		t.Errorf("formatResult int = %q", got)
	}
	if got := formatResult(7.5, floatSig); got != "7.5" {
		t.Errorf("formatResult float = %q", got)
	}
	if got := formatResult(7, w4Sig(nil, nil)); got != "..." {
		t.Errorf("formatResult no returns = %q", got)
	}
}

func TestW4ParseNum(t *testing.T) {
	if f, ok := parseNum("3.5"); !ok || f != 3.5 {
		t.Errorf("parseNum(3.5) = %v, %v", f, ok)
	}
	if f, ok := parseNum("'2'"); !ok || f != 2 {
		t.Errorf("parseNum('2') = %v, %v", f, ok)
	}
	if _, ok := parseNum("'abc'"); ok {
		t.Error("parseNum(abc) should fail")
	}
}

func TestW4BuildExampleExpr(t *testing.T) {
	vals := []string{"a", "b", "c"}
	cases := []struct {
		prefix int
		want   string
	}{
		{0, "w a b c"},
		{1, "c w a b"},
		{2, "c b w a"},
		{3, "c b a w"},
	}
	for _, cse := range cases {
		if got := buildExampleExpr("w", vals, cse.prefix, 3); got != cse.want {
			t.Errorf("buildExampleExpr(prefix=%d) = %q, want %q", cse.prefix, got, cse.want)
		}
	}
	args := buildSigArgs(vals, 1, 3)
	if len(args) != 3 || args[0] != "a" || args[2] != "c" {
		t.Errorf("buildSigArgs = %v, want [a b c]", args)
	}
}

func TestW4ExampleValCyclesAndFallback(t *testing.T) {
	c := 0
	if v := exampleVal("Scalar/Number/Integer", &c); v != "2" {
		t.Errorf("first Integer example = %q, want 2", v)
	}
	if v := exampleVal("Integer", &c); v != "3" {
		t.Errorf("second Integer example = %q, want 3", v)
	}
	// Unknown leaf falls back to Any's values.
	c = 0
	if v := exampleVal("Weird/Unknown", &c); v != "2" {
		t.Errorf("unknown type example = %q, want Any fallback 2", v)
	}
}

func TestW4IsNonCommutative2Arg(t *testing.T) {
	two := []SigInfo{w4Sig([]string{"Integer", "Integer"}, nil)}
	one := []SigInfo{w4Sig([]string{"Integer"}, nil)}
	if !isNonCommutative2Arg(FuncInfo{Name: "lt", Sigs: two}) {
		t.Error("lt should be non-commutative")
	}
	if isNonCommutative2Arg(FuncInfo{Name: "add", Sigs: two}) {
		t.Error("add should be commutative")
	}
	if isNonCommutative2Arg(FuncInfo{Name: "sub", Sigs: one}) {
		t.Error("1-arg-only word cannot be non-commutative-2arg")
	}
}

func TestW4TypeAbbrev(t *testing.T) {
	if got := typeAbbrev("Scalar/Number/Integer"); got != "Integer" {
		t.Errorf("typeAbbrev = %q", got)
	}
	if got := typeAbbrev("List"); got != "List" {
		t.Errorf("typeAbbrev bare = %q", got)
	}
}

// --- help_render.go / help_categories.go / overview.go ----------------------

func TestW4ModuleCatalog(t *testing.T) {
	cat := ModuleCatalog()
	if len(cat) == 0 {
		t.Fatal("empty module catalog")
	}
	for i := 1; i < len(cat); i++ {
		if cat[i-1].Name > cat[i].Name {
			t.Errorf("catalog unsorted: %q > %q", cat[i-1].Name, cat[i].Name)
		}
	}
	if s := ModuleSummary("log"); !strings.Contains(s, "logging") {
		t.Errorf("ModuleSummary(log) = %q", s)
	}
	if s := ModuleSummary("not-a-module"); s != "" {
		t.Errorf("ModuleSummary(not-a-module) = %q, want empty", s)
	}
}

func TestW4WriteWordsByCategory(t *testing.T) {
	var b strings.Builder
	WriteWordsByCategory(&b)
	out := b.String()
	for _, want := range []string{"math", "add", "storage", "help"} {
		if !strings.Contains(out, want) {
			t.Errorf("WriteWordsByCategory missing %q", want)
		}
	}
	// The math word list is long enough that the grid must have wrapped.
	if strings.Count(out, "\n") < len(Categories())*2 {
		t.Errorf("expected wrapped word grids:\n%s", out)
	}
}

func TestW4WriteModuleCatalog(t *testing.T) {
	var b strings.Builder
	WriteModuleCatalog(&b)
	out := b.String()
	if !strings.Contains(out, "math-util") || !strings.Contains(out, "log") {
		t.Errorf("WriteModuleCatalog missing modules:\n%s", out)
	}
}

func TestW4WriteCategory(t *testing.T) {
	cat, ok := LookupCategory("compare")
	if !ok {
		t.Fatal("compare category missing")
	}
	var b strings.Builder
	WriteCategory(&b, cat)
	out := b.String()
	if !strings.Contains(out, "compare — ") {
		t.Errorf("missing category header:\n%s", out)
	}
	if !strings.Contains(out, "lt") || !strings.Contains(out, "cmp") {
		t.Errorf("missing category words:\n%s", out)
	}
	// A word with a registered entry shows its summary.
	if e := Lookup("lt"); e != nil && e.Summary != "" {
		if !strings.Contains(out, e.Summary) {
			t.Errorf("missing summary %q:\n%s", e.Summary, out)
		}
	}
	// An unregistered word renders with an empty summary, not a crash.
	var b2 strings.Builder
	WriteCategory(&b2, Category{Name: "w4cat", Summary: "s", Words: []string{"w4-not-registered"}})
	if !strings.Contains(b2.String(), "w4-not-registered") {
		t.Errorf("unregistered word missing:\n%s", b2.String())
	}
}

func TestW4Categories(t *testing.T) {
	cats := Categories()
	if len(cats) == 0 {
		t.Fatal("Categories() empty")
	}
	if cats[0].Name != "math" {
		t.Errorf("first category = %q, want math (display order)", cats[0].Name)
	}
}

func TestW4Pad(t *testing.T) {
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q", got)
	}
	if got := pad("abcdef", 3); got != "abcdef" {
		t.Errorf("pad long = %q", got)
	}
}

func TestW4Overview(t *testing.T) {
	out := Overview()
	for _, want := range []string{
		"AQL — a concatenative query language.",
		"describe add",
		TutorialURL,
		ReferenceURL,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Overview missing %q", want)
		}
	}
}

func TestW4FormatDynamicKeywordSlot(t *testing.T) {
	// A signature with a keyword slot (position 1) renders the literal
	// `fn/q` instead of the bare `Atom`; the other positions keep their
	// abbreviated types. This is the def-form display fix.
	info := FuncInfo{
		Name:        "w4-kwdef",
		ForwardArgs: true,
		Sigs: []SigInfo{{
			Args:     []string{"Scalar/Atom", "Scalar/Atom", "Node/List"},
			Returns:  []string{"Any"},
			Keywords: map[int]string{1: "fn/q"},
		}},
	}
	out := FormatDynamic(info)
	if !strings.Contains(out, "[Atom fn/q List]") {
		t.Errorf("keyword slot must render as fn/q:\n%s", out)
	}
}

func TestW4FormatDynamicNoKeywordStaysBare(t *testing.T) {
	// Negative: a sig without Keywords renders every atom slot bare.
	info := FuncInfo{
		Name: "w4-plaindef",
		Sigs: []SigInfo{w4Sig([]string{"Scalar/Atom", "Any"}, []string{"Any"})},
	}
	out := FormatDynamic(info)
	if !strings.Contains(out, "[Atom Any]") {
		t.Errorf("plain atom slot must stay bare:\n%s", out)
	}
}

// WriteCategory must render module-provided words under the import that
// binds them — in BOTH shapes a category can take.
//
// `math` has core words AND module words, so the module block is preceded
// by a blank separator; `string` has ONLY module words, so the "Words:"
// header is skipped entirely and no leading blank line is emitted. Both
// arms matter: a reader who is told `upper` is a built-in and then gets
// undefined_word has been actively misled, which is what the pre-split
// table did for 80 words.
func TestWriteCategoryRendersModuleWords(t *testing.T) {
	mixed, ok := LookupCategory("math")
	if !ok {
		t.Fatal("math category missing")
	}
	var b strings.Builder
	WriteCategory(&b, mixed)
	out := b.String()
	if !strings.Contains(out, "Words:") {
		t.Errorf("mixed category lost its core-word header:\n%s", out)
	}
	if !strings.Contains(out, "import \"aql:math-util\"") {
		t.Errorf("mixed category does not name the import:\n%s", out)
	}
	if !strings.Contains(out, "add") || !strings.Contains(out, "sqrt") {
		t.Errorf("mixed category lost words:\n%s", out)
	}

	moduleOnly, ok := LookupCategory("string")
	if !ok {
		t.Fatal("string category missing")
	}
	var b2 strings.Builder
	WriteCategory(&b2, moduleOnly)
	out2 := b2.String()
	if strings.Contains(out2, "Words:") {
		t.Errorf("module-only category should not print a core-word header:\n%s", out2)
	}
	if !strings.Contains(out2, "import \"aql:string-util\"") {
		t.Errorf("module-only category does not name the import:\n%s", out2)
	}
	if !strings.Contains(out2, "upper") {
		t.Errorf("module-only category lost its words:\n%s", out2)
	}
	// No leading blank line when there is no core-word block above it.
	if strings.HasPrefix(out2[strings.Index(out2, "\n")+1:], "\n\n") {
		t.Errorf("module-only category emitted a doubled blank line:\n%q", out2)
	}
}

// The index listing shows the same import annotation, so `aql describe`
// with no argument is self-sufficient.
func TestWriteWordsByCategoryAnnotatesModules(t *testing.T) {
	var b strings.Builder
	WriteWordsByCategory(&b)
	out := b.String()
	for _, want := range []string{
		"(import \"aql:math-util\")",
		"(import \"aql:string-util\")",
		"(import \"aql:bin-util\")",
		"(import \"aql:logic-util\")",
		"(import \"aql:type-util\")",
		"(import \"aql:query\")",
		"(import \"aql:io\")",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("index listing missing %s:\n%s", want, out)
		}
	}
}
