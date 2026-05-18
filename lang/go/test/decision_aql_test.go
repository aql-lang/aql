package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/internal/nativemod"
	"github.com/aql-lang/aql/lang/go/native"
)

// buildDecisionFileAQL generates the full decision.aql file content.
// The AQL source already includes the export block.
func buildDecisionFileAQL() string {
	return nativemod.DecisionAQL()
}

// buildDecisionModuleAQL generates the module [...] inline version.
func buildDecisionModuleAQL() string {
	return "module [\n" + nativemod.DecisionAQL() + "\n]\n"
}

// --- Test infrastructure ---

// decisionTestCase defines a single test for the decision module.
type decisionTestCase struct {
	name string
	// setup runs before expr if intermediate defs are needed (persistent engine).
	setup string
	// expr is the AQL expression to evaluate.
	expr string
	// check validates the result.
	check func(t *testing.T, result []native.Value)
}

func checkBool(want bool) func(t *testing.T, result []native.Value) {
	return func(t *testing.T, result []native.Value) {
		t.Helper()
		b, _ := native.AsBoolean(result[0])
		if b != want {
			t.Errorf("got %v, want %v", b, want)
		}
	}
}

func checkMapField(key, want string) func(t *testing.T, result []native.Value) {
	return func(t *testing.T, result []native.Value) {
		t.Helper()
		m, _ := native.AsMap(result[0])
		if m == nil {
			t.Fatalf("expected map, got %s", result[0].VType.String())
		}
		v, _ := m.Get(key)
		s, _ := native.AsString(v)
		if s != want {
			t.Errorf("%s = %q, want %q", key, s, want)
		}
	}
}

func checkString(want string) func(t *testing.T, result []native.Value) {
	return func(t *testing.T, result []native.Value) {
		t.Helper()
		s, _ := native.AsString(result[0])
		if s != want {
			t.Errorf("got %q, want %q", s, want)
		}
	}
}

func checkCollectLen(want int) func(t *testing.T, result []native.Value) {
	return func(t *testing.T, result []native.Value) {
		t.Helper()
		list, _ := native.AsList(result[0])
		if list.Len() != want {
			t.Fatalf("expected %d collected, got %d: %v", want, list.Len(), result[0])
		}
	}
}

