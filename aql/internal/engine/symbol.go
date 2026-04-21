package engine

import "sync"

// Symbol is an interned, pointer-comparable representation of a
// word name or other hot string key. Two *Symbol values are equal
// iff their pointers are equal, which reduces map hashing and
// equality cost on the dispatch hot path.
//
// Symbols are created via Intern; direct construction is not
// supported. The zero value is not a valid Symbol.
type Symbol struct {
	Name string
}

var (
	internMu  sync.RWMutex
	internMap = make(map[string]*Symbol, 256)
)

// Intern returns the canonical *Symbol for s. Repeated calls with
// equal strings return the same pointer. Safe for concurrent use.
func Intern(s string) *Symbol {
	internMu.RLock()
	sym, ok := internMap[s]
	internMu.RUnlock()
	if ok {
		return sym
	}
	internMu.Lock()
	defer internMu.Unlock()
	if sym, ok = internMap[s]; ok {
		return sym
	}
	sym = &Symbol{Name: s}
	internMap[s] = sym
	return sym
}

// SymbolOf returns Intern(s) but tolerates nil input by returning
// nil. Useful for code paths that may carry an optional name.
func SymbolOf(s string) *Symbol {
	if s == "" {
		return nil
	}
	return Intern(s)
}

// Pre-interned symbols for hot token comparisons. These are the
// tokens whose equality checks appear in inner engine loops.
var (
	SymOpenParen  = Intern("(")
	SymCloseParen = Intern(")")
	SymEnd        = Intern("end")
	SymTrue       = Intern("true")
	SymFalse      = Intern("false")
)
