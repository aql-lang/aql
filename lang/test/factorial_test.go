package test

import (
	"testing"
)

// --- Factorial: recursive with named binding base case [_:0] ---

func TestFactorialNamedBase0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [Integer] [1] [x:Integer] [Integer] [x (fact (x sub 1)) mul]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialNamedBase1(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [Integer] [1] [x:Integer] [Integer] [x (fact (x sub 1)) mul]] end`,
		`1 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialNamedBase5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [Integer] [1] [x:Integer] [Integer] [x (fact (x sub 1)) mul]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialNamedBase10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[_:0] [Integer] [1] [x:Integer] [Integer] [x (fact (x sub 1)) mul]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}

// --- Factorial: recursive with unnamed literal 0 base case ---

func TestFactorialUnnamedBase0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 Integer [drop 1] [x:Integer] [Integer] [x (fact (x sub 1)) mul]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialUnnamedBase1(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 Integer [drop 1] [x:Integer] [Integer] [x (fact (x sub 1)) mul]] end`,
		`1 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialUnnamedBase5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 Integer [drop 1] [x:Integer] [Integer] [x (fact (x sub 1)) mul]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialUnnamedBase10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [0 Integer [drop 1] [x:Integer] [Integer] [x (fact (x sub 1)) mul]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}

// --- Factorial: tail-recursive with no named arguments ---

func TestFactorialTailRecBase0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[Integer] [Integer] [1 fact_acc]] end`,
		`def fact_acc fn [[acc:Integer,_:0] [Integer] [acc] [acc:Integer,n:Integer] [Integer] [(acc mul n) (n sub 1) swap fact_acc]] end`,
		`0 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTailRecBase1(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[Integer] [Integer] [1 fact_acc]] end`,
		`def fact_acc fn [[acc:Integer,_:0] [Integer] [acc] [acc:Integer,n:Integer] [Integer] [(acc mul n) (n sub 1) swap fact_acc]] end`,
		`1 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFactorialTailRecBase5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[Integer] [Integer] [1 fact_acc]] end`,
		`def fact_acc fn [[acc:Integer,_:0] [Integer] [acc] [acc:Integer,n:Integer] [Integer] [(acc mul n) (n sub 1) swap fact_acc]] end`,
		`5 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "120")
}

func TestFactorialTailRecBase10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def fact fn [[Integer] [Integer] [1 fact_acc]] end`,
		`def fact_acc fn [[acc:Integer,_:0] [Integer] [acc] [acc:Integer,n:Integer] [Integer] [(acc mul n) (n sub 1) swap fact_acc]] end`,
		`10 fact`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3628800")
}
