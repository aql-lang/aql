// Package help provides embedded help text for AQL built-in words.
package help

// Entry holds the help documentation for a single AQL word.
type Entry struct {
	Word        string   // canonical word name
	Summary     string   // one-line description
	Signatures  []string // type signatures, e.g. "[string string] -> [string]"
	Description string   // brief multi-line explanation
	Examples    []string // usage examples
	Notes       []string // warnings, gotchas, unexpected behaviour
}

// registry holds all help entries keyed by word name.
var registry = map[string]*Entry{}

// register adds an entry to the global help registry.
func register(e *Entry) {
	registry[e.Word] = e
}

// Lookup returns the help entry for a word, or nil if none exists.
func Lookup(word string) *Entry {
	return registry[word]
}

// Words returns all registered word names in no particular order.
func Words() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}
