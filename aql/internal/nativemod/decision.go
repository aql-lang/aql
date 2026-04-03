package nativemod

import (
	"fmt"
	"sync"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
)

// decisionParseCache caches the parsed AQL tokens for the decision module
// source. The parse is done once on first import and reused thereafter.
var (
	decisionParseOnce sync.Once
	decisionParsed    []engine.Value
	decisionParseErr  error
)

// BuildDecisionModule creates the "aql:decision" native module.
// Builder functions are defined in pure AQL. Evaluators are Go-implemented
// to avoid CallAQL nesting limitations with recursive predicate evaluation.
// The AQL source is parsed once and cached for reuse across imports.
func BuildDecisionModule(parent *engine.Registry) (engine.ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return engine.ModuleDesc{}, fmt.Errorf("decision: parser not configured")
	}

	// Parse AQL source once, cache for reuse.
	decisionParseOnce.Do(func() {
		decisionParsed, decisionParseErr = parent.ParseFunc(decisionAQL)
	})
	if decisionParseErr != nil {
		return engine.ModuleDesc{}, fmt.Errorf("decision: parse error: %w", decisionParseErr)
	}

	// Create sub-registry with full builtins + native words.
	subReg, err := engine.DefaultRegistry()
	if err != nil {
		return engine.ModuleDesc{}, err
	}
	subReg.ParseFunc = parent.ParseFunc
	native.Register(subReg)

	// Register Go-implemented evaluators in the sub-registry.
	registerEvalCond(subReg)
	registerEvalPred(subReg)
	registerEvalTable(subReg)
	registerEvalTree(subReg)
	registerDecide(subReg)

	// Run the cached AQL tokens in the sub-registry (copy to avoid mutation).
	eng := engine.NewTop(subReg)
	_, err = eng.Run(append([]engine.Value(nil), decisionParsed...))
	if err != nil {
		return engine.ModuleDesc{}, fmt.Errorf("decision: execution error: %w", err)
	}

	// Tag FnDefs from AQL with the sub-registry (closure semantics).
	for name, stack := range subReg.DefStacks {
		for i, val := range stack {
			if fnDef, ok := val.Data.(engine.FnDefInfo); ok && fnDef.Registry == nil {
				fnDef.Registry = subReg
				subReg.DefStacks[name][i] = engine.NewFnDef(fnDef)
			}
		}
	}

	// Build export map.
	exports := engine.NewOrderedMap()

	// AQL-defined builders
	aqlExports := []string{
		"cond", "all-of", "any-of", "not-of",
		"make-rule", "make-table", "make-tree", "make-branch", "make-leaf",
		"with-policy",
	}
	for _, name := range aqlExports {
		stack := subReg.DefStacks[name]
		if len(stack) > 0 {
			exports.Set(name, stack[len(stack)-1])
		}
	}

	// Go-implemented evaluators as FnDef wrappers
	exports.Set("eval-cond", makeFnDef("eval-cond", []engine.FnParam{{Type: engine.TMap}, {Type: engine.TMap}}, []engine.Type{engine.TBoolean}, subReg))
	exports.Set("eval-pred", makeFnDef("eval-pred", []engine.FnParam{{Type: engine.TMap}, {Type: engine.TMap}}, []engine.Type{engine.TBoolean}, subReg))
	exports.Set("eval-table", makeFnDef("eval-table", []engine.FnParam{{Type: engine.TMap}, {Type: engine.TMap}}, []engine.Type{engine.TAny}, subReg))
	exports.Set("eval-tree", makeFnDef("eval-tree", []engine.FnParam{{Type: engine.TMap}, {Type: engine.TMap}}, []engine.Type{engine.TAny}, subReg))
	exports.Set("decide", makeFnDef("decide", []engine.FnParam{{Type: engine.TMap}, {Type: engine.TMap}}, []engine.Type{engine.TAny}, subReg))

	modID := parent.NextModuleID()
	desc := engine.ModuleDesc{
		ID:      modID,
		Exports: map[string]*engine.OrderedMap{"decision": exports},
	}
	return desc, nil
}

