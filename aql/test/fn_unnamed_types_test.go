package test

import (
	"testing"
)

// Tests for unnamed type-literal parameters in fn signatures.
// Regression: bare Map in an input signature triggered a panic because
// parseFnParams conflated a Map type literal (Data==nil) with an actual
// map value, calling AsMap() on nil.

// --- Single unnamed Map parameter ---

func TestFnUnnamedMap(t *testing.T) {
	result, err := runSteps(t, []string{
		`def id fn [[Map] [] []] end`,
		`{a:1} id`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1}")
}

func TestFnUnnamedMapDup(t *testing.T) {
	result, err := runSteps(t, []string{
		`def d fn [[Map] [] [dup]] end`,
		`{a:1} d`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1} {a:1}")
}

// --- Multiple unnamed Map parameters ---

func TestFnUnnamedMapMap(t *testing.T) {
	result, err := runSteps(t, []string{
		`def sw fn [[Map, Map] [] [swap]] end`,
		`{a:1} {b:2} sw`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{b:2} {a:1}")
}

// --- Multi-signature fn with unnamed Map (the original panic case) ---

func TestFnMultiSigUnnamedMapOneArg(t *testing.T) {
	result, err := runSteps(t, []string{
		`def foo fn [[Map] [] [] [Map, Map] [] [swap]] end`,
		`{a:1} foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1}")
}

func TestFnMultiSigUnnamedMapTwoArgs(t *testing.T) {
	result, err := runSteps(t, []string{
		`def foo fn [[Map] [] [] [Map, Map] [] [swap]] end`,
		`{a:1} {b:2} foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{b:2} {a:1}")
}

// --- Unnamed List parameter ---

func TestFnUnnamedList(t *testing.T) {
	result, err := runSteps(t, []string{
		`def id fn [[List] [] []] end`,
		`quote [1 2 3] id`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "[1,2,3]")
}

func TestFnUnnamedListDup(t *testing.T) {
	result, err := runSteps(t, []string{
		`def d fn [[List] [] [dup]] end`,
		`quote [1 2] d`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "[1,2] [1,2]")
}

// --- Unnamed Boolean parameter ---

func TestFnUnnamedBoolean(t *testing.T) {
	result, err := runSteps(t, []string{
		`def notb fn [[Boolean] [] [not]] end`,
		`true notb`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "false")
}

// --- Unnamed String parameter ---

func TestFnUnnamedString(t *testing.T) {
	result, err := runSteps(t, []string{
		`def greet fn [[String] [] ["hello " swap add]] end`,
		`"world" greet`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'hello world'")
}

// --- Mixed named and unnamed Map in same signature ---

func TestFnMixedNamedUnnamedMap(t *testing.T) {
	result, err := runSteps(t, []string{
		`def sw fn [[m:Map, Map] [] [m swap]] end`,
		`{a:1} {b:2} sw`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// m binds {a:1}, unnamed pushes {b:2}, body [m swap] pushes m then swaps
	assertResult(t, result, "{a:1} {b:2}")
}

// --- Multi-signature mixing named Map and unnamed Map ---

func TestFnMultiSigMixedMapOneArg(t *testing.T) {
	result, err := runSteps(t, []string{
		`def foo fn [[m:Map] [] [m] [Map, Map] [] [swap]] end`,
		`{a:1} foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1}")
}

func TestFnMultiSigMixedMapTwoArgs(t *testing.T) {
	result, err := runSteps(t, []string{
		`def foo fn [[m:Map] [] [m] [Map, Map] [] [swap]] end`,
		`{a:1} {b:2} foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{b:2} {a:1}")
}
