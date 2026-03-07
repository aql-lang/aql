package test

import (
	"testing"
)

// --- Factorial using number/integer type form ---
// These tests verify that the full hierarchical type path "number/integer"
// works correctly in function signatures, matching the same semantics as
// the shorthand "integer".

// Named base case with number/integer types
func TestFactorialTypeScalingNamedBase0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [number/integer] [1] [x:number/integer] [number/integer] [x mul fact (x sub 1)]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingNamedBase1(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [number/integer] [1] [x:number/integer] [number/integer] [x mul fact (x sub 1)]] end`,
		`1 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingNamedBase5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [number/integer] [1] [x:number/integer] [number/integer] [x mul fact (x sub 1)]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialTypeScalingNamedBase10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [number/integer] [1] [x:number/integer] [number/integer] [x mul fact (x sub 1)]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}

// Unnamed literal base case with number/integer types
func TestFactorialTypeScalingUnnamedBase0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 number/integer [drop 1] [x:number/integer] [number/integer] [x mul fact (x sub 1)]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingUnnamedBase1(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 number/integer [drop 1] [x:number/integer] [number/integer] [x mul fact (x sub 1)]] end`,
		`1 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingUnnamedBase5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 number/integer [drop 1] [x:number/integer] [number/integer] [x mul fact (x sub 1)]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialTypeScalingUnnamedBase10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 number/integer [drop 1] [x:number/integer] [number/integer] [x mul fact (x sub 1)]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}

// Tail-recursive with number/integer types
func TestFactorialTypeScalingTailRec0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[number/integer] [number/integer] [1 fact_acc]] end`,
		`def fact_acc fn [[0,number/integer] [number/integer] [swap drop] [number/integer,number/integer] [number/integer] [over mul swap 1 sub swap fact_acc]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingTailRec1(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[number/integer] [number/integer] [1 fact_acc]] end`,
		`def fact_acc fn [[0,number/integer] [number/integer] [swap drop] [number/integer,number/integer] [number/integer] [over mul swap 1 sub swap fact_acc]] end`,
		`1 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingTailRec5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[number/integer] [number/integer] [1 fact_acc]] end`,
		`def fact_acc fn [[0,number/integer] [number/integer] [swap drop] [number/integer,number/integer] [number/integer] [over mul swap 1 sub swap fact_acc]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialTypeScalingTailRec10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[number/integer] [number/integer] [1 fact_acc]] end`,
		`def fact_acc fn [[0,number/integer] [number/integer] [swap drop] [number/integer,number/integer] [number/integer] [over mul swap 1 sub swap fact_acc]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}

// Mixed: use "number" (parent type) in return position, "number/integer" in input
func TestFactorialTypeScalingMixedTypes0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [number] [1] [x:number/integer] [number] [x mul fact (x sub 1)]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingMixedTypes5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [number] [1] [x:number/integer] [number] [x mul fact (x sub 1)]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialTypeScalingMixedTypes10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [number] [1] [x:number/integer] [number] [x mul fact (x sub 1)]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}
