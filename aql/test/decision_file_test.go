package test

import (
	"testing"
)

// --- File-based decision module tests ---
// These mirror the native module tests in nativemod/decision_test.go
// to verify that the .aql file module produces identical results.

// --- Condition builder ---

func TestFileDecisionCond(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`18 "gte" age decision.cond`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	if m == nil {
		t.Fatalf("expected map, got %s", result[0].VType.String())
	}
	field, _ := m.Get("field")
	if field.String() != "age" {
		t.Errorf("field = %v, want age", field)
	}
	op, _ := m.Get("op")
	s, _ := op.AsString()
	if s != "gte" {
		t.Errorf("op = %v, want gte", op)
	}
}

// --- Condition evaluation ---

func TestFileDecisionEvalCondTrue(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`{age:25} {field:age,op:"gte",value:18} decision.eval-cond`,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected true for age=25 gte 18")
	}
}

func TestFileDecisionEvalCondFalse(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`{age:15} {field:age,op:"gte",value:18} decision.eval-cond`,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := result[0].AsBoolean()
	if b {
		t.Error("expected false for age=15 gte 18")
	}
}

func TestFileDecisionEvalCondEq(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`{status:"active"} {field:status,op:"eq",value:"active"} decision.eval-cond`,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected true for status eq active")
	}
}

// --- Predicate evaluation ---

