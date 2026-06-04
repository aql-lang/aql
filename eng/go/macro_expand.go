package eng

import (
	"fmt"
	"strings"
)

// Macro expansion (design/MACROS-PHASE1.0.md §6). A macro is an FnDef flagged
// Macro=true whose every param is FormArgs raw-capture. At dispatch the
// operands are captured unevaluated, the template body is run, the returned
// token list is walked to resolve `unquote`/`splice`, and the result is
// spliced into the call site via an __SP marker that re-steps the tape.
//
// Model A (the plan's): the template is data (a `quote [ … ]` list); the
// expander walks it resolving the escapes — it is NOT live `unquote`/`splice`
// words run during the body. The walk recurses into List, ParenExpr, AND Map
// values (the §3.1 map channel).

// execMacro is the dispatch entry: the macro FnDef value sits at valIdx with
// its operands ahead on the tape (still raw — the macro branch fires before
// preEvalParens). It captures `arity` operand forms, expands, and splices the
// expansion in place of the `mac operand…` span.
func (e *Engine) execMacro(valIdx int, fnDef *FnDefInfo) error {
	if len(fnDef.Signatures) == 0 {
		return fmt.Errorf("macro %s: no signature", fnDef.Name)
	}
	sig := &fnDef.Signatures[0]
	arity := len(sig.Params)

	operands, endIdx, err := e.scanMacroOperands(fnDef, arity, valIdx)
	if err != nil {
		return err
	}
	// Memoize on (name + operand canon): the expansion is deterministic, so a
	// macro re-applied to the same operand forms (e.g. in a loop) expands once
	// and re-splices the cached tokens. See design/MACROS-PHASE1.0.md §8.
	key := macroOperandsKey(fnDef.Name, operands)
	expanded, ok := e.registry.macroCacheGet(key)
	if !ok {
		expanded, err = expandMacroWith(e.registry, fnDef, operands)
		if err != nil {
			return err
		}
		e.registry.macroCachePut(key, expanded)
	}
	// Replace the whole `mac operand…` span (valIdx..endIdx) with one __SP
	// marker; e.pointer == valIdx is left unchanged so the Run loop steps the
	// marker next, splicing the expansion onto the live tape and re-stepping.
	stackSplice(&e.stack, valIdx, endIdx-valIdx+1, NewSplice(NewList(expanded)))
	return nil
}

// macroOperandsKey builds the expansion-cache key from the macro name and the
// canon of each operand form — two calls with the same name and operand forms
// share an expansion, regardless of where they appear.
func macroOperandsKey(name string, operands []Value) string {
	var b strings.Builder
	b.WriteString(name)
	for _, op := range operands {
		b.WriteByte(0)
		b.WriteString(CanonValue(op))
	}
	return b.String()
}

// scanMacroOperands grabs the next `arity` operand FORMS after the macro value
// at valIdx — each a single tape value (a Word, literal, ParenExpr, List, Map,
// or Reach), stopping at a structural boundary. Returns the operands and the
// index of the last one. Errors if fewer than `arity` are available (a macro
// used without enough operands — including "as a bare value").
func (e *Engine) scanMacroOperands(fnDef *FnDefInfo, arity, valIdx int) ([]Value, int, error) {
	operands := make([]Value, 0, arity)
	idx := valIdx + 1
	last := valIdx
	for len(operands) < arity && idx < len(e.stack) {
		t := e.stack[idx]
		if IsEnd(t) || IsCloseParen(t) || IsOpenParen(t) || IsForward(t) ||
			t.Parent.Matches(TMark) || t.Parent.Matches(TMove) ||
			t.Parent.Matches(TInternal) || t.Parent.Matches(TReturnCheck) {
			break
		}
		operands = append(operands, t)
		last = idx
		idx++
	}
	if len(operands) < arity {
		return nil, 0, &AqlError{
			Code: "macro_error",
			Detail: fmt.Sprintf("macro %s expects %d operand(s), got %d",
				fnDef.Name, arity, len(operands)),
		}
	}
	return operands, last, nil
}