func makeFnDef(wordName string, params []engine.FnParam, returns []engine.Type, subReg *engine.Registry) engine.Value {
	// Give params names so CallAQL installs them as defs rather than
	// pushing unnamed tokens.  The body then pushes them in reverse order
	// so that the inner registered word's nearest-first matching sees
	// them in the original sig order (counteracts double reversal).
	named := make([]engine.FnParam, len(params))
	for i, p := range params {
		named[i] = engine.FnParam{Name: fmt.Sprintf("__p%d", i), Type: p.Type}
	}
	var body []engine.Value
	for i := len(named) - 1; i >= 0; i-- {
		body = append(body, engine.NewWord(named[i].Name))
	}
	body = append(body, engine.NewWord(wordName))
	return engine.NewFnDef(engine.FnDefInfo{
		Name: wordName,
		Sigs: []engine.FnSig{{
			Params:  named,
			Returns: returns,
			Body:    body,
		}},
		Registry: subReg,
	})
}

// --- Go evaluator: apply-op ---

func applyOp(op string, lhs, rhs engine.Value) (bool, error) {
	switch op {
	case "eq":
		return lhs.String() == rhs.String(), nil
	case "neq":
		return lhs.String() != rhs.String(), nil
	case "lt":
		lhsN, err := lhs.AsNumber()
		if err != nil {
			return false, err
		}
		rhsN, err := rhs.AsNumber()
		if err != nil {
			return false, err
		}
		return lhsN < rhsN, nil
	case "lte":
		lhsN, err := lhs.AsNumber()
		if err != nil {
			return false, err
		}
		rhsN, err := rhs.AsNumber()
		if err != nil {
			return false, err
		}
		return lhsN <= rhsN, nil
	case "gt":
		lhsN, err := lhs.AsNumber()
		if err != nil {
			return false, err
		}
		rhsN, err := rhs.AsNumber()
		if err != nil {
			return false, err
		}
		return lhsN > rhsN, nil
	case "gte":
		lhsN, err := lhs.AsNumber()
		if err != nil {
			return false, err
		}
		rhsN, err := rhs.AsNumber()
		if err != nil {
			return false, err
		}
		return lhsN >= rhsN, nil
	case "is_true":
		b, err := lhs.AsBoolean()
		if err != nil {
			return false, err
		}
		return b, nil
	case "is_false":
		b, err := lhs.AsBoolean()
		if err != nil {
			return false, err
		}
		return !b, nil
	case "is_null":
		return lhs.VType.Equal(engine.TNone), nil
	case "is_not_null":
		return !lhs.VType.Equal(engine.TNone), nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", op)
	}
}

// --- Go evaluator: eval-cond ---

func registerEvalCond(r *engine.Registry) {
	r.Register("eval-cond", engine.Signature{
		Args: []engine.Type{engine.TMap, engine.TMap},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			c := args[0].AsMap()
			input := args[1].AsMap()
			if c == nil || input == nil {
				return nil, fmt.Errorf("eval-cond: expected concrete maps")
			}
			return evalCondMap(c, input)
		},
	})
}

func evalCondMap(c engine.ReadMap, input engine.ReadMap) ([]engine.Value, error) {
	fieldVal, _ := c.Get("field")
	opVal, _ := c.Get("op")
	valueVal, _ := c.Get("value")

	fieldName := fieldVal.String()
	if fieldVal.IsAtom() {
		a, err := fieldVal.AsAtom()
		if err != nil {
			return nil, fmt.Errorf("eval-cond: field: %w", err)
		}
		fieldName = a
	}

	lhs, _ := input.Get(fieldName)
	op, err := opVal.AsString()
	if err != nil {
		return nil, fmt.Errorf("eval-cond: op: %w", err)
	}
	if opVal.IsAtom() {
		a, err := opVal.AsAtom()
		if err != nil {
			return nil, fmt.Errorf("eval-cond: op: %w", err)
		}
		op = a
	}

	result, err := applyOp(op, lhs, valueVal)
	if err != nil {
		return nil, fmt.Errorf("eval-cond: %w", err)
	}
	return []engine.Value{engine.NewBoolean(result)}, nil
}

