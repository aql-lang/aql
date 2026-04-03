package test

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

func decisionAQLRegistry(t *testing.T) *engine.Registry {
	t.Helper()
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)

	src := decisionModuleAQL + `
import decision
`
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	eng := engine.NewTop(r)
	_, err = eng.Run(vals)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return r
}

func runDecAQL(t *testing.T, r *engine.Registry, src string) []engine.Value {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	eng := engine.NewTop(r)
	result, err := eng.Run(vals)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return result
}

// --- Tests ---

func TestAQLDecisionCond(t *testing.T) {
	r := decisionAQLRegistry(t)
	result := runDecAQL(t, r, `18 "gte" age decision.cond`)
	m := result[0].AsMap()
	if m == nil {
		t.Fatalf("expected map, got %s", result[0].VType.String())
	}
	field, _ := m.Get("field")
	if field.String() != "age" {
		t.Errorf("field = %v, want age", field)
	}
}

func TestAQLDecisionEvalCondTrue(t *testing.T) {
	r := decisionAQLRegistry(t)
	result := runDecAQL(t, r, `{field:age,op:"gte",value:18} {age:25} decision.eval-cond`)
	if !result[0].AsBoolean() {
		t.Error("expected true for age=25 gte 18")
	}
}

func TestAQLDecisionEvalCondFalse(t *testing.T) {
	r := decisionAQLRegistry(t)
	result := runDecAQL(t, r, `{field:age,op:"gte",value:18} {age:15} decision.eval-cond`)
	if result[0].AsBoolean() {
		t.Error("expected false for age=15 gte 18")
	}
}

func TestAQLDecisionEvalPredAllOf(t *testing.T) {
	r := decisionAQLRegistry(t)
	result := runDecAQL(t, r, `
		[{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.all-of
		{age:25,score:80}
		decision.eval-pred
	`)
	if !result[0].AsBoolean() {
		t.Error("expected all-of true")
	}
}

func TestAQLDecisionEvalPredNotOf(t *testing.T) {
	r := decisionAQLRegistry(t)
	result := runDecAQL(t, r, `
		({field:age,op:"lt",value:18} decision.not-of)
		{age:25}
		decision.eval-pred
	`)
	if !result[0].AsBoolean() {
		t.Error("expected not-of true for age=25")
	}
}

func TestAQLDecisionTableFirst(t *testing.T) {
	r := decisionAQLRegistry(t)
	result := runDecAQL(t, r, `
		def tbl ([
			{when:{field:age,op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:age,op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)
		tbl {age:25} decision.eval-table
	`)
	m := result[0].AsMap()
	cat, _ := m.Get("category")
	if cat.AsString() != "adult" {
		t.Errorf("expected adult, got %v", cat)
	}
}

func TestAQLDecisionTree(t *testing.T) {
	r := decisionAQLRegistry(t)
	result := runDecAQL(t, r, `
		def tree ({kind:"tree", root:root, nodes:[
			{id:root, kind:"branch", branches:[
				{when:{field:age,op:"lt",value:18}, next:minor}
				{when:{field:age,op:"gte",value:18}, next:adult}
			]}
			{id:minor, kind:"leaf", result:{category:"minor"}}
			{id:adult, kind:"leaf", result:{category:"adult"}}
		]})
		tree {age:25} decision.eval-tree
	`)
	m := result[0].AsMap()
	cat, _ := m.Get("category")
	if cat.AsString() != "adult" {
		t.Errorf("expected adult, got %v", cat)
	}
}

func TestAQLDecisionDecide(t *testing.T) {
	r := decisionAQLRegistry(t)
	result := runDecAQL(t, r, `
		def model ({kind:"table", hit-policy:"first", rules:[
			{when:{field:x,op:"gt",value:0}, then:{sign:"positive"}}
			{when:{field:x,op:"lt",value:0}, then:{sign:"negative"}}
		]})
		model {x:5} decision.decide
	`)
	m := result[0].AsMap()
	sign, _ := m.Get("sign")
	if sign.AsString() != "positive" {
		t.Errorf("expected positive, got %v", sign)
	}
}

