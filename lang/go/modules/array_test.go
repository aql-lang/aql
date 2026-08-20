package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// arrayRegistry returns a registry with the boru:array module loaded and a
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
	arrExport, ok := desc.Exports["ArrayUtil"]
	if !ok {
		t.Fatal("expected 'array' export")
	}
	expected := []string{
		"shape", "rank", "reshape", "transpose",
		"where", "grade", "at", "sortby", "replicate", "expand", "compress",
		"eachrank", "foldaxis",
		"member", "unique", "indices", "group",
		"window", "pairs",
	}
	for _, name := range expected {
		if _, ok := arrExport.Get(name); !ok {
			t.Errorf("missing array export %q", name)
		}
	}
	// ADR-001: no export may shadow a core word. `flatten` is a core word,
	// so it must NOT be an array export (deep flatten is `flatten -1`).
	// `indexof` is the string word (boru:string-util); the array module's
	// list lookup is the distinctly-named `indices`, not `indexof`.
	for _, name := range []string{"flatten", "indexof"} {
		if _, ok := arrExport.Get(name); ok {
			t.Errorf("array must not export %q (shadows a core/other word — ADR-001)", name)
		}
	}
}

// --- Dispatch through the module (forward and infix forms) ---

func TestArrayModuleWords(t *testing.T) {
	r := arrayRegistry(t)
	cases := []struct{ src, want string }{
		// shape / structure
		{`ArrayUtil.shape [[1,2,3],[4,5,6]]`, "[2 3]"},
		{`ArrayUtil.rank [[1,2],[3,4]]`, "2"},
		{`iota 6 ArrayUtil.reshape [2,3]`, "[[0 1 2] [3 4 5]]"},
		{`ArrayUtil.transpose [[1,2,3],[4,5,6]]`, "[[1 4] [2 5] [3 6]]"},
		// selection / ordering
		{`ArrayUtil.where [true,false,true,true]`, "[0 2 3]"},
		{`ArrayUtil.grade [3,1,2]`, "[1 2 0]"},
		{`[10,20,30] ArrayUtil.at [2,0]`, "[30 10]"},
		{`[1,2,3] ArrayUtil.replicate [2,0,1]`, "[1 1 3]"},
		{`ArrayUtil.compress [true,false,true] [10,20,30]`, "[10 30]"},
		// rank polymorphism (quoted code body threads through the wrapper)
		{`ArrayUtil.eachrank 1 [each [add 10]] [[1,2],[3,4]]`, "[[11 12] [13 14]]"},
		{`ArrayUtil.eachrank 0 [mul 2] [[1,2],[3,4]]`, "[[2 4] [6 8]]"},
		{`ArrayUtil.foldaxis 0 [add] [[1,2],[3,4]]`, "[4 6]"},
		{`ArrayUtil.foldaxis 1 [add] [[1,2],[3,4]]`, "[3 7]"},
		// membership / grouping
		{`[1,2,3] ArrayUtil.member [2,3,4]`, "[true true false]"},
		{`[1,2,2,3] ArrayUtil.unique`, "[1 2 3]"},
		// indices: forward form `indices <needles> <haystack>` (haystack last);
		// for each needle its index in the haystack, or -1 when absent.
		{`ArrayUtil.indices [20,99,10] [10,20,30]`, "[1 -1 0]"},
		// neighborhoods
		{`[1,2,3,4] ArrayUtil.window 2`, "[[1 2] [2 3] [3 4]]"},
		{`ArrayUtil.pairs [1,2,3]`, "[[1 2] [2 3]]"},
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
	assertArrayResult(t, r, `ArrayUtil.eachrank 2 [reverse] `+data,
		`[[[3 4] [1 2]] [[7 8] [5 6]]]`)
	// rank 1: body sees each innermost row; reverse flips its elements.
	assertArrayResult(t, r, `ArrayUtil.eachrank 1 [reverse] `+data,
		`[[[2 1] [4 3]] [[6 5] [8 7]]]`)
}

// Negative cases: rank beyond the data, bad axis, ragged input.
func TestArrayRankPolyErrors(t *testing.T) {
	r := arrayRegistry(t)
	for _, src := range []string{
		`ArrayUtil.eachrank 5 [reverse] [[1,2]]`,   // rank exceeds data rank
		`ArrayUtil.foldaxis 2 [add] [[1,2],[3,4]]`, // axis must be 0 or 1
		`ArrayUtil.foldaxis 0 [add] [[1,2],[3]]`,   // not rectangular
	} {
		if _, err := runArraySrc(t, r, src); err == nil {
			t.Errorf("%q: expected error, got none", src)
		}
	}
}

// compress length mismatch is an error.
func TestArrayCompressMismatch(t *testing.T) {
	r := arrayRegistry(t)
	if _, err := runArraySrc(t, r, `ArrayUtil.compress [true,false] [1,2,3]`); err == nil {
		t.Errorf("compress with mismatched lengths should error")
	}
}

// group has two signatures (1-arg and 2-arg); confirm both dispatch.
// Keys are Strings only and the map key is the string's CONTENT, so the
// entries are `a:`/`b:` and not the render's `'a':`/`'b':` (NUR030).
func TestArrayModuleGroupBothSigs(t *testing.T) {
	r := arrayRegistry(t)
	// 1-arg: group equal keys by their index.
	assertArrayResult(t, r, `ArrayUtil.group ["a","b","a"]`, `{a:[0 2] b:[1]}`)
	// 2-arg (forward form, keys then values): group values by parallel keys.
	assertArrayResult(t, r, `ArrayUtil.group ["a","b","a"] [1,2,3]`, `{a:[1 3] b:[2]}`)
}

// The negative half: a non-String key is refused in BOTH signatures,
// with a message that names the requirement rather than a bare
// signature_error (NUR030's verdict makes the two accepted costs — the
// 1-arg form's lost generality and NaN's forbidden key — explicit).
func TestArrayModuleGroupRefusesNonStringKeys(t *testing.T) {
	r := arrayRegistry(t)
	for _, src := range []string{
		`ArrayUtil.group [1,2,3]`,
		`ArrayUtil.group [1,2] ["a","b"]`,
		`ArrayUtil.group [nan nan] [1,2]`,
		`ArrayUtil.group ["a",1] [1,2]`,
	} {
		_, err := runArraySrc(t, r, src)
		if err == nil {
			t.Errorf("%s: expected a refusal, got none", src)
			continue
		}
		if !strings.Contains(err.Error(), "grouping keys must be Strings") {
			t.Errorf("%s: error must name the String requirement, got %v", src, err)
		}
	}
}

// Deep flatten is the core `flatten -1` (no ArrayUtil.flatten); `indexof`
// is the string-only word in boru:string-util. The list-membership lookup
// is the distinctly-named ArrayUtil.indices (see TestArrayModuleWords).
func TestFlattenIsCoreIndexofIsString(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	// indexof moved to boru:string-util (string-only now); seed it bare.
	for _, n := range native.StringModuleNatives {
		r.RegisterNativeFunc(n)
	}
	assertArrayResult(t, r, `flatten -1 [1,[2,[3,[4]]]]`, "[1 2 3 4]") // deep flatten
	assertArrayResult(t, r, `flatten [1,[2,[3]]]`, "[1 2 [3]]")        // default = one level
	assertArrayResult(t, r, `indexof "ll" "hello"`, "2")               // string form only (needle haystack)
}

// --- Negative: the moved words are NOT globally available ---

// Without importing boru:array, the specialised words must error as
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
				t.Fatalf("expected %q to be undefined without boru:array, but it resolved", word)
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
			t.Errorf("core word program %q should run without boru:array: %v", src, err)
		}
	}
}
