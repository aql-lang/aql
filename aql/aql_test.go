package aql_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql"
)

func TestNew(t *testing.T) {
	a := aql.New()
	if a == nil {
		t.Fatal("New() returned nil")
	}
}

// --- Basic execution ---

func TestRunInteger(t *testing.T) {
	a := aql.New()
	result, err := a.Run("42")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != int64(42) {
		t.Errorf("got %v, want [42]", result)
	}
}

func TestRunString(t *testing.T) {
	a := aql.New()
	result, err := a.Run(`"hello"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != "hello" {
		t.Errorf("got %v, want [hello]", result)
	}
}

func TestRunArithmetic(t *testing.T) {
	a := aql.New()
	result, err := a.Run("1 add 2")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != int64(3) {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestRunEmptyResult(t *testing.T) {
	a := aql.New()
	result, err := a.Run("1 drop")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("got %v, want []", result)
	}
}

func TestRunMultipleValues(t *testing.T) {
	a := aql.New()
	result, err := a.Run("1 2 3")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3", len(result))
	}
	for i, want := range []int64{1, 2, 3} {
		if result[i] != want {
			t.Errorf("result[%d] = %v, want %d", i, result[i], want)
		}
	}
}

// --- Independent instances ---

func TestIndependentInstances(t *testing.T) {
	a := aql.New()
	b := aql.New()

	// Store in a.
	_, err := a.Run("set x 42 end")
	if err != nil {
		t.Fatal(err)
	}

	// b should not see x.
	_, err = b.Run("get x")
	if err == nil {
		t.Fatal("expected error: b should not have key x")
	}

	// a should still see x.
	result, err := a.Run("get x")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != int64(42) {
		t.Errorf("got %v, want [42]", result)
	}
}

func TestStatePersistsAcrossRuns(t *testing.T) {
	a := aql.New()

	_, err := a.Run("set counter 10 end")
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run("get counter")
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != int64(10) {
		t.Errorf("got %v, want 10", result[0])
	}
}

func TestManyIndependentInstances(t *testing.T) {
	instances := make([]*aql.AQL, 5)
	for i := range instances {
		instances[i] = aql.New()
	}

	// Each instance stores its own index.
	for i, a := range instances {
		_, err := a.Run("set idx " + itoa(i) + " end")
		if err != nil {
			t.Fatalf("instance %d set: %v", i, err)
		}
	}

	// Each instance retrieves only its own index.
	for i, a := range instances {
		result, err := a.Run("get idx")
		if err != nil {
			t.Fatalf("instance %d get: %v", i, err)
		}
		if result[0] != int64(i) {
			t.Errorf("instance %d: got %v, want %d", i, result[0], i)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// --- Multiline scripts ---

func TestMultilineNewlines(t *testing.T) {
	a := aql.New()
	result, err := a.Run("1\nadd\n2")
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != int64(3) {
		t.Errorf("got %v, want 3", result[0])
	}
}

func TestMultilineTabs(t *testing.T) {
	a := aql.New()
	result, err := a.Run("1\tadd\t2")
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != int64(3) {
		t.Errorf("got %v, want 3", result[0])
	}
}

func TestMultilineMixed(t *testing.T) {
	a := aql.New()
	src := "set x 10 end\nset y 20 end\nget x\nadd\nget y"
	result, err := a.Run(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != int64(30) {
		t.Errorf("got %v, want [30]", result)
	}
}

func TestMultilineCRLF(t *testing.T) {
	a := aql.New()
	result, err := a.Run("1\r\nadd\r\n2")
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != int64(3) {
		t.Errorf("got %v, want 3", result[0])
	}
}

func TestMultilineBlankLines(t *testing.T) {
	a := aql.New()
	result, err := a.Run("1\n\n\nadd\n\n2")
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != int64(3) {
		t.Errorf("got %v, want 3", result[0])
	}
}

func TestMultilineScript(t *testing.T) {
	a := aql.New()
	script := `
		set width 10 end
		set height 5 end
		get width
		mul
		get height
	`
	result, err := a.Run(script)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != int64(50) {
		t.Errorf("got %v, want [50]", result)
	}
}

func TestMultilineWithComments(t *testing.T) {
	a := aql.New()
	script := `
		# set up values
		set x 7 end
		set y 3 end
		# compute
		get x
		add
		get y
		# result should be 10
	`
	result, err := a.Run(script)
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != int64(10) {
		t.Errorf("got %v, want 10", result[0])
	}
}

// --- Error handling ---

func TestRunParseError(t *testing.T) {
	a := aql.New()
	_, err := a.Run(`"unterminated`)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRunEngineError(t *testing.T) {
	a := aql.New()
	_, err := a.Run("10 div 0")
	if err == nil {
		t.Fatal("expected engine error")
	}
}

func TestRunEmpty(t *testing.T) {
	a := aql.New()
	result, err := a.Run("")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("got %v, want []", result)
	}
}

// --- NewMemFileOps, SetFileOps, RegisterFormat ---

