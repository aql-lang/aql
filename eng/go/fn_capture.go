package eng

import (
	"sort"
)

// WalkBodyWords recursively visits every bare Word in a fn body's
// value stream, invoking callback for each. Used by computeCaptures
// to enumerate the names a body references at construction time.
//
// Walks INTO: nested lists (auto-eval or quoted), paren-expr
// payloads, interpolated-string expression parts. Does NOT walk into
// quoted lists (they're data), nor into nested FnDefInfo payloads
// (those are inner closures with their own capture computation).
//
// The walker is strictly read-only — it does not mutate body or
// registry state.
func WalkBodyWords(body []Value, callback func(WordInfo, Value)) {
	for _, v := range body {
		walkBodyValue(v, callback)
	}
}

func walkBodyValue(v Value, callback func(WordInfo, Value)) {
	// Quoted values are data — skip.
	if v.Quoted {
		return
	}
	// Bare Word: emit.
	if IsWord(v) {
		w, _ := AsWord(v)
		callback(w, v)
		return
	}
	// Nested FnDefInfo: opaque. Its captures were resolved at its
	// own construction; don't descend.
	if _, ok := v.Data.(FnDefInfo); ok {
		return
	}
	// List payload: recurse.
	if v.Parent.Equal(TList) && v.Data != nil {
		lst, _ := AsList(v)
		for _, e := range lst.Slice() {
			walkBodyValue(e, callback)
		}
		return
	}
	// Paren-expr payload (stored inside map values when a paren
	// group appears as a data position): walk the inner tokens.
	if IsParenExpr(v) {
		toks, _ := AsParenExpr(v)
		for _, t := range toks {
			walkBodyValue(t, callback)
		}
		return
	}
	// Interpolated string: walk each expression part.
	if IsInterpString(v) {
		parts, _ := AsInterpString(v)
		for _, p := range parts {
			for _, t := range p.Expr {
				walkBodyValue(t, callback)
			}
		}
		return
	}
	// Map payload: walk each value (keys are strings, not Words).
	if v.Parent.Equal(TMap) && v.Data != nil {
		m, _ := AsMap(v)
		if m == nil {
			return
		}
		for _, key := range m.Keys() {
			mv, _ := m.Get(key)
			walkBodyValue(mv, callback)
		}
		return
	}
	// All other shapes (numbers, strings, booleans, atoms, type
	// literals, markers): nothing to capture.
}

// ComputeCaptures walks a single fn-sig body and returns the list of
// enclosing-fn-local bindings the body references. Returns nil at top
// level (no enclosing fn → no captures) or when no body Word resolves
// to an enclosing-fn local. Result is sorted by name for deterministic
// install order at dispatch.
//
// Capture rule: name is captured iff (1) the name is currently bound
// in r.Defs AND (2) Depth(name) > baseline[name] where baseline is the
// innermost TopFnBaseline. Names not bound (forward refs / recursion),
// names bound at module/global scope (Depth ≤ baseline), and the sig's
// own named params are all skipped.
func ComputeCaptures(r *Registry, sig *FnSig) []CapturedBinding {
	if r == nil {
		return nil
	}
	baseline := r.TopFnBaseline()
	if baseline == nil {
		return nil
	}
	paramNames := make(map[string]bool, len(sig.Params))
	for _, p := range sig.Params {
		if p.Name != "" {
			paramNames[p.Name] = true
		}
	}
	seen := map[string]Value{}
	WalkBodyWords(sig.Body, func(w WordInfo, _ Value) {
		if w.Name == "" || paramNames[w.Name] {
			return
		}
		if _, dup := seen[w.Name]; dup {
			return
		}
		v, ok := r.Defs.Top(w.Name)
		if !ok {
			return
		}
		if r.Defs.Depth(w.Name) <= baseline[w.Name] {
			return
		}
		seen[w.Name] = v
	})
	if len(seen) == 0 {
		return nil
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]CapturedBinding, len(names))
	for i, n := range names {
		out[i] = CapturedBinding{Name: n, Value: seen[n]}
	}
	return out
}

// MergeCaptures combines per-sig capture lists into a single
// deduplicated list. Multi-sig fns get one captures list at the
// FnDefInfo level; if the same name appears in two sigs we take the
// first (they came from the same Defs.Top at the same construction
// time, so the Values are identical anyway).
func MergeCaptures(perSig [][]CapturedBinding) []CapturedBinding {
	if len(perSig) == 0 {
		return nil
	}
	if len(perSig) == 1 {
		return perSig[0]
	}
	seen := map[string]Value{}
	for _, list := range perSig {
		for _, cb := range list {
			if _, dup := seen[cb.Name]; dup {
				continue
			}
			seen[cb.Name] = cb.Value
		}
	}
	if len(seen) == 0 {
		return nil
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]CapturedBinding, len(names))
	for i, n := range names {
		out[i] = CapturedBinding{Name: n, Value: seen[n]}
	}
	return out
}
