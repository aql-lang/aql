package eng

import (
	"fmt"
	"strconv"
)

// GetKey extracts the key string from any key-typed value (Word,
// String, Atom, or any other value via Sprintf fallback). Exported
// so lang's accessor handlers (.dotted notation, getr, the
// production set / get handlers themselves) and any host plugin can
// reuse the same key-coercion rules as the kernel's container
// access.
//
// Numeric and Boolean values render via their canonical
// FormatDecimal / FormatInt forms, matching the language's
// printing rules.
func GetKey(v Value) string {
	if IsWord(v) {
		w, _ := AsWord(v)
		return w.Name
	}
	if v.Parent.Matches(TString) {
		s, _ := AsString(v)
		return s
	}
	if IsAtom(v) {
		a, _ := AsAtom(v)
		return a
	}
	if v.Parent.Matches(TInteger) {
		n, _ := AsInteger(v)
		return strconv.FormatInt(n, 10)
	}
	if v.Parent.Matches(TDecimal) {
		f, _ := AsDecimal(v)
		return FormatDecimal(f)
	}
	if v.Parent.Matches(TBoolean) {
		b, _ := AsBoolean(v)
		if b {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%v", v.Data)
}
