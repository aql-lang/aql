package eng

import (
	"math/rand/v2"
	"testing"
)

// refTape is the trivially-correct reference model: a flat slice with
// the same logical operations. The differential test below drives Tape
// and refTape with the same random op sequence and asserts identical
// logical content after every step.
type refTape struct{ s []Value }

func (r *refTape) Len() int           { return len(r.s) }
func (r *refTape) At(i int) Value     { return r.s[i] }
func (r *refTape) Set(i int, v Value) { r.s[i] = v }
func (r *refTape) Insert(i int, v Value) {
	r.s = append(r.s, Value{})
	copy(r.s[i+1:], r.s[i:len(r.s)-1])
	r.s[i] = v
}
func (r *refTape) Remove(i int) {
	copy(r.s[i:], r.s[i+1:])
	r.s = r.s[:len(r.s)-1]
}
func (r *refTape) Splice(i, count int, repl ...Value) {
	tail := append([]Value{}, r.s[i+count:]...)
	r.s = append(r.s[:i], append(repl, tail...)...)
}

func tapeEquals(t *testing.T, tape *Tape, ref *refTape, step int) {
	t.Helper()
	if tape.Len() != ref.Len() {
		t.Fatalf("step %d: Len = %d, want %d", step, tape.Len(), ref.Len())
	}
	for i := 0; i < ref.Len(); i++ {
		a, _ := AsInteger(tape.At(i))
		b, _ := AsInteger(ref.At(i))
		if a != b {
			t.Fatalf("step %d: At(%d) = %d, want %d", step, i, a, b)
		}
	}
}

// TestTapeDifferential drives Tape and the slice reference with the
// same pseudo-random operation sequence (seeded — deterministic) and
// checks full logical equality after every operation.
func TestTapeDifferential(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))
	tape := NewTape(nil, 8)
	ref := &refTape{}
	next := int64(0)

	for step := 0; step < 5000; step++ {
		n := ref.Len()
		switch op := rng.IntN(5); {
		case op == 0 || n == 0: // insert
			i := rng.IntN(n + 1)
			v := NewInteger(next)
			next++
			tape.Insert(i, v)
			ref.Insert(i, v)
		case op == 1: // remove
			i := rng.IntN(n)
			tape.Remove(i)
			ref.Remove(i)
		case op == 2: // set
			i := rng.IntN(n)
			v := NewInteger(next)
			next++
			tape.Set(i, v)
			ref.Set(i, v)
		case op == 3: // splice
			i := rng.IntN(n + 1)
			count := 0
			if i < n {
				count = rng.IntN(n - i + 1)
			}
			repl := make([]Value, rng.IntN(4))
			for k := range repl {
				repl[k] = NewInteger(next)
				next++
			}
			tape.Splice(i, count, repl...)
			ref.Splice(i, count, repl...)
		default: // gap move (no logical change)
			tape.MoveGap(rng.IntN(n + 1))
		}
		tapeEquals(t, tape, ref, step)
	}
}

func TestTapeBasics(t *testing.T) {
	vals := []Value{NewInteger(1), NewInteger(2), NewInteger(3)}
	tp := NewTape(vals, 4)
	if tp.Len() != 3 {
		t.Fatalf("Len = %d, want 3", tp.Len())
	}
	// NewTape copies its input.
	vals[0] = NewInteger(99)
	if n, _ := AsInteger(tp.At(0)); n != 1 {
		t.Error("NewTape aliased the caller's slice")
	}

	tp.Insert(1, NewInteger(10))   // 1 10 2 3
	tp.Remove(3)                   // 1 10 2
	tp.Splice(0, 2, NewInteger(7)) // 7 2
	want := []int64{7, 2}
	for i, w := range want {
		if n, _ := AsInteger(tp.At(i)); n != w {
			t.Errorf("At(%d) = %d, want %d", i, n, w)
		}
	}
}

