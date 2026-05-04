package nativemod

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

func decisionRegistry(t *testing.T) *engine.Registry {
	t.Helper()
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	if err := InstallDecisionExports(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func runDecisionAQL(t *testing.T, r *engine.Registry, src string) []engine.Value {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	eng := engine.NewTop(r)
	result, err := eng.Run(values)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	return result
}

// --- Module structure ---

func TestDecisionModuleExports(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	desc, err := BuildDecisionModule(r)
	if err != nil {
		t.Fatal(err)
	}
	decExport, ok := desc.Exports["decision"]
	if !ok {
		t.Fatal("expected 'decision' export")
	}
	for _, name := range []string{"cond", "all-of", "any-of", "not-of", "make-rule", "make-table",
		"eval-cond", "eval-pred", "eval-table", "eval-tree", "decide"} {
		if _, ok := decExport.Get(name); !ok {
			t.Errorf("missing export: %q", name)
		}
	}
}

// --- Condition builder ---

func TestDecisionCond(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `18 "gte" quote age decision.cond`)
	m := result[0].AsMap()
	if m == nil {
		t.Fatalf("expected map, got %s", result[0].VType.String())
	}
	field, _ := m.Get("field")
	if field.String() != "age" {
		t.Errorf("field = %v, want age", field)
	}
}

// --- Condition evaluation ---
// Convention: condition nearest to word, input further.
// e.g. {input} {condition} decision.eval-cond

func TestDecisionEvalCondTrue(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `{age:25} {field:"age",op:"gte",value:18} decision.eval-cond`)
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected true for age=25 gte 18")
	}
}

func TestDecisionEvalCondFalse(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `{age:15} {field:"age",op:"gte",value:18} decision.eval-cond`)
	b, _ := result[0].AsBoolean()
	if b {
		t.Error("expected false for age=15 gte 18")
	}
}

func TestDecisionEvalCondEq(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `{status:"active"} {field:"status",op:"eq",value:"active"} decision.eval-cond`)
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected true for status eq active")
	}
}

// --- Predicate evaluation ---

func TestDecisionEvalPredAllOf(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def pred ([{field:"age",op:"gte",value:18} {field:"score",op:"gt",value:50}] decision.all-of)
		{age:25,score:80} pred decision.eval-pred
	`)
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected all-of true for age=25,score=80")
	}
}

func TestDecisionEvalPredAllOfFalse(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def pred ([{field:"age",op:"gte",value:18} {field:"score",op:"gt",value:50}] decision.all-of)
		{age:25,score:30} pred decision.eval-pred
	`)
	b, _ := result[0].AsBoolean()
	if b {
		t.Error("expected all-of false for age=25,score=30")
	}
}

func TestDecisionEvalPredAnyOf(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def pred ([{field:"age",op:"gte",value:18} {field:"score",op:"gt",value:50}] decision.any-of)
		{age:10,score:80} pred decision.eval-pred
	`)
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected any-of true for age=10,score=80")
	}
}

func TestDecisionEvalPredNotOf(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def pred ({field:"age",op:"lt",value:18} decision.not-of)
		{age:25} pred decision.eval-pred
	`)
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected not-of(age lt 18) true for age=25")
	}
}

// --- Decision table ---

