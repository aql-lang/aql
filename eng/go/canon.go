package eng

import (
	"strconv"
	"strings"
)

// Canon renders a stack of values as canonical AQL source — a string
// that, when parsed and evaluated, reproduces the input stack. Where it
// diverges from Value.String:
//
//   - atoms render as `name/q` (bare `name` would parse as a word
//     lookup, not as an atom value; the /q suffix is the preferred
//     short form over `(quote name)`)
//   - quoted lists render as `(quote [...])` so the Quoted flag survives
//     a round-trip (the /q suffix is only defined for words)
//   - lists and maps are space-separated, matching AQL source syntax
//     instead of Value.String's comma-separated debug form
//
// Values without a known canonical form (runtime markers, errors,
// foreign types) fall back to Value.String.
func Canon(stack []Value) string {
	parts := make([]string, len(stack))
	for i, v := range stack {
		parts[i] = CanonValue(v)
	}
	return strings.Join(parts, " ")
}

// CanonValue renders one value as canonical AQL source. See Canon.
func CanonValue(v Value) string {
	// Behavior-driven dispatch for user-defined types: if a non-
	// builtin type in v.Parent's parent chain has a non-default
	// Behavior, route through it. This is how user-installed canon
	// bodies (`behave canon/q (fn [[T] [String] [body]])`) flow
	// into eng.Canon.
	//
	// Built-in Behaviors (listFormatBehavior, mapFormatBehavior,
	// dateFormatBehavior, …) are deliberately skipped here — they
	// produce Value.String's debug form (comma-separated lists,
	// time-domain renderings) which doesn't match Canon's source-
	// shape conventions (space-separated lists, quoted strings).
	// CanonValue's own switch below preserves those.
	if v.Data != nil && v.Parent != nil {
		for t := v.Parent; t != nil; t = t.Parent {
			if t.Origin == OriginBuiltin {
				continue
			}
			if t.Behavior == nil || t.Behavior == DefaultBehavior {
				continue
			}
			if _, ok := t.Behavior.(formatDelegatesToDefault); ok {
				continue
			}
			return t.Behavior.Format(v)
		}
	}
	switch {
	case IsNone(v):
		return "none"
	case v.Data == nil:
		if v.Parent != nil {
			if name := TypeNameByID(v.Parent.ID); name != "" {
				return name
			}
			return v.Parent.Leaf()
		}
		return "none"
	case v.IsDepScalar():
		return v.String()
	case v.Parent.Matches(TInteger):
		n, _ := AsInteger(v)
		return strconv.FormatInt(n, 10)
	case v.Parent.Matches(TDecimal):
		f, _ := AsDecimal(v)
		return FormatDecimal(f)
	case v.Parent.Matches(TString):
		s, _ := AsString(v)
		return "'" + s + "'"
	case v.Parent.Matches(TBoolean):
		b, _ := AsBoolean(v)
		if b {
			return "true"
		}
		return "false"
	case v.Parent.Equal(TAtom):
		s, _ := AsAtom(v)
		return s + "/q"
	case v.Parent.Matches(TList) && v.Data != nil:
		lst, _ := AsList(v)
		parts := make([]string, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			parts[i] = CanonValue(lst.Get(i))
		}
		body := "[" + strings.Join(parts, " ") + "]"
		if v.Quoted {
			return "(quote " + body + ")"
		}
		return body
	case v.Parent.Equal(TMap) && v.Data != nil:
		m, err := AsMap(v)
		if err != nil || m == nil {
			return v.String()
		}
		parts := make([]string, m.Len())
		for i, k := range m.Keys() {
			val, _ := m.Get(k)
			parts[i] = k + ":" + CanonValue(val)
		}
		return "{" + strings.Join(parts, " ") + "}"
	default:
		return v.String()
	}
}