func TestNewMemFileOps(t *testing.T) {
	ops := aql.NewMemFileOps()
	if ops == nil {
		t.Fatal("NewMemFileOps() returned nil")
	}
}

func TestSetFileOps(t *testing.T) {
	a := aql.New()
	ops := aql.NewMemFileOps()
	a.SetFileOps(ops)

	// Write and read back via the mem file ops.
	_, err := a.Run(`write "test.txt" "hello"`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Run(`read "test.txt"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != "hello" {
		t.Errorf("got %v, want [hello]", result)
	}
}

func TestRunDefaultBranch(t *testing.T) {
	// Exercise the default branch in the result conversion switch by
	// producing a value that is neither integer nor string (e.g. a map).
	a := aql.New()
	ops := aql.NewMemFileOps()
	a.SetFileOps(ops)

	// Write a JSON file with a map value, then read it back.
	// The result is a map which hits the default v.String() branch.
	ops.Files["data.json"] = []byte(`{"a":1}`)
	result, err := a.Run(`read "data.json"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	// The result should be the string representation of the map.
	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string, got %T", result[0])
	}
	if s == "" {
		t.Error("expected non-empty string representation")
	}
}

// --- Register / RegisterPrefixOnly ---

func TestRegisterSuffixWord(t *testing.T) {
	a := aql.New()
	// Register "double" as a suffix-precedence word: 5 double => 10
	a.Register("double", aql.Signature{
		Args: []aql.Type{aql.TInteger},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			n := args[0].AsInteger()
			return []aql.Value{aql.NewInteger(n * 2)}, nil
		},
	})

	result, err := a.Run("5 double")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != int64(10) {
		t.Errorf("got %v, want [10]", result)
	}
}

func TestRegisterSuffixWordCollectsAfter(t *testing.T) {
	a := aql.New()
	// Register "double" with suffix precedence — can collect arg after the word.
	a.Register("double", aql.Signature{
		Args: []aql.Type{aql.TInteger},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			n := args[0].AsInteger()
			return []aql.Value{aql.NewInteger(n * 2)}, nil
		},
	})

	result, err := a.Run("double 7")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != int64(14) {
		t.Errorf("got %v, want [14]", result)
	}
}

func TestRegisterPrefixOnlyWord(t *testing.T) {
	a := aql.New()
	// Register "neg" as prefix-only: 5 neg => -5
	a.RegisterPrefixOnly("neg", aql.Signature{
		Args: []aql.Type{aql.TInteger},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			n := args[0].AsInteger()
			return []aql.Value{aql.NewInteger(-n)}, nil
		},
	})

	result, err := a.Run("5 neg")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != int64(-5) {
		t.Errorf("got %v, want [-5]", result)
	}
}

func TestRegisterPrefixOnlyDoesNotCollectSuffix(t *testing.T) {
	a := aql.New()
	a.RegisterPrefixOnly("neg", aql.Signature{
		Args: []aql.Type{aql.TInteger},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			n := args[0].AsInteger()
			return []aql.Value{aql.NewInteger(-n)}, nil
		},
	})

	// "neg 5" — neg is prefix-only so it should not consume 5 from suffix.
	// Without a value on the stack, it should error.
	_, err := a.Run("neg 5")
	if err == nil {
		t.Fatal("expected error: neg is prefix-only and has no prefix args")
	}
}

func TestRegisterMultipleSignatures(t *testing.T) {
	a := aql.New()
	// Register "square" with two signatures: integer and string.
	a.Register("square",
		aql.Signature{
			Args: []aql.Type{aql.TInteger},
			Handler: func(args []aql.Value) ([]aql.Value, error) {
				n := args[0].AsInteger()
				return []aql.Value{aql.NewInteger(n * n)}, nil
			},
		},
		aql.Signature{
			Args: []aql.Type{aql.TString},
			Handler: func(args []aql.Value) ([]aql.Value, error) {
				s := args[0].AsString()
				return []aql.Value{aql.NewString(s + s)}, nil
			},
		},
	)

	// Integer signature.
	result, err := a.Run("4 square")
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != int64(16) {
		t.Errorf("got %v, want 16", result[0])
	}

	// String signature.
	result, err = a.Run(`"ab" square`)
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != "abab" {
		t.Errorf("got %v, want abab", result[0])
	}
}

func TestRegisterWithPrecedence(t *testing.T) {
	a := aql.New()

	// Register "myadd" with low precedence and "mymul" with high precedence.
	a.Register("myadd", aql.Signature{
		Args:       []aql.Type{aql.TInteger, aql.TInteger},
		Precedence: 1,
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			return []aql.Value{aql.NewInteger(args[0].AsInteger() + args[1].AsInteger())}, nil
		},
	})
	a.Register("mymul", aql.Signature{
		Args:       []aql.Type{aql.TInteger, aql.TInteger},
		Precedence: 2,
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			return []aql.Value{aql.NewInteger(args[0].AsInteger() * args[1].AsInteger())}, nil
		},
	})

	// 2 myadd 3 mymul 4 => mymul binds tighter, so 3*4=12 first, then 2+12=14
	result, err := a.Run("2 myadd 3 mymul 4")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != int64(14) {
		t.Errorf("got %v, want [14]", result)
	}
}