// expandMacroWith runs a macro's template body against the given raw operands
// and returns the expanded token list. The macro scope (params + captures +
// any body-locals) stays live across the template walk so `unquote`/`splice`
// resolve against it, then is torn down. Shared by execMacro and macroexpand.
func expandMacroWith(r *Registry, fnDef *FnDefInfo, operands []Value) ([]Value, error) {
	sig := &fnDef.Signatures[0]

	// Bind params to their raw operand forms. `bindings` (operand forms,
	// inert) is the fast path for `unquote <paramName>`; the same forms are
	// installed (quoted) in the def scope so an `unquote (expr)` referencing a
	// param resolves too. Body-locals created while running the template (e.g.
	// `def g (gensym)`) leak into the scope and stay live for the walk.
	snap := r.Defs.Snapshot()
	bindings := make(map[string]Value, len(sig.Params))
	for i, p := range sig.Params {
		if i >= len(operands) || p.Name == "" {
			continue
		}
		bindings[p.Name] = operands[i]
		q := operands[i]
		q.Quoted = true
		InstallDef(r, p.Name, q)
	}
	for _, cb := range fnDef.Captured {
		InstallDef(r, cb.Name, cb.Value)
	}

	// Run the template body → the template token list (its last result).
	body := make([]Value, len(sig.Body))
	copy(body, sig.Body)
	res, runErr := New(r).Run(body)
	if runErr != nil {
		r.Defs.Restore(snap)
		return nil, fmt.Errorf("macro %s: template: %w", fnDef.Name, runErr)
	}
	if len(res) == 0 {
		r.Defs.Restore(snap)
		return nil, &AqlError{Code: "macro_error",
			Detail: fmt.Sprintf("macro %s: template produced no value", fnDef.Name)}
	}
	template := res[len(res)-1]
	var tmpl []Value
	if template.Parent.Equal(TList) && IsConcrete(template) {
		l, _ := AsList(template)
		tmpl = l.Slice()
	} else {
		tmpl = []Value{template}
	}

	expanded, expErr := expandTemplate(r, tmpl, bindings)
	r.Defs.Restore(snap)
	if expErr != nil {
		return nil, fmt.Errorf("macro %s: %w", fnDef.Name, expErr)
	}
	return expanded, nil
}

// expandTemplate walks a template token list, resolving `unquote x` (insert one
// grouped node) and `splice xs` (flatten a list's elements), and recursing into
// nested List / ParenExpr / Map values. Bare tokens pass through as code.
func expandTemplate(r *Registry, toks []Value, bindings map[string]Value) ([]Value, error) {
	out := make([]Value, 0, len(toks))
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if IsWord(t) {
			w, _ := AsWord(t)
			if w.Name == "unquote" || w.Name == "splice" {
				if i+1 >= len(toks) {
					return nil, &AqlError{Code: "macro_error",
						Detail: w.Name + ": missing operand in template"}
				}
				val, err := resolveTemplateEscape(r, toks[i+1], bindings)
				if err != nil {
					return nil, err
				}
				i++ // consume the escape's operand
				if w.Name == "splice" {
					if !val.Parent.Equal(TList) || !IsConcrete(val) {
						return nil, &AqlError{Code: "macro_error",
							Detail: "splice: operand did not evaluate to a list"}
					}
					l, _ := AsList(val)
					out = append(out, l.Slice()...)
				} else {
					out = append(out, val)
				}
				continue
			}
		}
		nested, err := expandNested(r, t, bindings)
		if err != nil {
			return nil, err
		}
		out = append(out, nested)
	}
	return out, nil
}