func TestDecisionTableFirst(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def tbl ([
			{when:{field:"age",op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:"age",op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)
		{age:25} tbl decision.eval-table
	`)
	m := result[0].AsMap()
	cat, _ := m.Get("category")
	s, _ := cat.AsString()
	if s != "adult" {
		t.Errorf("expected adult, got %v", cat)
	}
}

func TestDecisionTableFirstMinor(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def tbl ([
			{when:{field:"age",op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:"age",op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)
		{age:12} tbl decision.eval-table
	`)
	m := result[0].AsMap()
	cat, _ := m.Get("category")
	s, _ := cat.AsString()
	if s != "minor" {
		t.Errorf("expected minor, got %v", cat)
	}
}

func TestDecisionTableUnique(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def rawtbl ([
			{when:{field:"score",op:"lt",value:50}, then:{grade:"fail"}}
			{when:{field:"score",op:"gte",value:50}, then:{grade:"pass"}}
		] decision.make-table)
		def tbl (rawtbl "unique" decision.with-policy)
		{score:75} tbl decision.eval-table
	`)
	m := result[0].AsMap()
	grade, _ := m.Get("grade")
	s, _ := grade.AsString()
	if s != "pass" {
		t.Errorf("expected pass, got %v", grade)
	}
}

func TestDecisionTableCollect(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def rawtbl ([
			{when:{field:"age",op:"gte",value:18}, then:{perk:"vote"}}
			{when:{field:"age",op:"gte",value:21}, then:{perk:"drink"}}
		] decision.make-table)
		def tbl (rawtbl "collect" decision.with-policy)
		{age:25} tbl decision.eval-table
	`)
	list := result[0].AsList()
	if list.Len() != 2 {
		t.Fatalf("expected 2 collected, got %d: %v", list.Len(), result[0])
	}
}

func TestDecisionTableNoMatch(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def tbl ([{when:{field:"age",op:"gt",value:100}, then:{x:1}}] decision.make-table)
		{age:25} tbl decision.eval-table
	`)
	m := result[0].AsMap()
	errVal, _ := m.Get("error")
	s, _ := errVal.AsString()
	if s != "no-match" {
		t.Errorf("expected no-match error, got %v", result[0])
	}
}

func TestDecisionTableCompound(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def tbl ({kind:"table", hit-policy:"first", rules:[
			{when:{kind:"group",op:"all",children:[{field:"age",op:"lt",value:30} {field:"score",op:"gte",value:90}]}, then:{tier:"premium"}}
			{when:{field:"score",op:"gte",value:50}, then:{tier:"standard"}}
		]})
		{age:25,score:95} tbl decision.eval-table
	`)
	m := result[0].AsMap()
	tier, _ := m.Get("tier")
	s, _ := tier.AsString()
	if s != "premium" {
		t.Errorf("expected premium, got %v", tier)
	}
}

// --- Decision tree ---

func TestDecisionTree(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def tree ({kind:"tree", root:"root", nodes:[
			{id:"root", kind:"branch", branches:[
				{when:{field:"age",op:"lt",value:18}, next:"minor"}
				{when:{field:"age",op:"gte",value:18}, next:"adult"}
			]}
			{id:"minor", kind:"leaf", result:{category:"minor"}}
			{id:"adult", kind:"leaf", result:{category:"adult"}}
		]})
		{age:25} tree decision.eval-tree
	`)
	m := result[0].AsMap()
	cat, _ := m.Get("category")
	s, _ := cat.AsString()
	if s != "adult" {
		t.Errorf("expected adult, got %v", cat)
	}
}

func TestDecisionTreeMinor(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def tree ({kind:"tree", root:"root", nodes:[
			{id:"root", kind:"branch", branches:[
				{when:{field:"age",op:"lt",value:18}, next:"minor"}
				{when:{field:"age",op:"gte",value:18}, next:"adult"}
			]}
			{id:"minor", kind:"leaf", result:"too-young"}
			{id:"adult", kind:"leaf", result:"welcome"}
		]})
		{age:12} tree decision.eval-tree
	`)
	s, _ := result[0].AsString()
	if s != "too-young" {
		t.Errorf("expected too-young, got %v", result[0])
	}
}

func TestDecisionTreeMultiLevel(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def tree ({kind:"tree", root:"check-age", nodes:[
			{id:"check-age", kind:"branch", branches:[
				{when:{field:"age",op:"lt",value:18}, next:"reject"}
				{when:{field:"age",op:"gte",value:18}, next:"check-score"}
			]}
			{id:"check-score", kind:"branch", branches:[
				{when:{field:"score",op:"gte",value:80}, next:"approve"}
				{when:{field:"score",op:"lt",value:80}, next:"review"}
			]}
			{id:"reject", kind:"leaf", result:"rejected"}
			{id:"approve", kind:"leaf", result:"approved"}
			{id:"review", kind:"leaf", result:"needs-review"}
		]})
		{age:25,score:90} tree decision.eval-tree
	`)
	s, _ := result[0].AsString()
	if s != "approved" {
		t.Errorf("expected approved, got %v", result[0])
	}
}

// --- decide (unified) ---

func TestDecideTable(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def model ({kind:"table", hit-policy:"first", rules:[
			{when:{field:"x",op:"gt",value:0}, then:{sign:"positive"}}
			{when:{field:"x",op:"lt",value:0}, then:{sign:"negative"}}
		]})
		{x:5} model decision.decide
	`)
	m := result[0].AsMap()
	sign, _ := m.Get("sign")
	s, _ := sign.AsString()
	if s != "positive" {
		t.Errorf("expected positive, got %v", sign)
	}
}

