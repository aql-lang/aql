package lang

// Check-prop property-body Function-argument miscompile pin (the third voxgig-
// surfaced main-era regression, sibling of the check-prop GENERATOR miscompile).
//
// `Test.check-prop`'s gen / property bodies compile as STORED-PARAM units
// (compileStoredParamBody, since 853dcaa4) — their params bind as CARRIERS. A
// carrier receiver makes signature dispatch of a competing-arity window
// ambiguous: the sort library's `(lst Sort.quick Sort.by-number)` — dispatch
// the comparator-taking sort `Sort.quick` over the generated list `lst`, passing
// the comparator fn `Sort.by-number` — re-matched to `Sort.by-number` (its
// Any/Any params greedily bound the `lst` carrier + the `Sort.quick` fn-VALUE)
// where the interpreter, over the CONCRETE list, dispatches `Sort.quick`. The
// compiled unit inverted the call — `cmp(lst, quick-fn)` — raising
// `cmp: cannot order List and Function` under `--compile` while the interpreter
// passed. A silent-fallback contract violation (sort_prop_test, 3 of 7 props).
//
// RecordUserCall now DECLINES a fn-VALUE argument to a dispatch inside a stored-
// ref body (inStoredRefUnit): the body falls back to the interpreter, which
// dispatches over the concrete generated value byte-identically. A regular fn
// body compiles the identical source correctly (concrete-enough operands) and is
// untouched — TestCheckPropFnArgRegularFnBodyStillCompiles pins that side.

import (
	"fmt"
	"strings"
	"testing"
)

// The miniature of the sort_prop_test shape: a check-prop property body that
// dispatches a comparator-taking sort (params comparator-FIRST, applying the
// comparator via `(a b comp)` inside a nested each — the quick-go shape) over
// the generated list, passing a module comparator fn as the argument. Before
// the fix the compiled property returned ok:false (cmp List/Function); it must
// now agree with the interpreter (compile == interpret, both ok:true).
func TestCheckPropStoredBodyFnArgDispatchMatchesInterp(t *testing.T) {
	src := `import "aql:test" end
import module [
  def by-number fn [[b:Any a:Any] [Integer] [(a cmp b)]]
  def srt fn [[comp:Function lst:List] [List] [
    def arr (flex lst)
    def _ (iota (lst size) each [ var [[i]
      def c ((arr get i) (arr get 0) comp)
      0
    ]])
    (arr slice 0 (lst size))
  ]]
  export "S" {by-number: by-number/r, srt: srt/r}
] end
Test.check-prop "sort-dispatch"
  [ iota (r.int 0 5) each [ var [[i] r.int 0 12 ] ] ]
  [ var [[lst] (lst S.srt S.by-number) drop true ] ]
  25 1 0
end`
	a, _ := New()
	want, werr := a.RunInterp(src)
	b, _ := New()
	got, _, gerr := b.RunCompiled(src)
	if (werr == nil) != (gerr == nil) {
		t.Fatalf("error disagreement (compile != interpret):\n  interp:   %v\n  compiled: %v", werr, gerr)
	}
	if werr == nil && fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("SILENT MISCOMPILE (compile != interpret):\n  interp:   %v\n  compiled: %v", want, got)
	}
	// The property must actually PASS on both surfaces — a shared `ok:false`
	// would match yet still be the bug (the miscompile fails the property).
	if !strings.Contains(fmt.Sprint(want), "true") {
		t.Fatalf("expected the property to hold under the interpreter, got %v", want)
	}
}

// The sibling safety property: the SAME module-fn dispatch with a fn-value
// argument, in a REGULAR fn body (NOT a stored-ref unit), must never miscompile
// — the decline is scoped to stored-ref bodies (inStoredRefUnit), so a regular
// body compiles or soundly falls back, but always agrees with the interpreter.
func TestCheckPropFnArgRegularFnBodyMatchesInterp(t *testing.T) {
	stage1aSound(t, `import module [
  def by-number fn [[b:Any a:Any] [Integer] [(a cmp b)]]
  def srt fn [[comp:Function lst:List] [List] [ lst comp sortby ]]
  export "S" {by-number: by-number/r, srt: srt/r}
] end
def run fn [[xs:List] [List] [(xs S.srt S.by-number)]]
([3 1 2] run) end`)
}
