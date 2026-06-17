package native

import (
	"fmt"
	"sort"
	"strings"

	eng "github.com/aql-lang/aql/eng/go"
)

// Patrun — a mutable pattern→value dispatch table, the vendored functionality
// of github.com/rjrodger/patrun (a clean-room port of its documented
// semantics). A pattern is a Map of Scalar match-values; `find` returns the
// value of the MOST SPECIFIC pattern that subset-matches a subject: more
// matched key-value pairs win, and unknown keys in the subject are ignored.
//
//	def routes (patrun)
//	add {a:1}     "A" routes
//	add {a:1 b:1} "B" routes
//	find {a:1}        routes      # → 'A'
//	find {a:1 b:1}    routes      # → 'B'   (specificity wins)
//	find {a:1 z:9}    routes      # → 'A'   (unknown key z ignored)
//	find {x:9}        routes      # → None
//
// Match-values are compared by STRING COERCION (ValToString), faithful to
// patrun/Seneca — so `1` and `"1"` share a rule, while `1.0` ("1.0") and
// `true` ("true") key on their own text. The stored value is any AQL value —
// typically a function, making a Patrun a dispatch/router table.
//
// Tie-break among equally-specific patterns: the lexicographically smaller
// sorted key list wins (patrun's alphabetical traversal order), so
// {a:1 b:1} beats {a:1 c:1}. Two distinct rules can never both fully match the
// same subject at the same key count (their values would have to differ on a
// shared key), so this is total and deterministic.

// TPatrun is Ideal/Patrun. FixedID 5004 — next free in the 5000–9999
// kernel/language band (Module 5000, ModuleExport 5001, KeyVal 5002,
// MiniLangCompiled 5003). See eng/go/CLAUDE.md "FixedID Allocation".
var TPatrun = registerPatrunType()

func registerPatrunType() *eng.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin("Ideal/Patrun", 5004, patrunBehavior{})
	if err != nil {
		// Init-time registration error — recorded, not panicked (ADR-005).
		recordTypeInitErr(fmt.Errorf("native_patrun: register Ideal/Patrun: %w", err))
	}
	return t
}

// patrunEntry is one registered rule: a pattern (alphabetically sorted keys +
// string-coerced values) bound to a stored value. raw keeps the original
// pattern Map for `patterns` listing.
type patrunEntry struct {
	keys []string          // pattern keys, sorted alphabetically
	pat  map[string]string // key -> coerced (ValToString) match-value
	raw  Value             // the original pattern Map
	val  Value             // the stored data (any value)
}

// patrunMatcher is the mutable table behind a Patrun value. add/remove mutate
// entries in place; a Patrun Value wraps the *patrunMatcher pointer (via
// ExtensionPayload), so mutation is visible through every copy — the Store
// mutation model.
type patrunMatcher struct {
	entries []*patrunEntry
}

// NewPatrun builds a fresh empty Patrun value.
func NewPatrun() Value {
	return eng.NewExtension(TPatrun, &patrunMatcher{})
}

func asPatrun(v Value) (*patrunMatcher, bool) {
	ep, ok := v.Data.(eng.ExtensionPayload)
	if !ok {
		return nil, false
	}
	m, ok := ep.Body.(*patrunMatcher)
	return m, ok
}