func TestTapeOutOfRangeNeverPanics(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("tape panicked on out-of-range op: %v", r)
		}
	}()
	tp := NewTape([]Value{NewInteger(1)}, 0)
	_ = tp.At(-1)
	_ = tp.At(99)
	tp.Set(-1, NewInteger(2))
	tp.Set(99, NewInteger(2))
	tp.Remove(-1)
	tp.Remove(99)
	tp.Insert(-5, NewInteger(3)) // clamps to 0
	tp.Insert(99, NewInteger(4)) // clamps to end
	tp.Splice(-1, 99, NewInteger(5))
	tp.MoveGap(-3)
	tp.MoveGap(99)
	_ = tp.Prefix(-1)
	_ = tp.CopyRange(5, 2)
}

func TestTapePrefixContiguous(t *testing.T) {
	tp := NewTape([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}, 4)
	tp.MoveGap(2) // gap at cursor position 2
	p := tp.Prefix(2)
	if len(p) != 2 {
		t.Fatalf("Prefix(2) len = %d, want 2", len(p))
	}
	if n, _ := AsInteger(p[1]); n != 2 {
		t.Errorf("Prefix[1] = %d, want 2", n)
	}
	// Crossing the gap falls back to a copy with the right content.
	p = tp.Prefix(3)
	if n, _ := AsInteger(p[2]); n != 3 {
		t.Errorf("Prefix(3)[2] = %d, want 3", n)
	}
}

// ---- benchmarks: the recursion access pattern ----
//
// One simulated call level: splice a 12-token body at the cursor, do 14
// insert+remove pairs there, then leave 8 tokens pending and move on —
// the op mix measured from the real engine (design/RECURSION-PERFORMANCE.10.md).
// The slice variant drags the pending tail on every edit; the Tape keeps
// the gap at the cursor so the tail never moves.

func benchSliceLevel(s *[]Value, ptr int, body []Value) int {
	stackSplice(s, ptr, 0, body...)
	ptr += len(body)
	var v Value
	for i := 0; i < 14; i++ {
		stackInsert(s, ptr, v)
		stackRemove(s, ptr)
	}
	return ptr - 8
}

func BenchmarkTapeSliceRecursion(b *testing.B) {
	body := make([]Value, 12)
	for depth := 1000; depth <= 4000; depth *= 2 {
		b.Run(itoa(depth), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				s := make([]Value, 0, 64)
				ptr := 0
				for d := 0; d < depth; d++ {
					ptr = benchSliceLevel(&s, ptr, body)
				}
			}
		})
	}
}

func BenchmarkTapeGapRecursion(b *testing.B) {
	body := make([]Value, 12)
	for depth := 1000; depth <= 4000; depth *= 2 {
		b.Run(itoa(depth), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				tp := NewTape(nil, 64)
				ptr := 0
				var v Value
				for d := 0; d < depth; d++ {
					tp.Splice(ptr, 0, body...)
					ptr += len(body)
					for i := 0; i < 14; i++ {
						tp.Insert(ptr, v)
						tp.Remove(ptr)
					}
					ptr -= 8
				}
			}
		})
	}
}

// ---- former engine primitives (engine_stack.go), retained as the
// benchmark baseline after the engine moved to the gap-buffer Tape ----

func stackInsert(s *[]Value, i int, val Value) {
	*s = append(*s, Value{})
	copy((*s)[i+1:], (*s)[i:len(*s)-1])
	(*s)[i] = val
}

func stackRemove(s *[]Value, i int) {
	copy((*s)[i:], (*s)[i+1:])
	(*s)[len(*s)-1] = Value{}
	*s = (*s)[:len(*s)-1]
}

func stackSplice(s *[]Value, i, count int, replacements ...Value) {
	delta := len(replacements) - count
	oldLen := len(*s)
	newLen := oldLen + delta

	if delta > 0 {
		for cap(*s) < newLen {
			*s = append(*s, Value{})
		}
		*s = (*s)[:newLen]
		copy((*s)[i+len(replacements):], (*s)[i+count:oldLen])
	} else if delta < 0 {
		copy((*s)[i+len(replacements):], (*s)[i+count:])
		for j := newLen; j < oldLen; j++ {
			(*s)[j] = Value{}
		}
		*s = (*s)[:newLen]
	}
	copy((*s)[i:], replacements)
}
