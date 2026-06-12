package eng

// Tape — a gap-buffer tape for the engine. PROTOTYPE (not yet wired
// into Engine; see design/TAPE-DATA-STRUCTURE.10.md).
//
// The engine executes on a tape of Values with a cursor (Engine.pointer)
// that mostly moves forward, and every structural edit — body splice,
// forward parking, result splice — lands at or within a few tokens of
// the cursor. The current []Value representation makes each such edit
// memmove the entire tail beyond the edit point; during recursion the
// tail holds every enclosing call's pending continuation, so the cost is
// O(depth) per edit and O(depth²) overall (measured: 95.9% of a deep
// recursion's runtime is runtime.memmove — design/RECURSION-PERFORMANCE.10.md).
//
// A gap buffer is the text-editor answer to exactly this access pattern
// (Emacs uses one per buffer): the storage keeps one hole (the gap) at
// the cursor. Logical layout:
//
//	indices  0 … gapStart-1   [ gap ]   gapStart … Len()-1
//	physical buf[:gapStart]  (unused)  buf[gapEnd:]
//
// Edits AT the gap are O(edit size): insert writes into the hole, delete
// widens it. Moving the cursor k positions moves k Values across the
// hole. The tail beyond the gap NEVER moves for an edit at the gap — the
// recursion tail therefore stays put and deep recursion becomes O(depth)
// total instead of O(depth²).
//
// Random access stays O(1) (one branch translates a logical index across
// the gap), and both regions are contiguous, so the two scan directions
// the engine uses (backward over collected values, forward over future
// tokens) remain cache-friendly contiguous walks.
type Tape struct {
	buf      []Value
	gapStart int // physical index of the first gap slot == logical gap position
	gapEnd   int // physical index one past the last gap slot
}

// NewTape builds a tape holding the given values with the gap at the
// end. The input is copied, mirroring Engine.Run's copy of its program.
func NewTape(vals []Value, headroom int) *Tape {
	if headroom < 0 {
		headroom = 0
	}
	buf := make([]Value, len(vals)+headroom)
	copy(buf, vals)
	return &Tape{buf: buf, gapStart: len(vals), gapEnd: len(buf)}
}

// Len reports the number of logical elements.
func (t *Tape) Len() int { return len(t.buf) - (t.gapEnd - t.gapStart) }

// phys translates a logical index to a physical one.
func (t *Tape) phys(i int) int {
	if i < t.gapStart {
		return i
	}
	return i + (t.gapEnd - t.gapStart)
}

// At returns the element at logical index i. Out-of-range reads return
// the zero Value (never panic — ADR-005); the engine bounds-checks its
// pointer before reading, so this is a belt-and-braces guard.
func (t *Tape) At(i int) Value {
	if i < 0 || i >= t.Len() {
		return Value{}
	}
	return t.buf[t.phys(i)]
}

// Set replaces the element at logical index i. Out-of-range writes are
// ignored (never panic — ADR-005).
func (t *Tape) Set(i int, v Value) {
	if i < 0 || i >= t.Len() {
		return
	}
	t.buf[t.phys(i)] = v
}

// MoveGap relocates the gap so it sits at logical index i — O(|i - gap|)
// Value copies. The engine calls this implicitly via the edit methods;
// the cursor's step-by-step advance makes the move cheap (usually 0-4).
func (t *Tape) MoveGap(i int) {
	if i < 0 {
		i = 0
	}
	if max := t.Len(); i > max {
		i = max
	}
	switch {
	case i < t.gapStart:
		n := t.gapStart - i
		copy(t.buf[t.gapEnd-n:t.gapEnd], t.buf[i:t.gapStart])
		zero(t.buf[i : i+min(n, t.gapEnd-t.gapStart)])
		t.gapStart, t.gapEnd = i, t.gapEnd-n
	case i > t.gapStart:
		n := i - t.gapStart
		copy(t.buf[t.gapStart:t.gapStart+n], t.buf[t.gapEnd:t.gapEnd+n])
		zero(t.buf[max(t.gapEnd, t.gapStart+n) : t.gapEnd+n])
		t.gapStart, t.gapEnd = i, t.gapEnd+n
	}
}

// grow widens the gap to at least need slots, reallocating once.
func (t *Tape) grow(need int) {
	if t.gapEnd-t.gapStart >= need {
		return
	}
	newCap := len(t.buf)*2 + need
	nb := make([]Value, newCap)
	copy(nb, t.buf[:t.gapStart])
	tail := len(t.buf) - t.gapEnd
	copy(nb[newCap-tail:], t.buf[t.gapEnd:])
	t.buf = nb
	t.gapEnd = newCap - tail
}

// Insert places v at logical index i, shifting later elements right
// (logically). O(gap distance + 1).
func (t *Tape) Insert(i int, v Value) {
	if i < 0 {
		i = 0
	}
	if max := t.Len(); i > max {
		i = max
	}
	t.MoveGap(i)
	t.grow(1)
	t.buf[t.gapStart] = v
	t.gapStart++
}

// Remove deletes the element at logical index i. O(gap distance).
func (t *Tape) Remove(i int) {
	if i < 0 || i >= t.Len() {
		return
	}
	t.MoveGap(i)
	// The element at logical i sits just after the gap; widen over it.
	t.buf[t.gapEnd] = Value{} // release references
	t.gapEnd++
}

// Splice replaces count elements starting at logical index i with the
// replacements. O(gap distance + count + len(repl)).
func (t *Tape) Splice(i, count int, repl ...Value) {
	if i < 0 {
		i = 0
	}
	if max := t.Len(); i > max {
		i = max
	}
	if count < 0 {
		count = 0
	}
	if avail := t.Len() - i; count > avail {
		count = avail
	}
	t.MoveGap(i)
	// Consume count elements after the gap.
	for k := 0; k < count; k++ {
		t.buf[t.gapEnd+k] = Value{}
	}
	t.gapEnd += count
	t.grow(len(repl))
	copy(t.buf[t.gapStart:], repl)
	t.gapStart += len(repl)
}

// Prefix returns the contiguous region below logical index end, when end
// is at or below the gap — the engine's dominant "scan everything before
// the pointer" view (hint scans, reorder candidates, handler stacks).
// With the gap kept at the cursor this is the zero-copy fast path; a
// request crossing the gap falls back to a copy via CopyRange.
func (t *Tape) Prefix(end int) []Value {
	if end < 0 {
		end = 0
	}
	if end > t.Len() {
		end = t.Len()
	}
	if end <= t.gapStart {
		return t.buf[:end]
	}
	return t.CopyRange(0, end)
}

// CopyRange returns a fresh copy of logical [i, j).
func (t *Tape) CopyRange(i, j int) []Value {
	if i < 0 {
		i = 0
	}
	if j > t.Len() {
		j = t.Len()
	}
	if i >= j {
		return nil
	}
	out := make([]Value, j-i)
	for k := range out {
		out[k] = t.buf[t.phys(i+k)]
	}
	return out
}

// Snapshot returns a fresh copy of the whole logical content (trace
// callbacks, end-of-run drains).
func (t *Tape) Snapshot() []Value { return t.CopyRange(0, t.Len()) }

func zero(s []Value) {
	for i := range s {
		s[i] = Value{}
	}
}
