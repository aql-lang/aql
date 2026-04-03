package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/nativemod"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// decisionEvalAQL contains the pure-AQL evaluator functions.
// These are used only by the file-based and module [...] AQL versions.
// The native Go module uses Go-implemented evaluators instead.
const decisionEvalAQL = `

# --- apply-op ---

def apply-op fn [[rhs:Any op:String lhs:Any] [Boolean] [if (op "eq" eq) [lhs rhs eq] [if (op "neq" eq) [lhs rhs neq] [if (op "lt" eq) [lhs rhs lt] [if (op "lte" eq) [lhs rhs lte] [if (op "gt" eq) [lhs rhs gt] [if (op "gte" eq) [lhs rhs gte] [false]]]]]]]]

def apply-op fn [[rhs:Any op:String] [Boolean] [if (op "is_true" eq) [rhs] [if (op "is_false" eq) [rhs not] [if (op "is_null" eq) [false] [if (op "is_not_null" eq) [true] [false]]]]]]

# --- eval-cond ---

def eval-cond fn [[c:Map input:Map] [Boolean] [input.((c.field convert String)) c.op c.value apply-op]]

# --- eval-pred helpers ---

def eval-pred-all fn [[children:List input:Map] [Boolean] [def result true for (children length) [def idx i if (input (children idx get) eval-pred not) [def result false] []] end result]]

def eval-pred-any fn [[children:List input:Map] [Boolean] [def result false for (children length) [def idx i if (input (children idx get) eval-pred) [def result true] []] end result]]

def eval-pred-not fn [[children:Map input:Map] [Boolean] [input children eval-cond not]]
def eval-pred-not fn [[children:List input:Map] [Boolean] [input (children 0 get) eval-pred not]]

# --- eval-pred ---

def eval-pred fn [[pred:Map input:Map] [Boolean] [if ((pred get "kind") "group" eq) [(def group-op (pred get "op") def children quote (pred get "children") if (group-op "all" eq) [input children eval-pred-all] [if (group-op "any" eq) [input children eval-pred-any] [if (group-op "not" eq) [input children eval-pred-not] [false]]])] [input pred eval-cond]]]

# --- eval-table helpers ---

def eval-table-first fn [[rules:List input:Map] [Any] [def result (do {ok: false, error: "no-match"}) def found false for (rules length) [def idx i def rule (rules idx get) if (found not) [if (input (rule get "when") eval-pred) [def result (rule get "then") def found true] []] []] end result]]

def eval-table-unique fn [[rules:List input:Map] [Any] [def result (do {ok: false, error: "no-match"}) def match-count 0 for (rules length) [def idx i def rule (rules idx get) if (input (rule get "when") eval-pred) [def result (rule get "then") def match-count (match-count 1 add)] []] end if (match-count 1 eq) [result] [if (match-count 0 eq) [do {ok: false, error: "no-match"}] [do {ok: false, error: "multiple-matches"}]]]]

# --- eval-table ---

def eval-table fn [[table:Map input:Map] [Any] [def rules quote (table get "rules") def policy (table get "hit-policy") if (policy "first" eq) [input rules eval-table-first] [if (policy "unique" eq) [input rules eval-table-unique] [input rules eval-table-first]]]]

# --- eval-tree helpers ---

def find-node fn [[id:Any nodes:List] [Any] [def found None for (nodes length) [def idx i def node (nodes idx get) if ((node get "id" convert String) (id convert String) eq) [def found node] []] end found]]

def find-branch-next fn [[branches:List input:Map] [Any] [def next-id None for (branches length) [def idx i def br (branches idx get) if (input (br get "when") eval-pred) [def next-id (br get "next")] []] end next-id]]

# --- eval-tree ---

def eval-tree fn [[tree:Map input:Map] [Any] [def nodes quote (tree get "nodes") def current (nodes (tree get "root") find-node) def done false def result (do {ok: false, error: "max-depth-exceeded"}) for 100 [def _i i if (done not) [if ((current get "kind") "leaf" eq) [def result (current get "result") def done true] [if ((current get "kind") "branch" eq) [(def next-id (input quote (current get "branches") find-branch-next) if (next-id None eq) [def result (do {ok: false, error: "no-branch-match"}) def done true] [def current (nodes next-id find-node) if (current None eq) [def result (do {ok: false, error: "node-not-found"}) def done true] []])] [def result (do {ok: false, error: "unknown-node-kind"}) def done true]]] []] end result]]

# --- decide ---

def decide fn [[model:Map input:Map] [Any] [if ((model get "kind") "table" eq) [def rules quote (model get "rules") def policy (model get "hit-policy") if (policy "first" eq) [input rules eval-table-first] [if (policy "unique" eq) [input rules eval-table-unique] [input rules eval-table-first]]] [if ((model get "kind") "tree" eq) [def nodes quote (model get "nodes") def current (nodes (model get "root") find-node) def done false def result (do {ok: false, error: "max-depth-exceeded"}) for 100 [def _i i if (done not) [if ((current get "kind") "leaf" eq) [def result (current get "result") def done true] [if ((current get "kind") "branch" eq) [(def next-id (input quote (current get "branches") find-branch-next) if (next-id None eq) [def result (do {ok: false, error: "no-branch-match"}) def done true] [def current (nodes next-id find-node) if (current None eq) [def result (do {ok: false, error: "node-not-found"}) def done true] []])] [def result (do {ok: false, error: "unknown-node-kind"}) def done true]]] []] end result] [do {ok: false, error: "unknown-model-kind"}]]]]

`

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
// deterministically from the native module's builder AQL (source of truth)
// plus the pure-AQL evaluators and export block.
func buildDecisionFileAQL() string {
	return nativemod.DecisionBuilderAQL() + decisionEvalAQL + decisionExportAQL
}

// buildDecisionModuleAQL generates the module [...] inline version
// from the same sources.
func buildDecisionModuleAQL() string {
	return "module [\n" + nativemod.DecisionBuilderAQL() + decisionEvalAQL +
		decisionExportAQL + "\n]\n"
}

// --- Test infrastructure ---

// decisionTestCase defines a single test for the decision module.
type decisionTestCase struct {
	name  string
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
// both the native Go module and the pure AQL file module.
// Tests that require collect hit-policy are marked and skipped for AQL.
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
		name: "EvalPredAllOf",
		setup: `def pred ([{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.all-of)`,
		expr:  `{age:25,score:80} pred decision.eval-pred`,
		check: checkBool(true),
	},
	{
		name: "EvalPredAllOfFalse",
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
		name: "TableNoMatch",
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

// Native-only tests (features only available with Go evaluators).
var decisionNativeOnlyTests = []decisionTestCase{
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

func TestNativeDecisionCollect(t *testing.T) {
	for _, tc := range decisionNativeOnlyTests {
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
	// builder AQL (source of truth) + pure AQL evaluators + export block.
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
	// Verify that the builder portion of the generated file exactly matches
	// the native module's builder AQL (single source of truth).
	content := buildDecisionFileAQL()
	builderAQL := nativemod.DecisionBuilderAQL()

	if !strings.Contains(content, strings.TrimSpace(builderAQL)) {
		t.Error("generated decision.aql does not contain the native module's builder AQL")
	}
}