// --- Go evaluator: eval-pred ---

func registerEvalPred(r *engine.Registry) {
	r.Register("eval-pred", engine.Signature{
		Args: []engine.Type{engine.TMap, engine.TMap},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			pred := args[0].AsMap()
			input := args[1].AsMap()
			if pred == nil || input == nil {
				return nil, fmt.Errorf("eval-pred: expected concrete maps")
			}
			result, err := evalPredMap(pred, input)
			if err != nil {
				return nil, err
			}
			return []engine.Value{engine.NewBoolean(result)}, nil
		},
	})
}

func evalPredMap(pred engine.ReadMap, input engine.ReadMap) (bool, error) {
	kindVal, hasKind := pred.Get("kind")
	if hasKind {
		kindStr, err := kindVal.AsString()
		if err != nil {
			return false, fmt.Errorf("eval-pred: kind: %w", err)
		}
		hasKind = kindStr == "group"
	}
	if hasKind {
		opVal, _ := pred.Get("op")
		op, err := opVal.AsString()
		if err != nil {
			return false, fmt.Errorf("eval-pred: op: %w", err)
		}
		if opVal.IsAtom() {
			a, err := opVal.AsAtom()
			if err != nil {
				return false, fmt.Errorf("eval-pred: op: %w", err)
			}
			op = a
		}
		childrenVal, _ := pred.Get("children")
		children := childrenVal.AsList()
		singleChild := false
		if children.IsNil() && childrenVal.AsMap() != nil {
			singleChild = true
		}

		switch op {
		case "all":
			for i := 0; i < children.Len(); i++ {
				child := children.Get(i)
				childMap := child.AsMap()
				if childMap == nil {
					return false, fmt.Errorf("eval-pred: child %d is not a map", i)
				}
				r, err := evalPredMap(childMap, input)
				if err != nil {
					return false, err
				}
				if !r {
					return false, nil
				}
			}
			return true, nil
		case "any":
			for i := 0; i < children.Len(); i++ {
				child := children.Get(i)
				childMap := child.AsMap()
				if childMap == nil {
					return false, fmt.Errorf("eval-pred: child %d is not a map", i)
				}
				r, err := evalPredMap(childMap, input)
				if err != nil {
					return false, err
				}
				if r {
					return true, nil
				}
			}
			return false, nil
		case "not":
			var childMap engine.ReadMap
			if singleChild {
				childMap = childrenVal.AsMap()
			} else if children.Len() > 0 {
				childMap = children.Get(0).AsMap()
			}
			if childMap == nil {
				return false, fmt.Errorf("eval-pred: child is not a map")
			}
			r, err := evalPredMap(childMap, input)
			if err != nil {
				return false, err
			}
			return !r, nil
		default:
			return false, fmt.Errorf("eval-pred: unknown group operator %q", op)
		}
	}

	// Atomic condition
	res, err := evalCondMap(pred, input)
	if err != nil {
		return false, err
	}
	b, err := res[0].AsBoolean()
	if err != nil {
		return false, err
	}
	return b, nil
}

// --- Go evaluator: eval-table ---

func registerEvalTable(r *engine.Registry) {
	r.Register("eval-table", engine.Signature{
		Args: []engine.Type{engine.TMap, engine.TMap},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			table := args[0].AsMap()
			input := args[1].AsMap()
			if table == nil || input == nil {
				return nil, fmt.Errorf("eval-table: expected concrete maps")
			}
			return evalTableMap(table, input)
		},
	})
}

