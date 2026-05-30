package modules

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// arrayRegistry returns a registry with the aql:array module loaded and a
// parse func installed, so source-string programs can be run.
func arrayRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	if err := InstallArrayExports(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func runArraySrc(t *testing.T, r *native.Registry, src string) ([]native.Value, error) {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return native.NewTop(r).Run(values)
}

func assertArrayResult(t *testing.T, r *native.Registry, src, want string) {
	t.Helper()
	result, err := runArraySrc(t, r, src)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", src, err)
	}
	if len(result) != 1 {
		t.Fatalf("%q: expected 1 result, got %d", src, len(result))
	}
	if got := result[0].String(); got != want {
		t.Errorf("%q = %s, want %s", src, got, want)
	}
}

// --- Module structure ---

func TestArrayModuleExports(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildArrayModule(r)
	if err != nil {
		t.Fatal(err)
	}
	arrExport, ok := desc.Exports["array"]
	if !ok {
		t.Fatal("expected 'array' export")
	}
	expected := []string{
		"shape", "rank", "reshape", "flatten", "transpose",
		"where", "grade", "at", "sortby", "replicate", "expand",
		"member", "indexof", "unique", "group",
		"window", "pairs",
	}
	for _, name := range expected {
		if _, ok := arrExport.Get(name); !ok {
			t.Errorf("missing array export %q", name)
		}
	}
}

// --- Dispatch through the module (forward and swap forms) ---

func TestArrayModuleWords(t *testing.T) {
	r := arrayRegistry(t)
	cases := []struct{ src, want string }{
		// shape / structure
		{`array.shape [[1,2,3],[4,5,6]]`, "[2,3]"},
		{`array.rank [[1,2],[3,4]]`, "2"},
		{`iota 6 array.reshape [2,3]`, "[[0,1,2],[3,4,5]]"},
		{`array.flatten [[1,2],[3]]`, "[1,2,3]"},
		{`array.transpose [[1,2,3],[4,5,6]]`, "[[1,4],[2,5],[3,6]]"},
		// selection / ordering
		{`array.where [true,false,true,true]`, "[0,2,3]"},
		{`array.grade [3,1,2]`, "[1,2,0]"},
		{`[10,20,30] array.at [2,0]`, "[30,10]"},
		{`[1,2,3] array.replicate [2,0,1]`, "[1,1,3]"},
		// membership / grouping
		{`[1,2,3] array.member [2,3,4]`, "[true,true,false]"},
		{`[1,2,2,3] array.unique`, "[1,2,3]"},
		// neighborhoods
		{`[1,2,3,4] array.window 2`, "[[1,2],[2,3],[3,4]]"},
		{`array.pairs [1,2,3]`, "[[1,2],[2,3]]"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			assertArrayResult(t, r, tc.src, tc.want)
		})
	}
}

// group has two signatures (1-arg and 2-arg); confirm both dispatch.
func TestArrayModuleGroupBothSigs(t *testing.T) {
	r := arrayRegistry(t)
	// 1-arg: group equal values by their index.
	assertArrayResult(t, r, `array.group ["a","b","a"]`, `{'a':[0,2],'b':[1]}`)
	// 2-arg (forward form, keys then values): group values by parallel keys.
	assertArrayResult(t, r, `array.group ["a","b","a"] [1,2,3]`, `{'a':[1,3],'b':[2]}`)
}

// --- Negative: the moved words are NOT globally available ---

// Without importing aql:array, the specialised words must error as
// undefined rather than silently resolving — that is the whole point of
// gating them behind the module.
func TestArrayWordsNotGlobal(t *testing.T) {
	for _, word := range []string{"shape", "reshape", "where", "grade", "transpose"} {
		t.Run(word, func(t *testing.T) {
			r, err := native.DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			r.SetParseFunc(parser.Parse)
			// No InstallArrayExports here.
			_, runErr := runArraySrc(t, r, "[[1,2],[3,4]] "+word)
			if runErr == nil {
				t.Fatalf("expected %q to be undefined without aql:array, but it resolved", word)
			}
		})
	}
}

// The core array words must remain global (not moved into the module).
func TestArrayCoreWordsStillGlobal(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	for _, src := range []string{
		`iota 3`,
		`range 0 3`,
		`[1,2,3] each [dup mul]`,
		`take 2 [1,2,3,4]`,
		`[1,2,3] reverse`,
	} {
		if _, err := runArraySrc(t, r, src); err != nil {
			t.Errorf("core word program %q should run without aql:array: %v", src, err)
		}
	}
}