// patrunNatives installs the Patrun words. `patrun` / `find` / `patterns` are
// new; `add` and `remove` carry a Patrun overload that FOLDS into the existing
// core words (upsertFnDef appends), disambiguated purely by the Patrun
// receiver type — `add 1 2` stays arithmetic.
var patrunNatives = []NativeFunc{
	{
		Name: "patrun",
		Signatures: []NativeSig{
			{Args: []*Type{}, Handler: patrunNewHandler, Returns: []*Type{TPatrun}, BarrierPos: -1},
		},
	},
	{
		Name: "add",
		Signatures: []NativeSig{
			// add PATTERN VALUE PATRUN — register a rule (in place).
			{Args: []*Type{TMap, TAny, TPatrun}, Handler: patrunAddHandler, Returns: []*Type{}, BarrierPos: -1},
		},
	},
	{
		Name: "find",
		Signatures: []NativeSig{
			// find SUBJECT PATRUN {opts} — opts: {exact:Boolean}.
			{Args: []*Type{TMap, TPatrun, TMap}, Handler: patrunFindHandler, Returns: []*Type{TAny}, BarrierPos: -1},
			// find SUBJECT PATRUN
			{Args: []*Type{TMap, TPatrun}, Handler: patrunFindHandler, Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "remove",
		Signatures: []NativeSig{
			// remove PATTERN PATRUN — delete a rule by its pattern (in place).
			{Args: []*Type{TMap, TPatrun}, Handler: patrunRemoveHandler, Returns: []*Type{}, BarrierPos: -1},
		},
	},
	{
		Name: "patterns",
		Signatures: []NativeSig{
			// patterns PATRUN — the registered rules as [{pattern value} …].
			{Args: []*Type{TPatrun}, Handler: patrunPatternsHandler, Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
}

func patrunNewHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewPatrun()}, nil
}

func patrunAddHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, ok := asPatrun(args[2])
	if !ok {
		return nil, r.AqlError("patrun_error", "add: expected a Patrun, got "+args[2].Parent.String(), "add")
	}
	keys, pat, err := coercePattern(args[0], "add", r)
	if err != nil {
		return nil, err
	}
	entry := &patrunEntry{keys: keys, pat: pat, raw: args[0], val: args[1]}
	// Re-adding the identical pattern overwrites its value (set semantics).
	for i, e := range m.entries {
		if samePattern(e, keys, pat) {
			m.entries[i] = entry
			return nil, nil
		}
	}
	m.entries = append(m.entries, entry)
	return nil, nil
}

func patrunFindHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, ok := asPatrun(args[1])
	if !ok {
		return nil, r.AqlError("patrun_error", "find: expected a Patrun, got "+args[1].Parent.String(), "find")
	}
	exact := false
	if len(args) == 3 {
		b, err := patrunOptBool(args[2], "exact")
		if err != nil {
			return nil, r.AqlError("patrun_error", "find: "+err.Error(), "find")
		}
		exact = b
	}
	subj, err := coerceSubject(args[0], "find")
	if err != nil {
		return nil, err
	}
	best := patrunBestMatch(m, subj, exact)
	if best == nil {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	return []Value{best.val}, nil
}

func patrunRemoveHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, ok := asPatrun(args[1])
	if !ok {
		return nil, r.AqlError("patrun_error", "remove: expected a Patrun, got "+args[1].Parent.String(), "remove")
	}
	keys, pat, err := coercePattern(args[0], "remove", r)
	if err != nil {
		return nil, err
	}
	for i, e := range m.entries {
		if samePattern(e, keys, pat) {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			break
		}
	}
	return nil, nil
}

func patrunPatternsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, ok := asPatrun(args[0])
	if !ok {
		return nil, r.AqlError("patrun_error", "patterns: expected a Patrun, got "+args[0].Parent.String(), "patterns")
	}
	out := make([]Value, 0, len(m.entries))
	for _, e := range m.entries {
		row := NewOrderedMap()
		row.Set("pattern", e.raw)
		row.Set("value", e.val)
		out = append(out, NewMap(row))
	}
	return []Value{NewList(out)}, nil
}

// ---- matching ----

// patrunBestMatch returns the most specific subset-matching entry, or nil.
func patrunBestMatch(m *patrunMatcher, subj map[string]string, exact bool) *patrunEntry {
	var best *patrunEntry
	for _, e := range m.entries {
		if !matchesSubset(e, subj) {
			continue
		}
		// Exact: the rule must account for every (scalar) key of the subject,
		// not just a subset — i.e. identical key sets.
		if exact && len(e.keys) != len(subj) {
			continue
		}
		if best == nil || moreSpecific(e, best) {
			best = e
		}
	}
	return best
}