func TestFileDecisionEvalPredAllOf(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def pred ([{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.all-of)`,
		`{age:25,score:80} pred decision.eval-pred`,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected all-of true for age=25,score=80")
	}
}

func TestFileDecisionEvalPredAllOfFalse(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def pred ([{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.all-of)`,
		`{age:25,score:30} pred decision.eval-pred`,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := result[0].AsBoolean()
	if b {
		t.Error("expected all-of false for age=25,score=30")
	}
}

func TestFileDecisionEvalPredAnyOf(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def pred ([{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.any-of)`,
		`{age:10,score:80} pred decision.eval-pred`,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected any-of true for age=10,score=80")
	}
}

func TestFileDecisionEvalPredNotOf(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def pred ({field:age,op:"lt",value:18} decision.not-of)`,
		`{age:25} pred decision.eval-pred`,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected not-of(age lt 18) true for age=25")
	}
}

// --- Decision table ---

func TestFileDecisionTableFirst(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def tbl ([
			{when:{field:age,op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:age,op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)`,
		`{age:25} tbl decision.eval-table`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	cat, _ := m.Get("category")
	s, _ := cat.AsString()
	if s != "adult" {
		t.Errorf("expected adult, got %v", cat)
	}
}

func TestFileDecisionTableFirstMinor(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def tbl ([
			{when:{field:age,op:"lt",value:18}, then:{category:"minor"}}
			{when:{field:age,op:"gte",value:18}, then:{category:"adult"}}
		] decision.make-table)`,
		`{age:12} tbl decision.eval-table`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	cat, _ := m.Get("category")
	s, _ := cat.AsString()
	if s != "minor" {
		t.Errorf("expected minor, got %v", cat)
	}
}

func TestFileDecisionTableUnique(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def rawtbl ([
			{when:{field:score,op:"lt",value:50}, then:{grade:"fail"}}
			{when:{field:score,op:"gte",value:50}, then:{grade:"pass"}}
		] decision.make-table)`,
		`def tbl (rawtbl "unique" decision.with-policy)`,
		`{score:75} tbl decision.eval-table`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	grade, _ := m.Get("grade")
	s, _ := grade.AsString()
	if s != "pass" {
		t.Errorf("expected pass, got %v", grade)
	}
}

// Note: "collect" hit-policy is only available in the native Go module.
// The pure AQL version lacks a list-append primitive needed for collect.

func TestFileDecisionTableNoMatch(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def tbl ([{when:{field:age,op:"gt",value:100}, then:{x:1}}] decision.make-table)`,
		`{age:25} tbl decision.eval-table`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	errVal, _ := m.Get("error")
	s, _ := errVal.AsString()
	if s != "no-match" {
		t.Errorf("expected no-match error, got %v", result[0])
	}
}

func TestFileDecisionTableCompound(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def tbl ({kind:"table", hit-policy:"first", rules:[
			{when:{kind:"group",op:"all",children:[{field:age,op:"lt",value:30} {field:score,op:"gte",value:90}]}, then:{tier:"premium"}}
			{when:{field:score,op:"gte",value:50}, then:{tier:"standard"}}
		]})`,
		`{age:25,score:95} tbl decision.eval-table`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	tier, _ := m.Get("tier")
	s, _ := tier.AsString()
	if s != "premium" {
		t.Errorf("expected premium, got %v", tier)
	}
}

// --- Decision tree ---

func TestFileDecisionTree(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def tree ({kind:"tree", root:root, nodes:[
			{id:root, kind:"branch", branches:[
				{when:{field:age,op:"lt",value:18}, next:minor}
				{when:{field:age,op:"gte",value:18}, next:adult}
			]}
			{id:minor, kind:"leaf", result:{category:"minor"}}
			{id:adult, kind:"leaf", result:{category:"adult"}}
		]})`,
		`{age:25} tree decision.eval-tree`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	cat, _ := m.Get("category")
	s, _ := cat.AsString()
	if s != "adult" {
		t.Errorf("expected adult, got %v", cat)
	}
}

func TestFileDecisionTreeMinor(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def tree ({kind:"tree", root:root, nodes:[
			{id:root, kind:"branch", branches:[
				{when:{field:age,op:"lt",value:18}, next:minor}
				{when:{field:age,op:"gte",value:18}, next:adult}
			]}
			{id:minor, kind:"leaf", result:"too-young"}
			{id:adult, kind:"leaf", result:"welcome"}
		]})`,
		`{age:12} tree decision.eval-tree`,
	})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := result[0].AsString()
	if s != "too-young" {
		t.Errorf("expected too-young, got %v", result[0])
	}
}

func TestFileDecisionTreeMultiLevel(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def tree ({kind:"tree", root:check-age, nodes:[
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
		`{age:25,score:90} tree decision.eval-tree`,
	})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := result[0].AsString()
	if s != "approved" {
		t.Errorf("expected approved, got %v", result[0])
	}
}

// --- decide (unified) ---

func TestFileDecideTable(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def model ({kind:"table", hit-policy:"first", rules:[
			{when:{field:x,op:"gt",value:0}, then:{sign:"positive"}}
			{when:{field:x,op:"lt",value:0}, then:{sign:"negative"}}
		]})`,
		`{x:5} model decision.decide`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result[0].AsMap()
	sign, _ := m.Get("sign")
	s, _ := sign.AsString()
	if s != "positive" {
		t.Errorf("expected positive, got %v", sign)
	}
}

func TestFileDecideTree(t *testing.T) {
	dir := moduleWorkDir(t)
	result, err := runRealFileSteps(t, dir, []string{
		`(import "./decision")`,
		`def model ({kind:"tree", root:root, nodes:[
			{id:root, kind:"branch", branches:[
				{when:{field:temp,op:"gt",value:30}, next:hot}
				{when:{field:temp,op:"lt",value:10}, next:cold}
			]}
			{id:hot, kind:"leaf", result:"hot"}
			{id:cold, kind:"leaf", result:"cold"}
		]})`,
		`{temp:35} model decision.decide`,
	})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := result[0].AsString()
	if s != "hot" {
		t.Errorf("expected hot, got %v", result[0])
	}
}

// --- Record type exports ---

func TestFileDecisionExportedTypes(t *testing.T) {
	dir := moduleWorkDir(t)
	types := []string{"Cond", "Pred", "Rule", "DTable", "BranchNode", "LeafNode", "DTree"}
	for _, typ := range types {
		result, err := runRealFileSteps(t, dir, []string{
			`(import "./decision")`,
			`decision.` + typ,
		})
		if err != nil {
			t.Errorf("decision.%s: %v", typ, err)
			continue
		}
		got := result[0].String()
		if got == "" {
			t.Errorf("decision.%s: empty result", typ)
		}
	}
}
