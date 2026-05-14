package eng

// Stack-slice primitives used by the Engine. Kept as free functions
// (taking *[]Value rather than *Engine) because they only manipulate
// the slice — they have no engine-state dependency. Engine callers
// pass &e.stack explicitly so the read sites are unambiguous about
// what's being mutated.

// stackInsert inserts val at index i, shifting elements right. Only
// allocates when capacity is exhausted.
func stackInsert(s *[]Value, i int, val Value) {
	*s = append(*s, Value{})
	copy((*s)[i+1:], (*s)[i:len(*s)-1])
	(*s)[i] = val
}

// stackRemove removes the element at index i, shifting elements left.
// Zeroes the freed slot to release interface references.
func stackRemove(s *[]Value, i int) {
	copy((*s)[i:], (*s)[i+1:])
	(*s)[len(*s)-1] = Value{}
	*s = (*s)[:len(*s)-1]
}

// stackSplice removes count elements starting at index i and inserts
// replacements in their place. Only allocates when net growth exceeds
// capacity.
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
