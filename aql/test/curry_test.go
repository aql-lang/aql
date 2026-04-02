package test

import (
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/nativemod"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// runSteps executes a sequence of AQL expressions on a shared engine,
// returning the result of the last step.
func runSteps(t *testing.T, steps []string) ([]engine.Value, error) {
	t.Helper()
	reg, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	nativemod.InstallMathExports(reg)
	eng := engine.NewTop(reg)

	var result []engine.Value
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			return nil, err
		}
		result, err = eng.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// assertResult checks that the result stack formatted as a string matches want.
func assertResult(t *testing.T, result []engine.Value, want string) {
	t.Helper()
	got := formatStack(result)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- Arithmetic partial application ---

func TestCurryAdd5(t *testing.T) {
	result, err := runSteps(t, []string{
		`def add5 [add 5] end`,
		`10 add5`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "15")
}

func TestCurrySub1(t *testing.T) {
	result, err := runSteps(t, []string{
		`def sub1 [sub 1] end`,
		`10 sub1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "9")
}

func TestCurryMul3(t *testing.T) {
	result, err := runSteps(t, []string{
		`def mul3 [mul 3] end`,
		`4 mul3`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "12")
}

func TestCurryDiv2(t *testing.T) {
	result, err := runSteps(t, []string{
		`def div2 [div 2] end`,
		`10 div2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "5")
}

func TestCurryMod3(t *testing.T) {
	result, err := runSteps(t, []string{
		`def mod3 [mod 3] end`,
		`10 mod3`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

// --- Multiple uses of a curried word ---

func TestCurryReuse(t *testing.T) {
	result, err := runSteps(t, []string{
		`def inc [1 add]`,
		`1 inc inc inc`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "4")
}

// --- Chaining curried words ---

func TestCurryChain(t *testing.T) {
	result, err := runSteps(t, []string{
		`def add5 [add 5] end`,
		`def mul2 [mul 2] end`,
		`3 add5 mul2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "16")
}

func TestCurryChainReversed(t *testing.T) {
	result, err := runSteps(t, []string{
		`def mul2 [mul 2] end`,
		`def add5 [add 5] end`,
		`3 mul2 add5`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "11")
}

// --- Curried word used in infix position ---

func TestCurryInfix(t *testing.T) {
	result, err := runSteps(t, []string{
		`def add5 [add 5] end`,
		`10 add (add5 3)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "18")
}

// --- List-style def (quotation) ---

func TestCurryListDouble(t *testing.T) {
	result, err := runSteps(t, []string{
		`def double [dup add]`,
		`5 double`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10")
}

func TestCurryListSquare(t *testing.T) {
	result, err := runSteps(t, []string{
		`def square [dup mul]`,
		`7 square`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "49")
}

// --- Curried boolean operators ---

func TestCurryAnd(t *testing.T) {
	result, err := runSteps(t, []string{
		`def and_true [and true] end`,
		`false and_true`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "false")
}

func TestCurryOr(t *testing.T) {
	result, err := runSteps(t, []string{
		`def or_true [or true] end`,
		`false or_true`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

// --- Curried comparison operators ---

func TestCurryLt10(t *testing.T) {
	result, err := runSteps(t, []string{
		`def lt10 [lt 10] end`,
		`5 lt10`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestCurryGte0(t *testing.T) {
	result, err := runSteps(t, []string{
		`def gte0 [gte 0] end`,
		`-1 gte0`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "false")
}

// --- Curried string operators ---

func TestCurryStringConcat(t *testing.T) {
	result, err := runSteps(t, []string{
		`def greet [add "hello "] end`,
		`greet "world"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Forward-first swap: curry expansion reverses string concat order
	assertResult(t, result, "'worldhello '")
}

// --- Curried words with parentheses ---

func TestCurryInParens(t *testing.T) {
	result, err := runSteps(t, []string{
		`def add5 [add 5] end`,
		`(3 add5) mul 2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "16")
}

// --- Composing curried words via def ---

func TestCurryCompose(t *testing.T) {
	result, err := runSteps(t, []string{
		`def add5 [add 5] end`,
		`def add5_twice [add5 add5]`,
		`10 add5_twice`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "20")
}

// --- Curried conversion ---

func TestCurryConvert(t *testing.T) {
	result, err := runSteps(t, []string{
		`def to_string [convert String] end`,
		`42 to_string`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'42'")
}

// --- Partial application error: curried word without outer forward errors ---

func TestCurryNoOuterForwardErrors(t *testing.T) {
	// Without an outer forward context, insufficient args should error.
	reg, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.NewTop(reg)
	vals, err := parser.Parse(`add 5`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.Run(vals)
	if err == nil {
		t.Fatal("expected error for add with only 1 forward arg and no outer forward")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Errorf("expected signature error, got: %v", err)
	}
}

// --- Verify def body preserves word identity ---

func TestCurryDefPreservesWord(t *testing.T) {
	// Ensure that a curried word defined via `def ... end` actually
	// creates a working definition, not just a list.
	result, err := runSteps(t, []string{
		`def add10 [add 10] end`,
		`5 add10`,
		`20 add10`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Last Run returns only the result of `20 add10`.
	assertResult(t, result, "30")
}