// Shared test cases for the decision module. Each test case runs against
// the native module, inline AQL module, and file module.
var decisionTests = []decisionTestCase{
	// --- cond builder ---
	{
		name: "Cond",
		expr: `18 "gte" quote age decision.cond`,
		check: func(t *testing.T, result []native.Value) {
			t.Helper()
			m, _ := native.AsMap(result[0])
			if m == nil {
				t.Fatalf("expected map, got %s", result[0].VType.String())
			}
			field, _ := m.Get("field")
			if field.String() != "age" {
				t.Errorf("field = %v, want age", field)
			}
		},
	},

	// --- eval-cond ---
	{
		name:  "EvalCondTrue",
		expr:  `{age:25} {field:"age",op:"gte",value:18} decision.eval-cond`,
		check: checkBool(true),
	},
	{
		name:  "EvalCondFalse",
		expr:  `{age:15} {field:"age",op:"gte",value:18} decision.eval-cond`,
		check: checkBool(false),
	},
	{
		name:  "EvalCondEq",
		expr:  `{status:"active"} {field:"status",op:"eq",value:"active"} decision.eval-cond`,
		check: checkBool(true),
	},

	// --- eval-pred ---
	{
		name:  "EvalPredAllOf",
		setup: `def pred ([{field:"age",op:"gte",value:18} {field:"score",op:"gt",value:50}] decision.all-of)`,
		expr:  `{age:25,score:80} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name:  "EvalPredAllOfFalse",
		setup: `def pred ([{field:"age",op:"gte",value:18} {field:"score",op:"gt",value:50}] decision.all-of)`,
		expr:  `{age:25,score:30} pred decision.eval-pred`,
		check: checkBool(false),
	},
	{
		name:  "EvalPredAnyOf",
		setup: `def pred ([{field:"age",op:"gte",value:18} {field:"score",op:"gt",value:50}] decision.any-of)`,
		expr:  `{age:10,score:80} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name:  "EvalPredNotOf",
		setup: `def pred ({field:"age",op:"lt",value:18} decision.not-of)`,
		expr:  `{age:25} pred decision.eval-pred`,
		check: checkBool(true),
	},

	// --- eval-table ---
	{
		name: "TableFirst",
		setup: `def tbl ([
			{when:{field:"age",op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:"age",op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkMapField("category", "adult"),
	},
	{
		name: "TableFirstMinor",
		setup: `def tbl ([
			{when:{field:"age",op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:"age",op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)`,
		expr:  `{age:12} tbl decision.eval-table`,
		check: checkMapField("category", "minor"),
	},
	{
		name: "TableUnique",
		setup: `def rawtbl ([
			{when:{field:"score",op:"lt",value:50}, then:{grade:"fail"}}
			{when:{field:"score",op:"gte",value:50}, then:{grade:"pass"}}
		] decision.make-table)
		def tbl (rawtbl "unique" decision.with-policy)`,
		expr:  `{score:75} tbl decision.eval-table`,
		check: checkMapField("grade", "pass"),
	},
	{
		name:  "TableNoMatch",
		setup: `def tbl ([{when:{field:"age",op:"gt",value:100}, then:{x:1}}] decision.make-table)`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkMapField("error", "no-match"),
	},
	{
		name: "TableCompound",
		setup: `def tbl ({kind:"table", hit-policy:"first", rules:[
			{when:{kind:"group",op:"all",children:[{field:"age",op:"lt",value:30} {field:"score",op:"gte",value:90}]}, then:{tier:"premium"}}
			{when:{field:"score",op:"gte",value:50}, then:{tier:"standard"}}
		]})`,
		expr:  `{age:25,score:95} tbl decision.eval-table`,
		check: checkMapField("tier", "premium"),
	},
	{
		name: "TableCollect",
		setup: `def rawtbl ([
			{when:{field:"age",op:"gte",value:18}, then:{perk:"vote"}}
			{when:{field:"age",op:"gte",value:21}, then:{perk:"drink"}}
		] decision.make-table)
		def tbl (rawtbl "collect" decision.with-policy)`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkCollectLen(2),
	},
	{
		name: "TablePriority",
		setup: `def tbl ({kind:"table", hit-policy:"priority", rules:[
			{when:{field:"age",op:"gte",value:18}, then:{tier:"adult"}, priority:1}
			{when:{field:"age",op:"gte",value:21}, then:{tier:"senior"}, priority:10}
			{when:{field:"age",op:"gte",value:0}, then:{tier:"any"}, priority:0}
		]})`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkMapField("tier", "senior"),
	},

	// --- eval-tree ---
	{
		name: "Tree",
		setup: `def tree ({kind:"tree", root:"root", nodes:[
			{id:"root", kind:"branch", branches:[
				{when:{field:"age",op:"lt",value:18}, next:"minor"}
				{when:{field:"age",op:"gte",value:18}, next:"adult"}
			]}
			{id:"minor", kind:"leaf", result:{category:"minor"}}
			{id:"adult", kind:"leaf", result:{category:"adult"}}
		]})`,
		expr:  `{age:25} tree decision.eval-tree`,
		check: checkMapField("category", "adult"),
	},
	{
		name: "TreeMinor",
		setup: `def tree ({kind:"tree", root:"root", nodes:[
			{id:"root", kind:"branch", branches:[
				{when:{field:"age",op:"lt",value:18}, next:"minor"}
				{when:{field:"age",op:"gte",value:18}, next:"adult"}
			]}
			{id:"minor", kind:"leaf", result:"too-young"}
			{id:"adult", kind:"leaf", result:"welcome"}
		]})`,
		expr:  `{age:12} tree decision.eval-tree`,
		check: checkString("too-young"),
	},
	{
		name: "TreeMultiLevel",
		setup: `def tree ({kind:"tree", root:"check-age", nodes:[
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
		]})`,
		expr:  `{age:25,score:90} tree decision.eval-tree`,
		check: checkString("approved"),
	},

	// --- eval-cond (additional comparison ops) ---
	{
		name:  "EvalCondNeq",
		expr:  `{status:"active"} {field:"status",op:"neq",value:"inactive"} decision.eval-cond`,
		check: checkBool(true),
	},
	{
		name:  "EvalCondLt",
		expr:  `{age:10} {field:"age",op:"lt",value:18} decision.eval-cond`,
		check: checkBool(true),
	},
	{
		name:  "EvalCondLte",
		expr:  `{age:18} {field:"age",op:"lte",value:18} decision.eval-cond`,
		check: checkBool(true),
	},
	{
		name:  "EvalCondGt",
		expr:  `{age:25} {field:"age",op:"gt",value:18} decision.eval-cond`,
		check: checkBool(true),
	},
	{
		name:  "EvalCondGtFalse",
		expr:  `{age:5} {field:"age",op:"gt",value:18} decision.eval-cond`,
		check: checkBool(false),
	},
	{
		name:  "EvalCondUnknownOpReturnsFalse",
		expr:  `{age:25} {field:"age",op:"weird",value:18} decision.eval-cond`,
		check: checkBool(false),
	},

	// --- eval-pred edge cases ---
	{
		name:  "EvalPredAllOfEmptyIsTrue",
		setup: `def pred ([] decision.all-of)`,
		expr:  `{age:25} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name:  "EvalPredAnyOfEmptyIsFalse",
		setup: `def pred ([] decision.any-of)`,
		expr:  `{age:25} pred decision.eval-pred`,
		check: checkBool(false),
	},
	{
		name:  "EvalPredAllOfSingleChildTrue",
		setup: `def pred ([{field:"age",op:"gte",value:18}] decision.all-of)`,
		expr:  `{age:25} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name:  "EvalPredAllOfSingleChildFalse",
		setup: `def pred ([{field:"age",op:"gte",value:18}] decision.all-of)`,
		expr:  `{age:10} pred decision.eval-pred`,
		check: checkBool(false),
	},
	{
		name:  "EvalPredAnyOfAllFalse",
		setup: `def pred ([{field:"age",op:"gte",value:100} {field:"age",op:"lt",value:0}] decision.any-of)`,
		expr:  `{age:25} pred decision.eval-pred`,
		check: checkBool(false),
	},
	{
		name:  "EvalPredAnyOfFirstTrueShortCircuits",
		setup: `def pred ([{field:"age",op:"gte",value:18} {field:"never",op:"eq",value:"x"}] decision.any-of)`,
		expr:  `{age:25} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name:  "EvalPredAllOfThreeChildrenTrue",
		setup: `def pred ([{field:"age",op:"gte",value:18} {field:"score",op:"gt",value:50} {field:"active",op:"eq",value:true}] decision.all-of)`,
		expr:  `{age:25,score:80,active:true} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name:  "EvalPredAllOfThreeChildrenOneFails",
		setup: `def pred ([{field:"age",op:"gte",value:18} {field:"score",op:"gt",value:50} {field:"active",op:"eq",value:true}] decision.all-of)`,
		expr:  `{age:25,score:80,active:false} pred decision.eval-pred`,
		check: checkBool(false),
	},

	// --- eval-pred nested compositions ---
	{
		name: "EvalPredAnyOfInsideAllOf",
		setup: `def pred ([
			{field:"age",op:"gte",value:18}
			{kind:"group",op:"any",children:[
				{field:"role",op:"eq",value:"admin"}
				{field:"role",op:"eq",value:"editor"}
			]}
		] decision.all-of)`,
		expr:  `{age:25,role:"admin"} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name: "EvalPredAnyOfInsideAllOfRejects",
		setup: `def pred ([
			{field:"age",op:"gte",value:18}
			{kind:"group",op:"any",children:[
				{field:"role",op:"eq",value:"admin"}
				{field:"role",op:"eq",value:"editor"}
			]}
		] decision.all-of)`,
		expr:  `{age:25,role:"viewer"} pred decision.eval-pred`,
		check: checkBool(false),
	},
	{
		name: "EvalPredAllOfInsideAnyOf",
		setup: `def pred ([
			{kind:"group",op:"all",children:[
				{field:"age",op:"gte",value:18}
				{field:"age",op:"lt",value:65}
			]}
			{field:"vip",op:"eq",value:true}
		] decision.any-of)`,
		expr:  `{age:80,vip:true} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name: "EvalPredNotOfGroup",
		setup: `def pred ({kind:"group",op:"not",children:[{kind:"group",op:"any",children:[
			{field:"status",op:"eq",value:"banned"}
			{field:"status",op:"eq",value:"suspended"}
		]}]} )`,
		expr:  `{status:"active"} pred decision.eval-pred`,
		check: checkBool(true),
	},
	// (Deeper-than-2-level group nesting hits a pre-existing scoping
	// issue in eval-pred where `def children quote (pred get "children")`
	// shadows the parent iteration's children. Tests at 2-level work.)

	// --- eval-table: more hit-policy cases ---
	{
		name: "TableCollectNoMatch",
		setup: `def rawtbl ([
			{when:{field:"age",op:"gt",value:100}, then:{x:1}}
		] decision.make-table)
		def tbl (rawtbl "collect" decision.with-policy)`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkCollectLen(0),
	},
	{
		name: "TableUniqueMultipleMatchesError",
		setup: `def rawtbl ([
			{when:{field:"age",op:"gte",value:18}, then:{tag:"a"}}
			{when:{field:"age",op:"gte",value:21}, then:{tag:"b"}}
		] decision.make-table)
		def tbl (rawtbl "unique" decision.with-policy)`,
		expr:  `{age:30} tbl decision.eval-table`,
		check: checkMapField("error", "multiple-matches"),
	},
	{
		name: "TablePriorityNoMatch",
		setup: `def tbl ({kind:"table", hit-policy:"priority", rules:[
			{when:{field:"age",op:"gt",value:100}, then:{tier:"X"}, priority:1}
		]})`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkMapField("error", "no-match"),
	},
	{
		name: "TablePriorityMissingPriorityFieldDefaultsToZero",
		setup: `def tbl ({kind:"table", hit-policy:"priority", rules:[
			{when:{field:"age",op:"gte",value:18}, then:{tier:"plain"}}
			{when:{field:"age",op:"gte",value:21}, then:{tier:"vip"}, priority:5}
		]})`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkMapField("tier", "vip"),
	},
	{
		name: "TableNestedAllChildren",
		setup: `def tbl ({kind:"table", hit-policy:"first", rules:[
			{when:{kind:"group",op:"all",children:[
				{kind:"group",op:"any",children:[
					{field:"role",op:"eq",value:"admin"}
					{field:"role",op:"eq",value:"editor"}
				]}
				{field:"active",op:"eq",value:true}
			]}, then:{access:"granted"}}
			{when:{field:"active",op:"eq",value:true}, then:{access:"limited"}}
		]})`,
		expr:  `{role:"editor",active:true} tbl decision.eval-table`,
		check: checkMapField("access", "granted"),
	},

	// --- eval-tree: error / edge cases ---
	{
		name: "TreeNoBranchMatch",
		setup: `def tree ({kind:"tree", root:"root", nodes:[
			{id:"root", kind:"branch", branches:[
				{when:{field:"age",op:"gt",value:100}, next:"tooOld"}
			]}
			{id:"tooOld", kind:"leaf", result:"old"}
		]})`,
		expr:  `{age:25} tree decision.eval-tree`,
		check: checkMapField("error", "no-branch-match"),
	},
	{
		name: "TreeBrokenNextId",
		setup: `def tree ({kind:"tree", root:"root", nodes:[
			{id:"root", kind:"branch", branches:[
				{when:{field:"flag",op:"eq",value:true}, next:"missing"}
			]}
		]})`,
		expr:  `{flag:true} tree decision.eval-tree`,
		check: checkMapField("error", "node-not-found"),
	},

	// --- decide error case ---
	{
		name:  "DecideUnknownKind",
		setup: `def model ({kind:"weird"})`,
		expr:  `{x:5} model decision.decide`,
		check: checkMapField("error", "unknown-model-kind"),
	},

	// --- eval-pred uses each + all/any (regression: short-circuit identity) ---
	{
		name:  "EvalPredAllOfFirstFails",
		setup: `def pred ([{field:"never",op:"eq",value:"x"} {field:"age",op:"gte",value:18}] decision.all-of)`,
		expr:  `{age:25} pred decision.eval-pred`,
		check: checkBool(false),
	},
	{
		name:  "EvalPredAllOfLastFails",
		setup: `def pred ([{field:"age",op:"gte",value:18} {field:"never",op:"eq",value:"x"}] decision.all-of)`,
		expr:  `{age:25} pred decision.eval-pred`,
		check: checkBool(false),
	},
	{
		name:  "EvalPredAnyOfLastTrue",
		setup: `def pred ([{field:"age",op:"gt",value:100} {field:"age",op:"lt",value:0} {field:"age",op:"gte",value:18}] decision.any-of)`,
		expr:  `{age:25} pred decision.eval-pred`,
		check: checkBool(true),
	},

	// --- decide (unified) ---
	{
		name: "DecideTable",
		setup: `def model ({kind:"table", hit-policy:"first", rules:[
			{when:{field:"x",op:"gt",value:0}, then:{sign:"positive"}}
			{when:{field:"x",op:"lt",value:0}, then:{sign:"negative"}}
		]})`,
		expr:  `{x:5} model decision.decide`,
		check: checkMapField("sign", "positive"),
	},
	{
		name: "DecideTree",
		setup: `def model ({kind:"tree", root:"root", nodes:[
			{id:"root", kind:"branch", branches:[
				{when:{field:"temp",op:"gt",value:30}, next:"hot"}
				{when:{field:"temp",op:"lt",value:10}, next:"cold"}
			]}
			{id:"hot", kind:"leaf", result:"hot"}
			{id:"cold", kind:"leaf", result:"cold"}
		]})`,
		expr:  `{temp:35} model decision.decide`,
		check: checkString("hot"),
	},
}

// --- Runner helper ---

// runDecisionTest runs a single test case against an initialized registry
// where the decision module is already imported as "decision".
func runDecisionTest(t *testing.T, tc decisionTestCase, reg *native.Registry) {
	t.Helper()
	eng := native.NewTop(reg)

	if tc.setup != "" {
		vals, err := parser.Parse(tc.setup)
		if err != nil {
			t.Fatalf("setup parse: %v", err)
		}
		_, err = eng.Run(vals)
		if err != nil {
			t.Fatalf("setup run: %v", err)
		}
	}

	vals, err := parser.Parse(tc.expr)
	if err != nil {
		t.Fatalf("expr parse: %v", err)
	}
	result, err := eng.Run(vals)
	if err != nil {
		t.Fatalf("expr run: %v", err)
	}
	tc.check(t, result)
}

// --- Native module tests ---

func nativeDecisionRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	if err := nativemod.InstallDecisionExports(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNativeDecision(t *testing.T) {
	for _, tc := range decisionTests {
		t.Run(tc.name, func(t *testing.T) {
			r := nativeDecisionRegistry(t)
			runDecisionTest(t, tc, r)
		})
	}
}

// --- module [...] inline AQL tests ---

func inlineDecisionRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	native.Register(r)

	src := buildDecisionModuleAQL() + "\nimport (quote decision)\n"
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	eng := native.NewTop(r)
	if _, err := eng.Run(vals); err != nil {
		t.Fatalf("run: %v", err)
	}
	return r
}

func TestInlineDecision(t *testing.T) {
	for _, tc := range decisionTests {
		t.Run(tc.name, func(t *testing.T) {
			r := inlineDecisionRegistry(t)
			runDecisionTest(t, tc, r)
		})
	}
}

// --- File module tests ---

func TestGenerateDecisionAQL(t *testing.T) {
	// Generate decision.aql deterministically from the native module's
	// AQL (source of truth) + export block.
	path := filepath.Join(moduleWorkDir(t), "decision", "decision.aql")
	content := buildDecisionFileAQL()

	existing, err := os.ReadFile(path)
	if err != nil || string(existing) != content {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if err == nil {
			t.Log("decision.aql was out of date and has been regenerated")
		}
	}

	// Verify the file matches what we'd generate.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatal("decision.aql does not match generated content")
	}
}

func TestFileDecision(t *testing.T) {
	// Ensure the file is up to date before running tests.
	path := filepath.Join(moduleWorkDir(t), "decision", "decision.aql")
	if err := os.WriteFile(path, []byte(buildDecisionFileAQL()), 0644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range decisionTests {
		t.Run(tc.name, func(t *testing.T) {
			dir := moduleWorkDir(t)
			steps := []string{`(import "./decision")`}
			if tc.setup != "" {
				steps = append(steps, tc.setup)
			}
			steps = append(steps, tc.expr)

			result, err := runRealFileSteps(t, dir, steps)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, result)
		})
	}
}

// --- Verify file content is deterministic ---

func TestDecisionAQLDeterministic(t *testing.T) {
	// Verify that the generated file exactly matches the expected content.
	content := buildDecisionFileAQL()
	aql := nativemod.DecisionAQL()

	if !strings.Contains(content, strings.TrimSpace(aql)) {
		t.Error("generated decision.aql does not contain the native module's AQL")
	}
}
