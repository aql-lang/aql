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
	// Interpolated XML literal: walk every ${...} expression in the
	// skeleton (attribute values and child holes, recursively) so a
	// closure over `<p>${x}</p>` captures `x`.
	if IsXmlInterp(v) {
		tmpl, _ := AsXmlInterp(v)
		walkXmlTmplExprs(tmpl, callback)
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
	// Reach (`.`/`!.` access, e.g. `e.code`, `m.a.(k)`): the receiver is a
	// value/word stream and a computed key is an expression. Walk both so a
	// name used ONLY as a reach receiver (`e1.code`) or inside a computed key
	// (`m.(k)`) is still seen — both for closure-capture analysis (a closure
	// over `x.field` must capture `x`) and for check-mode use-recording (the
	// receiver `def` is genuinely used). Literal bare-word keys (`.code`) are
	// field names, not references, and are NOT walked.
	if IsReach(v) {
		if info, err := AsReach(v); err == nil {
			for _, t := range info.Receiver {
				walkBodyValue(t, callback)
			}
			for _, seg := range info.Segments {
				for _, t := range seg.KeyExpr {
					walkBodyValue(t, callback)
				}
			}
		}
		return
	}
	// All other shapes (numbers, strings, booleans, atoms, type
	// literals, markers): nothing to capture.
}

// walkXmlTmplExprs walks every ${...} expression token in an XML
// interpolation skeleton — attribute values and child holes — recursing
// into nested child templates, so closure-capture analysis sees the word
// references inside a `<tag attr=${e}>${e2}</tag>` literal.
func walkXmlTmplExprs(t XmlTmpl, callback func(WordInfo, Value)) {
	for _, a := range t.Attr {
		for _, p := range a.Parts {
			for _, tok := range p.Expr {
				walkBodyValue(tok, callback)
			}
		}
	}
	for _, c := range t.Cren {
		switch c.Kind {
		case XmlCrenExpr:
			for _, tok := range c.Expr {
				walkBodyValue(tok, callback)
			}
		case XmlCrenChild:
			if c.Child != nil {
				walkXmlTmplExprs(*c.Child, callback)
			}
		}
	}
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
	// Names bound by a `def NAME …` (or `var [[NAME …] …]`) INSIDE this body are
	// the body's OWN locals — the closure-body compile promotes them to frame
	// slots, so they are never captures, even though an analysis sub-engine run of
	// the body leaves them bound in r ABOVE the enclosing-fn baseline (which would
	// otherwise make the depth check below grab them). This is the each/closure
	// body-local-value-def leaf: an each body `[def j (cur get 0) j]` must capture
	// only `cur` (the genuine enclosing binding), not its own `j`.
	bodyLocals := map[string]bool{}
	collectBodyLocalDefs(sig.body(), bodyLocals)
	seen := map[string]Value{}
	WalkBodyWords(sig.body(), func(w WordInfo, _ Value) {
		if w.Name == "" || paramNames[w.Name] || bodyLocals[w.Name] {
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

// collectBodyLocalDefs gathers the names a body binds for ITSELF — `def NAME …`
// at any non-closure depth, plus `var [[NAME …] …]` temporaries — into locals.
// These are frame-locals of the body being analysed, NOT captures (see
// ComputeCaptures). It recurses into list / paren tokens but NOT into a nested
// FnDefInfo (an inner closure owns its own locals + capture analysis), mirroring
// walkBodyValue's descent rules.
func collectBodyLocalDefs(body []Value, locals map[string]bool) {
	for i := 0; i < len(body); i++ {
		v := body[i]
		if v.Quoted {
			continue
		}
		if _, isFn := v.Data.(FnDefInfo); isFn {
			continue // nested closure: its defs are its own
		}
		if w, err := AsWord(v); err == nil {
			switch w.Name {
			case "def":
				// def NAME … — NAME is the next bare-word token.
				if i+1 < len(body) {
					if nw, nerr := AsWord(body[i+1]); nerr == nil && nw.Name != "" {
						locals[nw.Name] = true
					}
				}
			case "var":
				// var [[NAME …] body] — the decl list's first element holds the
				// temporary names; they desugar to `def NAME` at run time.
				if i+1 < len(body) && body[i+1].Parent.Equal(TList) && body[i+1].Data != nil {
					if outer, lerr := AsList(body[i+1]); lerr == nil {
						elems := outer.Slice()
						if len(elems) > 0 && elems[0].Parent.Equal(TList) && elems[0].Data != nil {
							if decls, derr := AsList(elems[0]); derr == nil {
								for _, d := range decls.Slice() {
									if dw, dwe := AsWord(d); dwe == nil && dw.Name != "" {
										locals[dw.Name] = true
									}
								}
							}
						}
					}
				}
			}
		}
		if v.Parent.Equal(TList) && v.Data != nil {
			if lst, lerr := AsList(v); lerr == nil {
				collectBodyLocalDefs(lst.Slice(), locals)
			}
		} else if IsParenExpr(v) {
			if toks, perr := AsParenExpr(v); perr == nil {
				collectBodyLocalDefs(toks, locals)
			}
		}
	}
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
