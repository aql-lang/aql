package test

import (
	"testing"
)

// --- Factorial using Number/Integer type form ---
// These tests verify that the full hierarchical type path "Number/Integer"
// works correctly in function signatures, matching the same semantics as
// the shorthand "integer".

// Named base case with Number/Integer types
func TestFactorialTypeScalingNamedBase0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[zero:0] [Number/Integer] [1] [x:Number/Integer] [Number/Integer] [x (fact (x sub 1)) mul]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingNamedBase1(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[zero:0] [Number/Integer] [1] [x:Number/Integer] [Number/Integer] [x (fact (x sub 1)) mul]] end`,
		`1 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingNamedBase5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[zero:0] [Number/Integer] [1] [x:Number/Integer] [Number/Integer] [x (fact (x sub 1)) mul]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialTypeScalingNamedBase10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[zero:0] [Number/Integer] [1] [x:Number/Integer] [Number/Integer] [x (fact (x sub 1)) mul]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}

// Unnamed literal base case with Number/Integer types
func TestFactorialTypeScalingUnnamedBase0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 Number/Integer [drop 1] [x:Number/Integer] [Number/Integer] [x (fact (x sub 1)) mul]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingUnnamedBase1(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 Number/Integer [drop 1] [x:Number/Integer] [Number/Integer] [x (fact (x sub 1)) mul]] end`,
		`1 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingUnnamedBase5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 Number/Integer [drop 1] [x:Number/Integer] [Number/Integer] [x (fact (x sub 1)) mul]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialTypeScalingUnnamedBase10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 Number/Integer [drop 1] [x:Number/Integer] [Number/Integer] [x (fact (x sub 1)) mul]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}

// Tail-recursive with Number/Integer types
func TestFactorialTypeScalingTailRec0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[Number/Integer] [Number/Integer] [1 fact_acc]] end`,
		`def fact_acc fn [[acc:Number/Integer,zero:0] [Number/Integer] [acc] [acc:Number/Integer,n:Number/Integer] [Number/Integer] [(acc mul n) (n sub 1) swap fact_acc]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingTailRec1(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[Number/Integer] [Number/Integer] [1 fact_acc]] end`,
		`def fact_acc fn [[acc:Number/Integer,zero:0] [Number/Integer] [acc] [acc:Number/Integer,n:Number/Integer] [Number/Integer] [(acc mul n) (n sub 1) swap fact_acc]] end`,
		`1 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingTailRec5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[Number/Integer] [Number/Integer] [1 fact_acc]] end`,
		`def fact_acc fn [[acc:Number/Integer,zero:0] [Number/Integer] [acc] [acc:Number/Integer,n:Number/Integer] [Number/Integer] [(acc mul n) (n sub 1) swap fact_acc]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialTypeScalingTailRec10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[Number/Integer] [Number/Integer] [1 fact_acc]] end`,
		`def fact_acc fn [[acc:Number/Integer,zero:0] [Number/Integer] [acc] [acc:Number/Integer,n:Number/Integer] [Number/Integer] [(acc mul n) (n sub 1) swap fact_acc]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}

// Mixed: use "number" (parent type) in return position, "Number/Integer" in input
func TestFactorialTypeScalingMixedTypes0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[zero:0] [Number] [1] [x:Number/Integer] [Number] [x (fact (x sub 1)) mul]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTypeScalingMixedTypes5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[zero:0] [Number] [1] [x:Number/Integer] [Number] [x (fact (x sub 1)) mul]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialTypeScalingMixedTypes10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[zero:0] [Number] [1] [x:Number/Integer] [Number] [x (fact (x sub 1)) mul]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}
