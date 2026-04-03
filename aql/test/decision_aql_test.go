package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"github.com/metsitaba/voxgig-exp/aql/internal/nativemod"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// decisionExportAQL is the export block listing all functions.
const decisionExportAQL = `
export decision {
  Cond:        Cond
  Pred:        Pred
  Rule:        Rule
  DTable:      DTable
  BranchNode:  BranchNode
  LeafNode:    LeafNode
  DTree:       DTree
  cond:        cond
  all-of:      all-of
  any-of:      any-of
  not-of:      not-of
  make-rule:   make-rule
  make-table:  make-table
  with-policy: with-policy
  make-branch: make-branch
  make-leaf:   make-leaf
  make-tree:   make-tree
  apply-op:    apply-op
  eval-cond:   eval-cond
  eval-pred:   eval-pred
  eval-table:  eval-table
  eval-tree:   eval-tree
  decide:      decide
}
`

// buildDecisionFileAQL generates the full decision.aql file content
// deterministically from the native module's AQL (source of truth)
// plus the export block.
func buildDecisionFileAQL() string {
	return nativemod.DecisionAQL() + decisionExportAQL
}

// buildDecisionModuleAQL generates the module [...] inline version
// from the same source.
func buildDecisionModuleAQL() string {
	return "module [\n" + nativemod.DecisionAQL() +
		decisionExportAQL + "\n]\n"
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
	check func(t *testing.T, result []engine.Value)
}

func checkBool(want bool) func(t *testing.T, result []engine.Value) {
	return func(t *testing.T, result []engine.Value) {
		t.Helper()
		b, _ := result[0].AsBoolean()
		if b != want {
			t.Errorf("got %v, want %v", b, want)
		}
	}
}

func checkMapField(key, want string) func(t *testing.T, result []engine.Value) {
	return func(t *testing.T, result []engine.Value) {
		t.Helper()
		m := result[0].AsMap()
		if m == nil {
			t.Fatalf("expected map, got %s", result[0].VType.String())
		}
		v, _ := m.Get(key)
		s, _ := v.AsString()
		if s != want {
			t.Errorf("%s = %q, want %q", key, s, want)
		}
	}
}

func checkString(want string) func(t *testing.T, result []engine.Value) {
	return func(t *testing.T, result []engine.Value) {
		t.Helper()
		s, _ := result[0].AsString()
		if s != want {
			t.Errorf("got %q, want %q", s, want)
		}
	}
}