func evalTableMap(table engine.ReadMap, input engine.ReadMap) ([]engine.Value, error) {
	rulesVal, _ := table.Get("rules")
	rules := rulesVal.AsList()
	policyVal, _ := table.Get("hit-policy")
	policy, err := policyVal.AsString()
	if err != nil {
		return nil, fmt.Errorf("eval-table: hit-policy: %w", err)
	}
	if policyVal.IsAtom() {
		a, err := policyVal.AsAtom()
		if err != nil {
			return nil, fmt.Errorf("eval-table: hit-policy: %w", err)
		}
		policy = a
	}

	var matched []engine.Value
	for i := 0; i < rules.Len(); i++ {
		rule := rules.Get(i)
		ruleMap := rule.AsMap()
		if ruleMap == nil {
			continue
		}
		whenVal, _ := ruleMap.Get("when")
		whenMap := whenVal.AsMap()
		if whenMap == nil {
			continue
		}
		match, err := evalPredMap(whenMap, input)
		if err != nil {
			return nil, fmt.Errorf("eval-table: rule %d: %w", i, err)
		}
		if match {
			matched = append(matched, rule)
		}
	}

	nMatched := len(matched)
	noMatch := engine.NewMap(makeErrorMap("no-match"))

	switch policy {
	case "first":
		if nMatched > 0 {
			thenVal, _ := matched[0].AsMap().Get("then")
			return []engine.Value{thenVal}, nil
		}
		return []engine.Value{noMatch}, nil

	case "unique":
		if nMatched == 1 {
			thenVal, _ := matched[0].AsMap().Get("then")
			return []engine.Value{thenVal}, nil
		}
		if nMatched == 0 {
			return []engine.Value{noMatch}, nil
		}
		return []engine.Value{engine.NewMap(makeErrorMap("multiple-matches"))}, nil

	case "collect":
		results := make([]engine.Value, nMatched)
		for i, m := range matched {
			thenVal, _ := m.AsMap().Get("then")
			results[i] = thenVal
		}
		return []engine.Value{engine.NewList(results)}, nil

	case "priority":
		if nMatched == 0 {
			return []engine.Value{noMatch}, nil
		}
		best := matched[0]
		bestPri := int64(0)
		if m := best.AsMap(); m != nil {
			if p, ok := m.Get("priority"); ok {
				v, err := p.AsInteger()
				if err != nil {
					return nil, fmt.Errorf("eval-table: priority: %w", err)
				}
				bestPri = v
			}
		}
		for _, m := range matched[1:] {
			if mm := m.AsMap(); mm != nil {
				if p, ok := mm.Get("priority"); ok {
					v, err := p.AsInteger()
					if err != nil {
						return nil, fmt.Errorf("eval-table: priority: %w", err)
					}
					if v > bestPri {
						best = m
						bestPri = v
					}
				}
			}
		}
		thenVal, _ := best.AsMap().Get("then")
		return []engine.Value{thenVal}, nil

	default:
		if nMatched > 0 {
			thenVal, _ := matched[0].AsMap().Get("then")
			return []engine.Value{thenVal}, nil
		}
		return []engine.Value{noMatch}, nil
	}
}

func makeErrorMap(errMsg string) *engine.OrderedMap {
	m := engine.NewOrderedMap()
	m.Set("ok", engine.NewBoolean(false))
	m.Set("error", engine.NewString(errMsg))
	return m
}

// --- Go evaluator: eval-tree ---

func registerEvalTree(r *engine.Registry) {
	r.Register("eval-tree", engine.Signature{
		Args: []engine.Type{engine.TMap, engine.TMap},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			tree := args[0].AsMap()
			input := args[1].AsMap()
			if tree == nil || input == nil {
				return nil, fmt.Errorf("eval-tree: expected concrete maps")
			}
			return evalTreeMap(tree, input)
		},
	})
}