func TestRegisterReturnsMultipleValues(t *testing.T) {
	a := aql.New()
	// Register "divmod" that returns quotient and remainder.
	a.Register("divmod", aql.Signature{
		Args: []aql.Type{aql.TInteger, aql.TInteger},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			a, b := args[0].AsInteger(), args[1].AsInteger()
			if b == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			return []aql.Value{aql.NewInteger(a / b), aql.NewInteger(a % b)}, nil
		},
	})

	result, err := a.Run("17 divmod 5")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0] != int64(3) || result[1] != int64(2) {
		t.Errorf("got %v, want [3 2]", result)
	}
}

func TestRegisterErrorPropagation(t *testing.T) {
	a := aql.New()
	a.Register("fail", aql.Signature{
		Args: []aql.Type{aql.TAny},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			return nil, fmt.Errorf("intentional error")
		},
	})

	_, err := a.Run("42 fail")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "intentional error") {
		t.Errorf("got %q, want error containing 'intentional error'", err.Error())
	}
}

func TestRegisterWorksWithBuiltins(t *testing.T) {
	a := aql.New()
	a.Register("triple", aql.Signature{
		Args: []aql.Type{aql.TInteger},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			return []aql.Value{aql.NewInteger(args[0].AsInteger() * 3)}, nil
		},
	})

	// Mix native Go word with built-in "add".
	result, err := a.Run("2 triple add 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != int64(7) {
		t.Errorf("got %v, want [7]", result)
	}
}

func TestRegisterIsolatedBetweenInstances(t *testing.T) {
	a := aql.New()
	b := aql.New()

	a.Register("custom", aql.Signature{
		Args: []aql.Type{aql.TInteger},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			return []aql.Value{aql.NewInteger(args[0].AsInteger() + 100)}, nil
		},
	})

	// a should have "custom".
	result, err := a.Run("5 custom")
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != int64(105) {
		t.Errorf("got %v, want 105", result[0])
	}

	// b should not have "custom" — it becomes an atom.
	result, err = b.Run("5 custom")
	if err != nil {
		t.Fatal(err)
	}
	// "custom" is not registered in b, so it stays as an atom on the stack.
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2 (5 and atom custom)", len(result))
	}
}

func TestRegisterStringHandler(t *testing.T) {
	a := aql.New()
	a.Register("shout", aql.Signature{
		Args: []aql.Type{aql.TString},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			s := args[0].AsString()
			return []aql.Value{aql.NewString(strings.ToUpper(s) + "!")}, nil
		},
	})

	result, err := a.Run(`"hello" shout`)
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != "HELLO!" {
		t.Errorf("got %v, want HELLO!", result[0])
	}
}

func TestRegisterZeroArgWord(t *testing.T) {
	a := aql.New()
	counter := 0
	a.Register("tick", aql.Signature{
		Args: []aql.Type{},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			counter++
			return []aql.Value{aql.NewInteger(int64(counter))}, nil
		},
	})

	result, err := a.Run("tick tick tick")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 || result[0] != int64(1) || result[1] != int64(2) || result[2] != int64(3) {
		t.Errorf("got %v, want [1 2 3]", result)
	}
}

func TestRegisterAddsAlongsideBuiltin(t *testing.T) {
	a := aql.New()
	// Add a new integer signature to the built-in "upper" word.
	// The existing string signature still works; the new one handles integers.
	a.Register("upper", aql.Signature{
		Args: []aql.Type{aql.TInteger},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			return []aql.Value{aql.NewInteger(args[0].AsInteger() + 1000)}, nil
		},
	})

	// Built-in string signature still works.
	result, err := a.Run(`"hello" upper`)
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != "HELLO" {
		t.Errorf("got %v, want HELLO", result[0])
	}

	// New integer signature works.
	result, err = a.Run("7 upper")
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != int64(1007) {
		t.Errorf("got %v, want 1007", result[0])
	}
}

func TestNewTypeCustom(t *testing.T) {
	// Verify NewType creates usable custom types.
	myType := aql.NewType("custom/special")
	if myType.String() != "custom/special" {
		t.Errorf("got %q, want custom/special", myType.String())
	}
}

func TestRegisterWithTypeAny(t *testing.T) {
	a := aql.New()
	a.Register("identity", aql.Signature{
		Args: []aql.Type{aql.TAny},
		Handler: func(args []aql.Value) ([]aql.Value, error) {
			return args, nil
		},
	})

	// Works with integer.
	result, err := a.Run("42 identity")
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != int64(42) {
		t.Errorf("got %v, want 42", result[0])
	}

	// Works with string.
	result, err = a.Run(`"hi" identity`)
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != "hi" {
		t.Errorf("got %v, want hi", result[0])
	}
}