func TestDecideTree(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def model ({kind:"tree", root:"root", nodes:[
			{id:"root", kind:"branch", branches:[
				{when:{field:"temp",op:"gt",value:30}, next:"hot"}
				{when:{field:"temp",op:"lt",value:10}, next:"cold"}
			]}
			{id:"hot", kind:"leaf", result:"hot"}
			{id:"cold", kind:"leaf", result:"cold"}
		]})
		{temp:35} model decision.decide
	`)
	s, _ := result[0].AsString()
	if s != "hot" {
		t.Errorf("expected hot, got %v", result[0])
	}
}

// --- Deep structure regression tests ---
//
// These tests exercise the decision module against deeply nested data
// shapes. Before the strict undefined-word rule, bare-word values inside
// nested predicate / branch / rule maps were lazily converted to
// `Atom{Undefined:true}` and could leak through several layers of
// auto-evaluation, causing intermittent end-of-Run failures depending
// on whether each particular nesting level happened to consume them as
// an Atom slot. With strict mode the only way to put a name into a
// deep structure is an explicit string or `(quote name)`, so the data
// shape is unambiguous all the way down. The tests below build trees /
// predicates / tables several levels deep using string field names and
// confirm the evaluator threads a result through every layer.

// TestDecisionDeepNestedPredicates builds a 4-level predicate tree
// any-of(all-of(p1, not-of(p2)), all-of(p3, p4)) and runs eval-pred
// against an input that should make exactly one branch true.
func TestDecisionDeepNestedPredicates(t *testing.T) {
	r := decisionRegistry(t)
	src := `
		def p1 {field:"a", op:"gte", value:10}
		def p2 {field:"b", op:"lt", value:5}
		def p3 {field:"c", op:"eq", value:"x"}
		def p4 {field:"d", op:"neq", value:"y"}

		def inner-not (p2 decision.not-of)
		def left  ([p1 inner-not] decision.all-of)
		def right ([p3 p4] decision.all-of)
		def root  ([left right] decision.any-of)
	`

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		// left fires: a>=10 AND not(b<5).
		{"LeftBranchTrue", `{a:15, b:10, c:"none", d:"y"}`, true},
		// right fires: c==x AND d!=y.
		{"RightBranchTrue", `{a:0, b:0, c:"x", d:"z"}`, true},
		// neither fires.
		{"BothBranchesFalse", `{a:0, b:10, c:"y", d:"y"}`, false},
		// left fails because b<5 makes the not-of false; right fails on c.
		{"NotOfBlocksLeft", `{a:15, b:1, c:"y", d:"z"}`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := runDecisionAQL(t, r, src+`
				`+tc.input+` root decision.eval-pred
			`)
			b, _ := result[0].AsBoolean()
			if b != tc.want {
				t.Errorf("got %v, want %v for input %s", b, tc.want, tc.input)
			}
		})
	}
}

// TestDecisionDeepCompoundTable builds a decision table whose rules
// contain compound predicates at three nesting levels (group-all
// containing group-any plus group-not-of-cond). Each level used to be
// vulnerable to bare-word leakage in the lenient regime.
func TestDecisionDeepCompoundTable(t *testing.T) {
	r := decisionRegistry(t)
	src := `
		def tbl ({kind:"table", hit-policy:"first", rules:[
		  {when:{kind:"group", op:"all", children:[
		    {field:"role", op:"eq", value:"admin"}
		    {kind:"group", op:"any", children:[
		      {field:"region", op:"eq", value:"us"}
		      {field:"region", op:"eq", value:"eu"}
		    ]}
		    {kind:"group", op:"not", children:{field:"banned", op:"eq", value:true}}
		  ]}, then:{access:"full"}}
		  {when:{kind:"group", op:"any", children:[
		    {field:"role", op:"eq", value:"user"}
		    {field:"role", op:"eq", value:"reader"}
		  ]}, then:{access:"limited"}}
		  {when:{field:"role", op:"eq", value:"guest"}, then:{access:"none"}}
		]})
	`

	cases := []struct {
		name  string
		input string
		want  string // empty string => expect no-match
	}{
		{"AdminEUNotBanned", `{role:"admin", region:"eu", banned:false}`, "full"},
		{"AdminUSNotBanned", `{role:"admin", region:"us", banned:false}`, "full"},
		{"AdminBanned", `{role:"admin", region:"eu", banned:true}`, ""}, // rule 1 fails on not(banned); admin doesn't match rules 2/3
		{"User", `{role:"user", region:"us", banned:false}`, "limited"},
		{"Reader", `{role:"reader", region:"eu", banned:false}`, "limited"},
		{"Guest", `{role:"guest", region:"us", banned:false}`, "none"},
		{"AdminAsiaNotBanned", `{role:"admin", region:"asia", banned:false}`, ""}, // rule 1's any-of(us,eu) fails
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := runDecisionAQL(t, r, src+`
				`+tc.input+` tbl decision.eval-table
			`)
			m := result[0].AsMap()
			if m == nil {
				t.Fatalf("expected map result, got %s", result[0].VType.String())
			}
			if tc.want == "" {
				errVal, ok := m.Get("error")
				if !ok {
					t.Fatalf("expected no-match error, got %s", result[0])
				}
				es, _ := errVal.AsString()
				if es != "no-match" {
					t.Errorf("got error=%q, want no-match", es)
				}
				return
			}
			access, ok := m.Get("access")
			if !ok {
				t.Fatalf("missing 'access' key in result %s", result[0])
			}
			s, _ := access.AsString()
			if s != tc.want {
				t.Errorf("got access=%q, want %q for input %s", s, tc.want, tc.input)
			}
		})
	}
}

// TestDecisionDeepBranchingTree exercises a 3-level decision tree where
// each branch points at another branch, terminating in distinct leaf
// nodes. The same id strings appear in different roles (next pointer,
// node id, leaf result) so any leakage of an undefined-atom version of
// the name would change one of them.
func TestDecisionDeepBranchingTree(t *testing.T) {
	r := decisionRegistry(t)
	src := `
		def model ({kind:"tree", root:"start", nodes:[
		  {id:"start", kind:"branch", branches:[
		    {when:{field:"region", op:"eq", value:"us"}, next:"us-check"}
		    {when:{field:"region", op:"eq", value:"eu"}, next:"eu-check"}
		  ]}
		  {id:"us-check", kind:"branch", branches:[
		    {when:{field:"age", op:"gte", value:21}, next:"us-adult"}
		    {when:{field:"age", op:"lt", value:21}, next:"us-minor"}
		  ]}
		  {id:"eu-check", kind:"branch", branches:[
		    {when:{field:"age", op:"gte", value:18}, next:"eu-adult"}
		    {when:{field:"age", op:"lt", value:18}, next:"eu-minor"}
		  ]}
		  {id:"us-adult", kind:"leaf", result:"us-allow"}
		  {id:"us-minor", kind:"leaf", result:"us-deny"}
		  {id:"eu-adult", kind:"leaf", result:"eu-allow"}
		  {id:"eu-minor", kind:"leaf", result:"eu-deny"}
		]})
	`

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"USAdult", `{region:"us", age:25}`, "us-allow"},
		{"USMinor", `{region:"us", age:19}`, "us-deny"},
		{"EUAdult", `{region:"eu", age:19}`, "eu-allow"},
		{"EUMinor", `{region:"eu", age:17}`, "eu-deny"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := runDecisionAQL(t, r, src+`
				`+tc.input+` model decision.decide
			`)
			s, _ := result[0].AsString()
			if s != tc.want {
				t.Errorf("got %q, want %q for input %s", s, tc.want, tc.input)
			}
		})
	}
}

// TestDecisionTreeDeepLeafResult confirms that the leaf result can
// itself be an arbitrarily nested map and that the evaluator returns it
// unchanged. Earlier the eval-tree loop ran maps through autoEval
// repeatedly, which could rewrite undefined-atom values inside the
// payload.
func TestDecisionTreeDeepLeafResult(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def model ({kind:"tree", root:"root", nodes:[
		  {id:"root", kind:"branch", branches:[
		    {when:{field:"x", op:"gt", value:0}, next:"pos"}
		    {when:{field:"x", op:"lt", value:0}, next:"neg"}
		  ]}
		  {id:"pos", kind:"leaf", result:{
		    sign:"positive",
		    detail:{
		      bucket:"high",
		      tags:["good","ok"],
		      nested:{level:3, label:"deep"}
		    }
		  }}
		  {id:"neg", kind:"leaf", result:"negative"}
		]})
		{x:5} model decision.decide
	`)
	m := result[0].AsMap()
	if m == nil {
		t.Fatalf("expected map result, got %s", result[0].VType.String())
	}
	sign, _ := m.Get("sign")
	s, _ := sign.AsString()
	if s != "positive" {
		t.Errorf("sign = %q, want positive", s)
	}
	detail, _ := m.Get("detail")
	dm := detail.AsMap()
	if dm == nil {
		t.Fatalf("expected detail map, got %s", detail.VType.String())
	}
	bucket, _ := dm.Get("bucket")
	if bs, _ := bucket.AsString(); bs != "high" {
		t.Errorf("detail.bucket = %q, want high", bs)
	}
	nested, _ := dm.Get("nested")
	nm := nested.AsMap()
	if nm == nil {
		t.Fatalf("expected nested map, got %s", nested.VType.String())
	}
	label, _ := nm.Get("label")
	if ls, _ := label.AsString(); ls != "deep" {
		t.Errorf("detail.nested.label = %q, want deep", ls)
	}
}

