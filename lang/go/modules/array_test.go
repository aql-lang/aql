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
		"shape", "rank", "reshape", "transpose",
		"where", "grade", "at", "sortby", "replicate", "expand", "compress",
		"eachrank", "foldaxis",
		"member", "unique", "group",
		"window", "pairs",
	}
	for _, name := range expected {
		if _, ok := arrExport.Get(name); !ok {
			t.Errorf("missing array export %q", name)
		}
	}
	// ADR-001: no export may shadow a core word. flatten and indexof are
	// core words, so they must NOT be array exports (deep flatten is
	// `flatten -1`; list lookup is the core indexof list overload).
	for _, name := range []string{"flatten", "indexof"} {
		if _, ok := arrExport.Get(name); ok {
			t.Errorf("array must not export %q (shadows a core word — ADR-001)", name)
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
		{`array.transpose [[1,2,3],[4,5,6]]`, "[[1,4],[2,5],[3,6]]"},
		// selection / ordering
		{`array.where [true,false,true,true]`, "[0,2,3]"},
		{`array.grade [3,1,2]`, "[1,2,0]"},
		{`[10,20,30] array.at [2,0]`, "[30,10]"},
		{`[1,2,3] array.replicate [2,0,1]`, "[1,1,3]"},
		{`array.compress [true,false,true] [10,20,30]`, "[10,30]"},
		// rank polymorphism (quoted code body threads through the wrapper)
		{`array.eachrank 1 [each [add 10]] [[1,2],[3,4]]`, "[[11,12],[13,14]]"},
		{`array.eachrank 0 [mul 2] [[1,2],[3,4]]`, "[[2,4],[6,8]]"},
		{`array.foldaxis 0 [add] [[1,2],[3,4]]`, "[4,6]"},
		{`array.foldaxis 1 [add] [[1,2],[3,4]]`, "[3,7]"},
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

// eachrank rank is the J-style CELL rank, measured from the leaves, so
// it targets a consistent depth regardless of total nesting.
func TestArrayEachrankCellRank(t *testing.T) {
	r := arrayRegistry(t)
	data := `[[[1,2],[3,4]],[[5,6],[7,8]]]` // rank 3
	// rank 2: body sees each rank-2 matrix; reverse flips its rows.
	assertArrayResult(t, r, `array.eachrank 2 [reverse] `+data,
		`[[[3,4],[1,2]],[[7,8],[5,6]]]`)
	// rank 1: body sees each innermost row; reverse flips its elements.
	assertArrayResult(t, r, `array.eachrank 1 [reverse] `+data,
		`[[[2,1],[4,3]],[[6,5],[8,7]]]`)
}

// Negative cases: rank beyond the data, bad axis, ragged input.
func TestArrayRankPolyErrors(t *testing.T) {
	r := arrayRegistry(t)
	for _, src := range []string{
		`array.eachrank 5 [reverse] [[1,2]]`,   // rank exceeds data rank
		`array.foldaxis 2 [add] [[1,2],[3,4]]`, // axis must be 0 or 1
		`array.foldaxis 0 [add] [[1,2],[3]]`,   // not rectangular
	} {
		if _, err := runArraySrc(t, r, src); err == nil {
			t.Errorf("%q: expected error, got none", src)
		}
	}
}

// compress length mismatch is an error.
func TestArrayCompressMismatch(t *testing.T) {
	r := arrayRegistry(t)
	if _, err := runArraySrc(t, r, `array.compress [true,false] [1,2,3]`); err == nil {
		t.Errorf("compress with mismatched lengths should error")
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

// ADR-001 replacements: deep flatten and list indexof are core words,
// reached without importing aql:array. (That array.flatten / array.indexof
// are not exports is pinned in TestArrayModuleExports.)
func TestFlattenAndIndexofAreCore(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	// No aql:array import — these are core.
	assertArrayResult(t, r, `flatten -1 [1,[2,[3,[4]]]]`, "[1,2,3,4]")  // deep flatten
	assertArrayResult(t, r, `flatten [1,[2,[3]]]`, "[1,2,[3]]")         // default = one level
	assertArrayResult(t, r, `indexof [20,99,10] [10,20,30]`, "[1,3,0]") // list overload
	assertArrayResult(t, r, `indexof "hello" "ll"`, "2")                // string overload, same word
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
