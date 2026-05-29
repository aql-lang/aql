package modules

import (
	"fmt"
	"sync"

	"github.com/aql-lang/aql/lang/go/native"
)

// decisionParseCache caches the parsed AQL tokens for the decision module
// source. The parse is done once on first import and reused thereafter.
var (
	decisionParseOnce sync.Once
	decisionParsed    []native.Value
	decisionParseErr  error
)

// BuildDecisionModule creates the "aql:decision" native module.
// All functionality is implemented in pure AQL — record types, builders,
// evaluators, and exports. The AQL source is parsed once and cached;
// execution and export collection are handled by RunModuleBody.
func BuildDecisionModule(parent *native.Registry) (native.ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return native.ModuleDesc{}, fmt.Errorf("decision: parser not configured")
	}

	// Parse AQL source once, cache for reuse.
	decisionParseOnce.Do(func() {
		decisionParsed, decisionParseErr = parent.ParseFunc(decisionAQL)
	})
	if decisionParseErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("decision: parse error: %w", decisionParseErr)
	}

	// Ensure native words (push, etc.) are available inside the module.
	if parent.Modules.InitFunc == nil {
		native.Register(parent)
	}

	// Copy tokens to avoid mutation, then let RunModuleBody handle
	// registry setup, execution, export collection, and FnDef tagging.
	tokens := append([]native.Value(nil), decisionParsed...)
	return native.RunModuleBody(parent, tokens)
}