// decisionModuleAQL is the pure AQL decision module loaded via module [...].
const decisionModuleAQL = `
module [

# --- Builders ---

def cond fn [[field:Atom op:String value:Any] [Map] [do {field: field, op: op, value: value}]]

def all-of fn [[children:List] [Map] [do {kind: "group", op: "all", children: children}]]

def any-of fn [[children:List] [Map] [do {kind: "group", op: "any", children: children}]]

def not-of fn [[child:Map] [Map] [do {kind: "group", op: "not", children: child}]]

def make-rule fn [[when:Map then:Map] [Map] [do {when: when, then: then}]]

def make-table fn [[rules:List] [Map] [do {kind: "table", rules: rules, hit-policy: "first"}]]

def with-policy fn [[policy:String table:Map] [Map] [def rules (table.rules) def kind (table.kind) do {kind: kind, rules: rules, hit-policy: policy}]]

def make-branch fn [[id:Atom branches:List] [Map] [do {id: id, kind: "branch", branches: branches}]]

def make-leaf fn [[id:Atom result:Any] [Map] [do {id: id, kind: "leaf", result: result}]]

def make-tree fn [[root:Atom nodes:List] [Map] [do {kind: "tree", root: root, nodes: nodes}]]

# --- apply-op ---

def apply-op fn [[rhs:Any op:String lhs:Any] [Boolean] [if (op "eq" eq) [lhs rhs eq] [if (op "neq" eq) [lhs rhs neq] [if (op "lt" eq) [lhs rhs lt] [if (op "lte" eq) [lhs rhs lte] [if (op "gt" eq) [lhs rhs gt] [if (op "gte" eq) [lhs rhs gte] [false]]]]]]]]

def apply-op fn [[rhs:Any op:String] [Boolean] [if (op "is_true" eq) [rhs] [if (op "is_false" eq) [rhs not] [if (op "is_null" eq) [false] [if (op "is_not_null" eq) [true] [false]]]]]]

# --- eval-cond ---

def eval-cond fn [[c:Map input:Map] [Boolean] [input.((c.field convert String)) c.op c.value apply-op]]

# --- eval-pred helpers ---

def eval-pred-all fn [[children:List input:Map] [Boolean] [def result true for (children length) [def idx i if ((children idx get) input eval-pred not) [def result false] []] end result]]

def eval-pred-any fn [[children:List input:Map] [Boolean] [def result false for (children length) [def idx i if ((children idx get) input eval-pred) [def result true] []] end result]]

def eval-pred-not fn [[children:Map input:Map] [Boolean] [children input eval-cond not]]
def eval-pred-not fn [[children:List input:Map] [Boolean] [(children 0 get) input eval-pred not]]

# --- eval-pred ---

def eval-pred fn [[pred:Map input:Map] [Boolean] [if ((pred get "kind") "group" eq) [(def group-op (pred get "op") def children quote (pred get "children") if (group-op "all" eq) [children input eval-pred-all] [if (group-op "any" eq) [children input eval-pred-any] [if (group-op "not" eq) [children input eval-pred-not] [false]]])] [pred input eval-cond]]]

# --- eval-table helpers ---

def eval-table-first fn [[rules:List input:Map] [Any] [def result (do {ok: false, error: "no-match"}) def found false for (rules length) [def idx i def rule (rules idx get) if (found not) [if ((rule get "when") input eval-pred) [def result (rule get "then") def found true] []] []] end result]]

def eval-table-unique fn [[rules:List input:Map] [Any] [def result (do {ok: false, error: "no-match"}) def match-count 0 for (rules length) [def idx i def rule (rules idx get) if ((rule get "when") input eval-pred) [def result (rule get "then") def match-count (match-count 1 add)] []] end if (match-count 1 eq) [result] [if (match-count 0 eq) [do {ok: false, error: "no-match"}] [do {ok: false, error: "multiple-matches"}]]]]

# --- eval-table ---

def eval-table fn [[table:Map input:Map] [Any] [def rules quote (table get "rules") def policy (table get "hit-policy") if (policy "first" eq) [rules input eval-table-first] [if (policy "unique" eq) [rules input eval-table-unique] [rules input eval-table-first]]]]

# --- eval-tree helpers ---

def find-node fn [[id:Any nodes:List] [Any] [def found None for (nodes length) [def idx i def node (nodes idx get) if ((node get "id" convert String) (id convert String) eq) [def found node] []] end found]]

def find-branch-next fn [[branches:List input:Map] [Any] [def next-id None for (branches length) [def idx i def br (branches idx get) if ((br get "when") input eval-pred) [def next-id (br get "next")] []] end next-id]]

# --- eval-tree ---

def eval-tree fn [[tree:Map input:Map] [Any] [def nodes quote (tree get "nodes") def current ((tree get "root") nodes find-node) def done false def result (do {ok: false, error: "max-depth-exceeded"}) for 100 [def _i i if (done not) [if ((current get "kind") "leaf" eq) [def result (current get "result") def done true] [if ((current get "kind") "branch" eq) [(def next-id (quote (current get "branches") input find-branch-next) if (next-id None eq) [def result (do {ok: false, error: "no-branch-match"}) def done true] [def current (next-id nodes find-node) if (current None eq) [def result (do {ok: false, error: "node-not-found"}) def done true] []])] [def result (do {ok: false, error: "unknown-node-kind"}) def done true]]] []] end result]]

# --- decide ---

def decide fn [[model:Map input:Map] [Any] [if ((model get "kind") "table" eq) [def rules quote (model get "rules") def policy (model get "hit-policy") if (policy "first" eq) [rules input eval-table-first] [if (policy "unique" eq) [rules input eval-table-unique] [rules input eval-table-first]]] [if ((model get "kind") "tree" eq) [def nodes quote (model get "nodes") def current ((model get "root") nodes find-node) def done false def result (do {ok: false, error: "max-depth-exceeded"}) for 100 [def _i i if (done not) [if ((current get "kind") "leaf" eq) [def result (current get "result") def done true] [if ((current get "kind") "branch" eq) [(def next-id (quote (current get "branches") input find-branch-next) if (next-id None eq) [def result (do {ok: false, error: "no-branch-match"}) def done true] [def current (next-id nodes find-node) if (current None eq) [def result (do {ok: false, error: "node-not-found"}) def done true] []])] [def result (do {ok: false, error: "unknown-node-kind"}) def done true]]] []] end result] [do {ok: false, error: "unknown-model-kind"}]]]]

# --- exports ---

export decision {cond: cond, all-of: all-of, any-of: any-of, not-of: not-of, make-rule: make-rule, make-table: make-table, with-policy: with-policy, make-branch: make-branch, make-leaf: make-leaf, make-tree: make-tree, apply-op: apply-op, eval-cond: eval-cond, eval-pred: eval-pred, eval-table: eval-table, eval-tree: eval-tree, decide: decide}

]
`
