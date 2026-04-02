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
	result := runDecisionAQL(t, r, `age "gte" 18 decision.cond`)
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

func TestDecisionEvalCondTrue(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `{field:age,op:"gte",value:18} {age:25} decision.eval-cond`)
	if !result[0].AsBoolean() {
		t.Error("expected true for age=25 gte 18")
	}
}

func TestDecisionEvalCondFalse(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `{field:age,op:"gte",value:18} {age:15} decision.eval-cond`)
	if result[0].AsBoolean() {
		t.Error("expected false for age=15 gte 18")
	}
}

func TestDecisionEvalCondEq(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `{field:status,op:"eq",value:"active"} {status:"active"} decision.eval-cond`)
	if !result[0].AsBoolean() {
		t.Error("expected true for status eq active")
	}
}

// --- Predicate evaluation ---

func TestDecisionEvalPredAllOf(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		([{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.all-of)
		{age:25,score:80}
		decision.eval-pred
	`)
	if !result[0].AsBoolean() {
		t.Error("expected all-of true for age=25,score=80")
	}
}

func TestDecisionEvalPredAllOfFalse(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		([{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.all-of)
		{age:25,score:30}
		decision.eval-pred
	`)
	if result[0].AsBoolean() {
		t.Error("expected all-of false for age=25,score=30")
	}
}

func TestDecisionEvalPredAnyOf(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		([{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.any-of)
		{age:10,score:80}
		decision.eval-pred
	`)
	if !result[0].AsBoolean() {
		t.Error("expected any-of true for age=10,score=80")
	}
}

func TestDecisionEvalPredNotOf(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		({field:age,op:"lt",value:18} decision.not-of)
		{age:25}
		decision.eval-pred
	`)
	if !result[0].AsBoolean() {
		t.Error("expected not-of(age lt 18) true for age=25")
	}
}

// --- Decision table ---

func TestDecisionTableFirst(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def table ([
			{when:{field:age,op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:age,op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)
		table {age:25} decision.eval-table
	`)
	m := result[0].AsMap()
	cat, _ := m.Get("category")
	if cat.AsString() != "adult" {
		t.Errorf("expected adult, got %v", cat)
	}
}

func TestDecisionTableFirstMinor(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def table ([
			{when:{field:age,op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:age,op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)
		table {age:12} decision.eval-table
	`)
	m := result[0].AsMap()
	cat, _ := m.Get("category")
	if cat.AsString() != "minor" {
		t.Errorf("expected minor, got %v", cat)
	}
}

func TestDecisionTableUnique(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def table ("unique" ([
			{when:{field:score,op:"lt",value:50}, then:{grade:"fail"}}
			{when:{field:score,op:"gte",value:50}, then:{grade:"pass"}}
		] decision.make-table) decision.with-policy)
		table {score:75} decision.eval-table
	`)
	m := result[0].AsMap()
	grade, _ := m.Get("grade")
	if grade.AsString() != "pass" {
		t.Errorf("expected pass, got %v", grade)
	}
}

func TestDecisionTableCollect(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def table ("collect" ([
			{when:{field:age,op:"gte",value:18}, then:{perk:"vote"}}
			{when:{field:age,op:"gte",value:21}, then:{perk:"drink"}}
		] decision.make-table) decision.with-policy)
		table {age:25} decision.eval-table
	`)
	list := result[0].AsList()
	if list.Len() != 2 {
		t.Fatalf("expected 2 collected, got %d: %v", list.Len(), result[0])
	}
}

func TestDecisionTableNoMatch(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def table ([{when:{field:age,op:"gt",value:100}, then:{x:1}}] decision.make-table)
		table {age:25} decision.eval-table
	`)
	m := result[0].AsMap()
	errVal, _ := m.Get("error")
	if errVal.AsString() != "no-match" {
		t.Errorf("expected no-match error, got %v", result[0])
	}
}

func TestDecisionTableCompound(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def table ({kind:"table", hit-policy:"first", rules:[
			{when:{kind:"group",op:"all",children:[{field:age,op:"lt",value:30} {field:score,op:"gte",value:90}]}, then:{tier:"premium"}}
			{when:{field:score,op:"gte",value:50}, then:{tier:"standard"}}
		]})
		table {age:25,score:95} decision.eval-table
	`)
	m := result[0].AsMap()
	tier, _ := m.Get("tier")
	if tier.AsString() != "premium" {
		t.Errorf("expected premium, got %v", tier)
	}
}

// --- Decision tree ---

func TestDecisionTree(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
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

func TestDecisionTreeMinor(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def tree ({kind:"tree", root:root, nodes:[
			{id:root, kind:"branch", branches:[
				{when:{field:age,op:"lt",value:18}, next:minor}
				{when:{field:age,op:"gte",value:18}, next:adult}
			]}
			{id:minor, kind:"leaf", result:"too-young"}
			{id:adult, kind:"leaf", result:"welcome"}
		]})
		tree {age:12} decision.eval-tree
	`)
	if result[0].AsString() != "too-young" {
		t.Errorf("expected too-young, got %v", result[0])
	}
}

func TestDecisionTreeMultiLevel(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def tree ({kind:"tree", root:check-age, nodes:[
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
		]})
		tree {age:25,score:90} decision.eval-tree
	`)
	if result[0].AsString() != "approved" {
		t.Errorf("expected approved, got %v", result[0])
	}
}

// --- decide (unified) ---

func TestDecideTable(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
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

func TestDecideTree(t *testing.T) {
	r := decisionRegistry(t)
	result := runDecisionAQL(t, r, `
		def model ({kind:"tree", root:root, nodes:[
			{id:root, kind:"branch", branches:[
				{when:{field:temp,op:"gt",value:30}, next:hot}
				{when:{field:temp,op:"lt",value:10}, next:cold}
			]}
			{id:hot, kind:"leaf", result:"hot"}
			{id:cold, kind:"leaf", result:"cold"}
		]})
		model {temp:35} decision.decide
	`)
	if result[0].AsString() != "hot" {
		t.Errorf("expected hot, got %v", result[0])
	}
}