// resolveTemplateEscape resolves the operand of an `unquote`/`splice`. A bare
// param name maps directly to its raw operand form (fast path, no eval); any
// other operand (a paren / nested expr / body-local word) is evaluated in the
// live macro scope.
//
// An ATOM result becomes a WORD: a name produced for splicing (a `gensym`, or
// any name value) must be an identifier in the generated code, not inert atom
// data. `def unquote g` then binds the name and `if unquote g` resolves it —
// whereas a spliced atom would be truthy data in value position.
func resolveTemplateEscape(r *Registry, operand Value, bindings map[string]Value) (Value, error) {
	if IsWord(operand) {
		w, _ := AsWord(operand)
		if form, ok := bindings[w.Name]; ok {
			return asTemplateNode(form), nil
		}
	}
	res, err := New(r).Run([]Value{operand})
	if err != nil {
		return Value{}, err
	}
	if len(res) == 0 {
		return Value{}, &AqlError{Code: "macro_error",
			Detail: "unquote/splice operand produced no value"}
	}
	return asTemplateNode(res[len(res)-1]), nil
}

// asTemplateNode normalizes a resolved escape value for insertion into the
// expansion: an Atom (a name — e.g. a gensym) becomes a Word so it acts as a
// code identifier; everything else passes through unchanged.
func asTemplateNode(v Value) Value {
	if IsAtom(v) {
		name, _ := AsAtom(v)
		w := NewWord(name)
		w.Pos = v.Pos
		return w
	}
	return v
}

// expandNested recurses the escape walk into container values (a template may
// embed unquote/splice inside a list, a paren, or a map value — §3.1).
func expandNested(r *Registry, t Value, bindings map[string]Value) (Value, error) {
	switch {
	case t.Parent.Equal(TList) && IsConcrete(t):
		l, _ := AsList(t)
		inner, err := expandTemplate(r, l.Slice(), bindings)
		if err != nil {
			return Value{}, err
		}
		nl := NewList(inner)
		nl.Quoted = t.Quoted
		return nl, nil
	case IsParenExpr(t):
		toks, _ := AsParenExpr(t)
		inner, err := expandTemplate(r, toks, bindings)
		if err != nil {
			return Value{}, err
		}
		return NewParenExpr(inner), nil
	case t.Parent.Equal(TMap) && IsConcrete(t):
		m, _ := AsMap(t)
		if m == nil {
			return t, nil
		}
		out := NewOrderedMap()
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			rv, err := expandNested(r, v, bindings)
			if err != nil {
				return Value{}, err
			}
			out.Set(k, rv)
		}
		return NewMap(out), nil
	default:
		return t, nil
	}
}

// ExpandMacroForm expands a `(mac operand…)` form and returns the resulting
// token list WITHOUT splicing or running it — the `macroexpand` introspection
// word. form is the captured raw call (a ParenExpr or List).
func ExpandMacroForm(r *Registry, form Value) ([]Value, error) {
	var toks []Value
	switch {
	case IsParenExpr(form):
		toks, _ = AsParenExpr(form)
	case form.Parent.Equal(TList) && IsConcrete(form):
		l, _ := AsList(form)
		toks = l.Slice()
	default:
		return nil, &AqlError{Code: "macroexpand_error",
			Detail: "macroexpand: expected a (macro operand…) form"}
	}
	if len(toks) == 0 || !IsWord(toks[0]) {
		return nil, &AqlError{Code: "macroexpand_error",
			Detail: "macroexpand: form must start with a macro word"}
	}
	name, _ := AsWord(toks[0])
	bound, ok := r.Defs.Top(name.Name)
	if !ok {
		return nil, &AqlError{Code: "macroexpand_error",
			Detail: "macroexpand: undefined macro " + name.Name}
	}
	fnDef, ok := bound.Data.(FnDefInfo)
	if !ok || !fnDef.Macro {
		return nil, &AqlError{Code: "macroexpand_error",
			Detail: "macroexpand: " + name.Name + " is not a macro"}
	}
	return expandMacroWith(r, &fnDef, toks[1:])
}