// TestDecisionDeepStructureRejectsUndefinedWords is a regression guard
// for the strict undefined-word rule: a bare-word `field` value buried
// inside a deeply nested when-clause must error with `undefined_word`
// instead of silently becoming an Atom that propagates through the
// evaluator. Before the rule, this kind of typo could survive several
// auto-eval passes and produce surprising "no-match" results; now it
// fails fast with a clear error.
func TestDecisionDeepStructureRejectsUndefinedWords(t *testing.T) {
	r := decisionRegistry(t)
	src := `
		def tbl ({kind:"table", hit-policy:"first", rules:[
		  {when:{kind:"group", op:"all", children:[
		    {field:age, op:"gte", value:18}
		  ]}, then:{ok:true}}
		]})
		{age:25} tbl decision.eval-table
	`
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	eng := engine.NewTop(r)
	_, err = eng.Run(values)
	if err == nil {
		t.Fatal("expected undefined_word error for bare-word field value, got nil")
	}
	if !contains(err.Error(), "undefined_word") || !contains(err.Error(), "age") {
		t.Errorf("expected undefined_word error mentioning 'age', got: %v", err)
	}
}

// TestDecisionDeepTreeRejectsUndefinedWords confirms the same strict
// rule applies inside decision-tree node payloads.
func TestDecisionDeepTreeRejectsUndefinedWords(t *testing.T) {
	r := decisionRegistry(t)
	// Bare-word "us" is the typo here — it must error rather than
	// silently mismatch every region.
	src := `
		def model ({kind:"tree", root:"start", nodes:[
		  {id:"start", kind:"branch", branches:[
		    {when:{field:"region", op:"eq", value:us}, next:"us"}
		  ]}
		  {id:"us", kind:"leaf", result:"ok"}
		]})
		{region:"us"} model decision.decide
	`
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	eng := engine.NewTop(r)
	_, err = eng.Run(values)
	if err == nil {
		t.Fatal("expected undefined_word error for bare-word value, got nil")
	}
	if !contains(err.Error(), "undefined_word") || !contains(err.Error(), "us") {
		t.Errorf("expected undefined_word error mentioning 'us', got: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
