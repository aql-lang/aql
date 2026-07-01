package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestBranchValueDefPromote pins the fix for a value-def bound to a 2-arm `if`
// result that is read MORE THAN ONCE — `def bi (if (raw gte n) [(n sub 1)] [raw])`
// then `bcount set bi ((bcount get bi) add 1)` (bucket-sort's bucket-index clamp).
// planValueDefLocals USED to skip every evBranch result (only ever dead-dropping a
// zero-ref merge, never promoting), so bi's single simulated-stack copy was consumed
// by the first use (`bcount get bi`) and the SECOND use (the `set` key) could not be
// seated — "stack discipline: operands of set not adjacent on top". A multiply-read
// single-value 2-arm if-result value-def is now PROMOTED to a frame local (stored
// after the branch, references re-push from the slot), exactly like a multiply-read
// call result. This is the bucket-sort leaf; compile == interpret MUST hold: compiles
// natively (no island), RunCompiledStrict == Run.
func TestBranchValueDefPromote(t *testing.T) {
	strict := []struct{ name, src, want string }{
		// the bucket clamp shape: an if-result value-def read by a get AND a set key.
		{"if-result value-def read twice (bucket clamp)",
			`def bcount (flex [0 0 0])
def n 3
def raw 5
def bi (if (raw gte n) [(n sub 1)] [raw])
def _ (bcount set bi ((bcount get bi) add 1)) end
(node bcount)`, "[[0 0 1]]"},
		// an if-result read across a get and an arithmetic use (two reads, no set).
		{"if-result value-def in two arithmetic reads",
			`def k (if (3 gt 1) [2] [0])
((k mul 10) add k)`, "[22]"},
		// inside an each-closure body (the real bucket nesting): if-result read twice.
		{"if-result value-def read twice inside an each body",
			`import module [
  def srt fn [[xs:List] [List] [
    def bcount (flex (iota 3 each [drop 0]))
    def n 3
    def _bc (iota 4 each [ var [[i]
      def raw (i add 1)
      def bi (if (raw gte n) [(n sub 1)] [raw])
      def _ (bcount set bi ((bcount get bi) add 1)) end
      0
    ] ])
    (node bcount)
  ]]
  export "M" {srt: srt/r}
] end ([0 0 0 0] M.srt)`, "[[0 1 3]]"},
	}
	for _, c := range strict {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile natively, refused: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("%s must compile native (no island)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.Run(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