func checkCollectLen(want int) func(t *testing.T, result []engine.Value) {
	return func(t *testing.T, result []engine.Value) {
		t.Helper()
		list := result[0].AsList()
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
		expr: `18 "gte" age decision.cond`,
		check: func(t *testing.T, result []engine.Value) {
			t.Helper()
			m := result[0].AsMap()
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
		expr:  `{age:25} {field:age,op:"gte",value:18} decision.eval-cond`,
		check: checkBool(true),
	},
	{
		name:  "EvalCondFalse",
		expr:  `{age:15} {field:age,op:"gte",value:18} decision.eval-cond`,
		check: checkBool(false),
	},
	{
		name:  "EvalCondEq",
		expr:  `{status:"active"} {field:status,op:"eq",value:"active"} decision.eval-cond`,
		check: checkBool(true),
	},

	// --- eval-pred ---
	{
		name:  "EvalPredAllOf",
		setup: `def pred ([{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.all-of)`,
		expr:  `{age:25,score:80} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name:  "EvalPredAllOfFalse",
		setup: `def pred ([{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.all-of)`,
		expr:  `{age:25,score:30} pred decision.eval-pred`,
		check: checkBool(false),
	},
	{
		name:  "EvalPredAnyOf",
		setup: `def pred ([{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.any-of)`,
		expr:  `{age:10,score:80} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name:  "EvalPredNotOf",
		setup: `def pred ({field:age,op:"lt",value:18} decision.not-of)`,
		expr:  `{age:25} pred decision.eval-pred`,
		check: checkBool(true),
	},

	// --- eval-table ---
	{
		name: "TableFirst",
		setup: `def tbl ([
			{when:{field:age,op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:age,op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkMapField("category", "adult"),
	},
	{
		name: "TableFirstMinor",
		setup: `def tbl ([
			{when:{field:age,op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:age,op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)`,
		expr:  `{age:12} tbl decision.eval-table`,
		check: checkMapField("category", "minor"),
	},
	{
		name: "TableUnique",
		setup: `def rawtbl ([
			{when:{field:score,op:"lt",value:50}, then:{grade:"fail"}}
			{when:{field:score,op:"gte",value:50}, then:{grade:"pass"}}
		] decision.make-table)
		def tbl (rawtbl "unique" decision.with-policy)`,
		expr:  `{score:75} tbl decision.eval-table`,
		check: checkMapField("grade", "pass"),
	},
	{
		name:  "TableNoMatch",
		setup: `def tbl ([{when:{field:age,op:"gt",value:100}, then:{x:1}}] decision.make-table)`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkMapField("error", "no-match"),
	},
	{
		name: "TableCompound",
		setup: `def tbl ({kind:"table", hit-policy:"first", rules:[
			{when:{kind:"group",op:"all",children:[{field:age,op:"lt",value:30} {field:score,op:"gte",value:90}]}, then:{tier:"premium"}}
			{when:{field:score,op:"gte",value:50}, then:{tier:"standard"}}
		]})`,
		expr:  `{age:25,score:95} tbl decision.eval-table`,
		check: checkMapField("tier", "premium"),
	},
	{
		name: "TableCollect",
		setup: `def rawtbl ([
			{when:{field:age,op:"gte",value:18}, then:{perk:"vote"}}
			{when:{field:age,op:"gte",value:21}, then:{perk:"drink"}}
		] decision.make-table)
		def tbl (rawtbl "collect" decision.with-policy)`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkCollectLen(2),
	},
	{
		name: "TablePriority",
		setup: `def tbl ({kind:"table", hit-policy:"priority", rules:[
			{when:{field:age,op:"gte",value:18}, then:{tier:"adult"}, priority:1}
			{when:{field:age,op:"gte",value:21}, then:{tier:"senior"}, priority:10}
			{when:{field:age,op:"gte",value:0}, then:{tier:"any"}, priority:0}
		]})`,
		expr:  `{age:25} tbl decision.eval-table`,
		check: checkMapField("tier", "senior"),
	},

	// --- eval-tree ---
	{
		name: "Tree",
		setup: `def tree ({kind:"tree", root:root, nodes:[
			{id:root, kind:"branch", branches:[
				{when:{field:age,op:"lt",value:18}, next:minor}
				{when:{field:age,op:"gte",value:18}, next:adult}
			]}
			{id:minor, kind:"leaf", result:{category:"minor"}}
			{id:adult, kind:"leaf", result:{category:"adult"}}
		]})`,
		expr:  `{age:25} tree decision.eval-tree`,
		check: checkMapField("category", "adult"),
	},
	{
		name: "TreeMinor",
		setup: `def tree ({kind:"tree", root:root, nodes:[
			{id:root, kind:"branch", branches:[
				{when:{field:age,op:"lt",value:18}, next:minor}
				{when:{field:age,op:"gte",value:18}, next:adult}
			]}
			{id:minor, kind:"leaf", result:"too-young"}
			{id:adult, kind:"leaf", result:"welcome"}
		]})`,
		expr:  `{age:12} tree decision.eval-tree`,
		check: checkString("too-young"),
	},
	{
		name: "TreeMultiLevel",
		setup: `def tree ({kind:"tree", root:check-age, nodes:[
			{id:check-age, kind:"branch", branches:[
				{when:{field:age,op:"lt",value:18}, next:reject}
				{when:{field:age,op:"gte",value:18}, next:check-score}
			]}
			{id:check-score, kind:"branch", branches:[
				{when:{field:score,op:"gte",value:80}, next:approve}
				{when:{field:score,op:"lt",value:80}, next:review}
			]}
			{id:reject, kind:"leaf", result:"rejected"}
			{id:approve, kind:"leaf", result:"approved"}
			{id:review, kind:"leaf", result:"needs-review"}
		]})`,
		expr:  `{age:25,score:90} tree decision.eval-tree`,
		check: checkString("approved"),
	},

	// --- decide (unified) ---
	{
		name: "DecideTable",
		setup: `def model ({kind:"table", hit-policy:"first", rules:[
			{when:{field:x,op:"gt",value:0}, then:{sign:"positive"}}
			{when:{field:x,op:"lt",value:0}, then:{sign:"negative"}}
		]})`,
		expr:  `{x:5} model decision.decide`,
		check: checkMapField("sign", "positive"),
	},
	{
		name: "DecideTree",
		setup: `def model ({kind:"tree", root:root, nodes:[
			{id:root, kind:"branch", branches:[
				{when:{field:temp,op:"gt",value:30}, next:hot}
				{when:{field:temp,op:"lt",value:10}, next:cold}
			]}
			{id:hot, kind:"leaf", result:"hot"}
			{id:cold, kind:"leaf", result:"cold"}
		]})`,
		expr:  `{temp:35} model decision.decide`,
		check: checkString("hot"),
	},
}

// --- Runner helper ---

// runDecisionTest runs a single test case against an initialized registry
// where the decision module is already imported as "decision".
func runDecisionTest(t *testing.T, tc decisionTestCase, reg *engine.Registry) {
	t.Helper()
	eng := engine.NewTop(reg)

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

func nativeDecisionRegistry(t *testing.T) *engine.Registry {
	t.Helper()
	r, err := engine.DefaultRegistry()
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

func inlineDecisionRegistry(t *testing.T) *engine.Registry {
	t.Helper()
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	native.Register(r)

	src := buildDecisionModuleAQL() + "\nimport decision\n"
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	eng := engine.NewTop(r)
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