// decisionAQL contains the complete AQL source for the decision module.
// This is the single source of truth — the pure-AQL file module and
// module [...] inline tests are generated from this.
const decisionAQL = `

# ============================================================
# aql:decision — Record types and builder functions
# ============================================================

def Cond refine Record [field:Atom op:String value:Any]
def Pred refine Record [kind:String op:String children:Any]
def Rule refine Record [when:Map then:Map]
def DTable refine Record [kind:String rules:List hit-policy:String]
def BranchNode refine Record [id:Atom kind:String branches:List]
def LeafNode refine Record [id:Atom kind:String result:Any]
def DTree refine Record [kind:String root:Atom nodes:List]

def cond fn [[field:Atom op:String value:Any] [Map] [
  make Cond {field:field op:op value:value}
]]

def all-of fn [[children:List] [Map] [
  make Pred {kind:"group" op:"all" children:children}
]]

def any-of fn [[children:List] [Map] [
  make Pred {kind:"group" op:"any" children:children}
]]

def not-of fn [[child:Map] [Map] [
  make Pred {kind:"group" op:"not" children:child}
]]

def make-rule fn [[when:Map then:Map] [Map] [
  make Rule {when:when then:then}
]]

def make-table fn [[rules:List] [Map] [
  make DTable {kind:"table" rules:rules hit-policy:"first"}
]]

def with-policy fn [[policy:String table:Map] [Map] [
  make DTable {kind:(table.kind) rules:(table.rules) hit-policy:policy}
]]

def make-branch fn [[id:Atom branches:List] [Map] [
  make BranchNode {id:id kind:"branch" branches:branches}
]]

def make-leaf fn [[id:Atom result:Any] [Map] [
  make LeafNode {id:id kind:"leaf" result:result}
]]

def make-tree fn [[root:Atom nodes:List] [Map] [
  make DTree {kind:"tree" root:root nodes:nodes}
]]

# ============================================================
# aql:decision — Evaluators
# ============================================================

# --- apply-op ---

def apply-op fn [[rhs:Any op:String lhs:Any] [Boolean] [if (op "eq" eq) [lhs rhs eq] [if (op "neq" eq) [lhs rhs neq] [if (op "lt" eq) [lhs rhs lt] [if (op "lte" eq) [lhs rhs lte] [if (op "gt" eq) [lhs rhs gt] [if (op "gte" eq) [lhs rhs gte] [false]]]]]]]]

def apply-op fn [[rhs:Any op:String] [Boolean] [if (op "is_true" eq) [rhs] [if (op "is_false" eq) [rhs not] [if (op "is_null" eq) [false] [if (op "is_not_null" eq) [true] [false]]]]]]

# --- eval-cond ---

def eval-cond fn [[c:Map input:Map] [Boolean] [input.((c.field convert String)) c.op c.value apply-op]]

# --- eval-pred helpers ---
# eval-pred-all/any fold per-child eval-pred results with the matching
# list-quantifier word. The earlier implementations used a manual
# for-loop with a mutable result flag; the new versions rely on
# each + all/any, which short-circuit at the quantifier.

def eval-pred-all fn [[children:List input:Map] [Boolean] [(children each [input swap eval-pred]) all]]

def eval-pred-any fn [[children:List input:Map] [Boolean] [(children each [input swap eval-pred]) any]]

def eval-pred-not fn [[children:Map input:Map] [Boolean] [input children eval-cond not]]
def eval-pred-not fn [[children:List input:Map] [Boolean] [input (children 0 get) eval-pred not]]

# --- eval-pred ---

def eval-pred fn [[pred:Map input:Map] [Boolean] [if ((pred get "kind") "group" eq) [(def group-op (pred get "op") def children quote (pred get "children") if (group-op "all" eq) [input children eval-pred-all] [if (group-op "any" eq) [input children eval-pred-any] [if (group-op "not" eq) [input children eval-pred-not] [false]]])] [input pred eval-cond]]]

# --- eval-table helpers ---

def eval-table-first fn [[rules:List input:Map] [Any] [def result (do {ok: false, error: "no-match"}) def found false for (rules size) [def idx i def rule (rules idx get) if (found not) [if (input (rule get "when") eval-pred) [def result (rule get "then") def found true] []] []] end result]]

def eval-table-unique fn [[rules:List input:Map] [Any] [def result (do {ok: false, error: "no-match"}) def match-count 0 for (rules size) [def idx i def rule (rules idx get) if (input (rule get "when") eval-pred) [def result (rule get "then") def match-count (match-count 1 add)] []] end if (match-count 1 eq) [result] [if (match-count 0 eq) [do {ok: false, error: "no-match"}] [do {ok: false, error: "multiple-matches"}]]]]

def eval-table-collect fn [[rules:List input:Map] [Any] [def results quote [] for (rules size) [def idx i def rule (rules idx get) if (input (rule get "when") eval-pred) [def results (quote (results (rule get "then") push))] []] end results]]

def eval-table-priority fn [[rules:List input:Map] [Any] [def best (do {ok: false, error: "no-match"}) def best-pri 0 def found false for (rules size) [def idx i def rule (rules idx get) if (input (rule get "when") eval-pred) [def pri (if ((rule get "priority") None neq) [(rule get "priority")] [0]) if (found not) [def best (rule get "then") def best-pri pri def found true] [if (pri best-pri gt) [def best (rule get "then") def best-pri pri] []]] []] end best]]

# --- eval-table ---

def eval-table fn [[table:Map input:Map] [Any] [def rules quote (table get "rules") def policy (table get "hit-policy") if (policy "first" eq) [input rules eval-table-first] [if (policy "unique" eq) [input rules eval-table-unique] [if (policy "collect" eq) [input rules eval-table-collect] [if (policy "priority" eq) [input rules eval-table-priority] [input rules eval-table-first]]]]]]

# --- eval-tree helpers ---

def find-node fn [[id:Any nodes:List] [Any] [def found None for (nodes size) [def idx i def node (nodes idx get) if ((node get "id" convert String) (id convert String) eq) [def found node] []] end found]]

def find-branch-next fn [[branches:List input:Map] [Any] [def next-id None for (branches size) [def idx i def br (branches idx get) if (input (br get "when") eval-pred) [def next-id (br get "next")] []] end next-id]]

# --- eval-tree ---

def eval-tree fn [[tree:Map input:Map] [Any] [def nodes quote (tree get "nodes") def current (nodes (tree get "root") find-node) def done false def result (do {ok: false, error: "max-depth-exceeded"}) for 100 [def _i i if (done not) [if ((current get "kind") "leaf" eq) [def result (current get "result") def done true] [if ((current get "kind") "branch" eq) [(def next-id (input quote (current get "branches") find-branch-next) if (next-id None eq) [def result (do {ok: false, error: "no-branch-match"}) def done true] [def current (nodes next-id find-node) if (current None eq) [def result (do {ok: false, error: "node-not-found"}) def done true] []])] [def result (do {ok: false, error: "unknown-node-kind"}) def done true]]] []] end result]]

# --- decide ---

def decide fn [[model:Map input:Map] [Any] [if ((model get "kind") "table" eq) [input model eval-table] [if ((model get "kind") "tree" eq) [input model eval-tree] [do {ok: false, error: "unknown-model-kind"}]]]]

# ============================================================
# aql:decision — Exports
# ============================================================

export "decision" {
  Cond:        Cond
  Pred:        Pred
  Rule:        Rule
  DTable:      DTable
  BranchNode:  BranchNode
  LeafNode:    LeafNode
  DTree:       DTree
  cond:        cond/r
  all-of:      all-of/r
  any-of:      any-of/r
  not-of:      not-of/r
  make-rule:   make-rule/r
  make-table:  make-table/r
  with-policy: with-policy/r
  make-branch: make-branch/r
  make-leaf:   make-leaf/r
  make-tree:   make-tree/r
  apply-op:    apply-op/r
  eval-cond:   eval-cond/r
  eval-pred:   eval-pred/r
  eval-table:  eval-table/r
  eval-tree:   eval-tree/r
  decide:      decide/r
}

`
