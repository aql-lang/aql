// Package help provides embedded help text for AQL built-in words.
package help

import "strings"

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

// Format renders a help entry as a human-readable string.
func Format(e *Entry) string {
	var b strings.Builder

	b.WriteString(e.Word)
	b.WriteString(" — ")
	b.WriteString(e.Summary)
	b.WriteByte('\n')

	b.WriteString("\nSignatures:\n")
	for _, sig := range e.Signatures {
		b.WriteString("  ")
		b.WriteString(sig)
		b.WriteByte('\n')
	}

	b.WriteString("\nDescription:\n  ")
	b.WriteString(e.Description)
	b.WriteByte('\n')

	if len(e.Examples) > 0 {
		b.WriteString("\nExamples:\n")
		for _, ex := range e.Examples {
			b.WriteString("  ")
			b.WriteString(ex)
			b.WriteByte('\n')
		}
	}

	if len(e.Notes) > 0 {
		b.WriteString("\nNotes:\n")
		for _, n := range e.Notes {
			b.WriteString("  - ")
			b.WriteString(n)
			b.WriteByte('\n')
		}
	}

	return b.String()
}
