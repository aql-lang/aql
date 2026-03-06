package aql_test

import (
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