// matchesSubset reports whether every key-value of the rule is present and
// equal (by coerced string) in the subject.
func matchesSubset(e *patrunEntry, subj map[string]string) bool {
	for _, k := range e.keys {
		if sv, ok := subj[k]; !ok || sv != e.pat[k] {
			return false
		}
	}
	return true
}

// moreSpecific reports whether a should beat b: more matched keys wins; on a
// tie the lexicographically smaller sorted key list wins (alphabetical
// traversal order).
func moreSpecific(a, b *patrunEntry) bool {
	if len(a.keys) != len(b.keys) {
		return len(a.keys) > len(b.keys)
	}
	for i := range a.keys {
		if a.keys[i] != b.keys[i] {
			return a.keys[i] < b.keys[i]
		}
	}
	return false
}

func samePattern(e *patrunEntry, keys []string, pat map[string]string) bool {
	if len(e.keys) != len(keys) {
		return false
	}
	for i, k := range e.keys {
		if keys[i] != k || e.pat[k] != pat[k] {
			return false
		}
	}
	return true
}

// ---- coercion ----

// coercePattern coerces a pattern Map to sorted keys + a key→string map. Every
// match-value must be a concrete Scalar; anything else is a loud error (unlike
// patrun-JS, which silently stringifies objects).
func coercePattern(pv Value, op string, r *Registry) ([]string, map[string]string, error) {
	m, err := RequireConcreteMap(pv, op)
	if err != nil {
		return nil, nil, err
	}
	keys := m.Keys()
	pat := make(map[string]string, len(keys))
	for _, k := range keys {
		val, _ := m.Get(k)
		if !IsConcrete(val) || !val.Parent.ConformsTo(TScalar) {
			return nil, nil, r.AqlErrorHint("patrun_error",
				fmt.Sprintf("%s: pattern value for %q must be a Scalar, got %s", op, k, val.Parent.String()),
				op, "patterns match on scalar values (Integer/Float/String/Boolean/Atom)")
		}
		pat[k] = ValToString(val)
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	return sorted, pat, nil
}

// coerceSubject coerces a subject Map to a key→string map, keeping only its
// scalar values (a non-scalar subject value is simply not matchable, so it is
// skipped rather than erroring — only patterns are strict).
func coerceSubject(sv Value, op string) (map[string]string, error) {
	m, err := RequireConcreteMap(sv, op)
	if err != nil {
		return nil, err
	}
	keys := m.Keys()
	subj := make(map[string]string, len(keys))
	for _, k := range keys {
		val, _ := m.Get(k)
		if IsConcrete(val) && val.Parent.ConformsTo(TScalar) {
			subj[k] = ValToString(val)
		}
	}
	return subj, nil
}

func patrunOptBool(opts Value, key string) (bool, error) {
	m, _ := AsMap(opts)
	if m == nil {
		return false, nil
	}
	v, ok := m.Get(key)
	if !ok {
		return false, nil
	}
	b, err := v.AsConcreteBoolean()
	if err != nil {
		return false, fmt.Errorf("opts.%s: %w", key, err)
	}
	return b, nil
}

// ---- behavior ----

// patrunBehavior renders a Patrun as "Patrun(k=v,… -> value; …)"; Match and
// Equal defer to the kernel default (nominal identity).
type patrunBehavior struct{}

func (patrunBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (patrunBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (patrunBehavior) Format(v Value) string {
	m, ok := asPatrun(v)
	if !ok || len(m.entries) == 0 {
		return "Patrun()"
	}
	var b strings.Builder
	b.WriteString("Patrun(")
	for i, e := range m.entries {
		if i > 0 {
			b.WriteString("; ")
		}
		for j, k := range e.keys {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(e.pat[k])
		}
		b.WriteString(" -> ")
		b.WriteString(ValToString(e.val))
	}
	b.WriteByte(')')
	return b.String()
}