func evalTreeMap(tree engine.ReadMap, input engine.ReadMap) ([]engine.Value, error) {
	rootVal, _ := tree.Get("root")
	nodesVal, _ := tree.Get("nodes")
	nodes := nodesVal.AsList()

	rootID := rootVal.String()
	if rootVal.IsAtom() {
		a, err := rootVal.AsAtom()
		if err != nil {
			return nil, fmt.Errorf("eval-tree: root: %w", err)
		}
		rootID = a
	}

	currentID := rootID
	for depth := 0; depth < 100; depth++ {
		node := findNodeByID(currentID, nodes)
		if node == nil {
			return []engine.Value{engine.NewMap(makeErrorMap("node-not-found"))}, nil
		}

		kindVal, _ := node.Get("kind")
		kind, err := kindVal.AsString()
		if err != nil {
			return nil, fmt.Errorf("eval-tree: kind: %w", err)
		}
		if kindVal.IsAtom() {
			a, err := kindVal.AsAtom()
			if err != nil {
				return nil, fmt.Errorf("eval-tree: kind: %w", err)
			}
			kind = a
		}

		switch kind {
		case "leaf":
			resultVal, _ := node.Get("result")
			return []engine.Value{resultVal}, nil

		case "branch":
			branchesVal, _ := node.Get("branches")
			branches := branchesVal.AsList()
			nextID := ""
			for i := 0; i < branches.Len(); i++ {
				br := branches.Get(i)
				brMap := br.AsMap()
				if brMap == nil {
					continue
				}
				whenVal, _ := brMap.Get("when")
				whenMap := whenVal.AsMap()
				if whenMap == nil {
					continue
				}
				match, err := evalPredMap(whenMap, input)
				if err != nil {
					return nil, fmt.Errorf("eval-tree: branch %d: %w", i, err)
				}
				if match {
					nextVal, _ := brMap.Get("next")
					nextID = nextVal.String()
					if nextVal.IsAtom() {
						a, err := nextVal.AsAtom()
						if err != nil {
							return nil, fmt.Errorf("eval-tree: next: %w", err)
						}
						nextID = a
					}
					break
				}
			}
			if nextID == "" {
				return []engine.Value{engine.NewMap(makeErrorMap("no-branch-match"))}, nil
			}
			currentID = nextID

		default:
			return []engine.Value{engine.NewMap(makeErrorMap("unknown-node-kind"))}, nil
		}
	}
	return []engine.Value{engine.NewMap(makeErrorMap("max-depth-exceeded"))}, nil
}

func findNodeByID(id string, nodes engine.ReadList) engine.ReadMap {
	for i := 0; i < nodes.Len(); i++ {
		node := nodes.Get(i)
		nodeMap := node.AsMap()
		if nodeMap == nil {
			continue
		}
		idVal, _ := nodeMap.Get("id")
		nodeID := idVal.String()
		if idVal.IsAtom() {
			a, _ := idVal.AsAtom()
			nodeID = a
		}
		if nodeID == id {
			return nodeMap
		}
	}
	return nil
}

// --- Go evaluator: decide ---

func registerDecide(r *engine.Registry) {
	r.Register("decide", engine.Signature{
		Args: []engine.Type{engine.TMap, engine.TMap},
		Handler: func(args []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			model := args[0].AsMap()
			input := args[1].AsMap()
			if model == nil || input == nil {
				return nil, fmt.Errorf("decide: expected concrete maps")
			}
			kindVal, _ := model.Get("kind")
			kind, err := kindVal.AsString()
			if err != nil {
				return nil, fmt.Errorf("decide: kind: %w", err)
			}
			if kindVal.IsAtom() {
				a, err := kindVal.AsAtom()
				if err != nil {
					return nil, fmt.Errorf("decide: kind: %w", err)
				}
				kind = a
			}
			switch kind {
			case "table":
				return evalTableMap(model, input)
			case "tree":
				return evalTreeMap(model, input)
			default:
				return []engine.Value{engine.NewMap(makeErrorMap("unknown-model-kind"))}, nil
			}
		},
	})
}

// decisionAQL contains the AQL source for record types and builder functions.
// The evaluators are Go-implemented due to CallAQL nesting limitations
// with recursive predicate evaluation.
//
// This is the single source of truth for decision module builder AQL.
// The pure-AQL file module and module [...] tests are generated from this.
const decisionAQL = `

# ============================================================
# aql:decision — Record types and builder functions
# ============================================================

type Cond record [field:Atom op:String value:Any]
type Pred record [kind:String op:String children:Any]
type Rule record [when:Map then:Map]
type DTable record [kind:String rules:List hit-policy:String]
type BranchNode record [id:Atom kind:String branches:List]
type LeafNode record [id:Atom kind:String result:Any]
type DTree record [kind:String root:Atom nodes:List]

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

`
