package test

import (
	"strings"
	"testing"
)

// TestStringsNeverPromotedToWords confirms a core invariant of the
// language: a quoted string is DATA. A string whose text happens to name
// a registered word (`"add"`, `"if"`, `"dup"`, `"get"`, `"do"`, …) is
// NEVER silently promoted to that word and dispatched — in any
// container, at any nesting depth, or through any data-manipulation
// word. Only UNQUOTED words (which the parser yields as Word values)
// execute. Silent string→word promotion is a footgun, not a feature
// (voxgig DX report T4); these tests pin its absence everywhere.
//
// Every case uses core words for the *names* too, so the strings under
// test genuinely could be dispatched if the invariant were broken —
// `"if"`/`"add"` would error or change the result the moment they ran.
func TestStringsNeverPromotedToWords(t *testing.T) {
	// check runs src and asserts the residual stack renders to want. A
	// run error almost always means a string was dispatched as a word
	// (e.g. `if` with no branches), so the error message says so.
	check := func(t *testing.T, src, want string) {
		t.Helper()
		out, err := runExpr(t, src)
		if err != nil {
			t.Fatalf("%s\n  unexpected error (a string was likely dispatched as a word): %v", src, err)
		}
		if got := formatStack(out); got != want {
			t.Errorf("%s\n   got: %s\n  want: %s", src, got, want)
		}
	}

	// 1. A word-naming string stays data in every container, at depth:
	// bare lists, nested lists, maps, nested maps, lists in maps, and
	// typed lists/maps.
	t.Run("containers", func(t *testing.T) {
		cases := []struct{ src, want string }{
			{`["add" "if" "dup"]`, `['add' 'if' 'dup']`},
			{`[["if"] ["add"]]`, `[['if'] ['add']]`},
			{`{a:"add" b:"if"}`, `{a:'add' b:'if'}`},
			{`{a:{b:"add"}}`, `{a:{b:'add'}}`},
			{`{a:["add" "if"]}`, `{a:['add' 'if']}`},
			{`[:String "add" "if"] size`, `2`},
			{`[:String "add" "if"] 0 get`, `'add'`},
			{`{:String a:"add"} "a" get`, `'add'`},
		}
		for _, c := range cases {
			check(t, c.src, c.want)
		}
	})

	// 2. `do` evaluates a list value as code, but a quoted string inside
	// that code is data — it is not dispatched (this is exactly DX report
	// T4). Recurses through nested maps and applies to `do [list]` too.
	t.Run("do_evaluation", func(t *testing.T) {
		cases := []struct{ src, want string }{
			{`do {a:["add"]}`, `{a:'add'}`},
			{`do {a:["if" "get" "dup"]}`, `{a:['if' 'get' 'dup']}`},
			{`do {a:{b:["if"]}}`, `{a:{b:'if'}}`},
			{`do ["add" "dup"]`, `'add' 'dup'`},
		}
		for _, c := range cases {
			check(t, c.src, c.want)
		}
	})

	// 3. Data-manipulation core words operate on the strings as values;
	// they never run them. (The code BODY of `each` still runs — `dup` in
	// `[dup]` is an unquoted word — but the string ELEMENTS do not.)
	t.Run("data_words", func(t *testing.T) {
		cases := []struct{ src, want string }{
			{`["add" "if"] each [dup]`, `['add' 'if']`},
			{`["add" "if"] reverse`, `['if' 'add']`},
			{`["add" "if"] size`, `2`},
			{`["add" "if"] 0 get`, `'add'`},
			{`["if"] "add" push`, `['if' 'add']`},
			{`["if" "add"] "" join`, `'ifadd'`},
			{`"add" dup`, `'add' 'add'`},
			{`def f fn [[s:Any] [Any] [s]] end "if" f`, `'if'`},
		}
		for _, c := range cases {
			check(t, c.src, c.want)
		}
	})

	// 4. The quoting contrast — the definitive proof. Same shapes; the
	// only difference is the quote, and it flips run-vs-store. If strings
	// were promoted, the quoted forms would match the unquoted ones.
	t.Run("quoting_contrast", func(t *testing.T) {
		check(t, `[add 1 2]`, `[3]`)                      // unquoted word runs
		check(t, `["add" 1 2]`, `['add' 1 2]`)            // quoted string is data
		check(t, `do {a:[add 1 2]}`, `{a:3}`)             // unquoted runs
		check(t, `do {a:["add" 1 2]}`, `{a:['add' 1 2]}`) // quoted stays
	})

	// 5. A module body is code, but a quoted string naming a word inside
	// it is still data — the body builds without dispatching it. If `"if"`
	// were promoted it would dispatch with no branches and error.
	t.Run("module_body", func(t *testing.T) {
		out, err := runExpr(t, `module ["if" def x 7 export "Foo" {v: x}]`)
		if err != nil {
			t.Fatalf("module body dispatched a quoted string: %v", err)
		}
		if got := formatStack(out); !strings.Contains(got, "Module") {
			t.Errorf("expected a Module value, got: %s", got)
		}
	})
}
