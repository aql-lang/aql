// Completion: registered words → []CompletionItem.

package lsp

import (
	helppkg "github.com/aql-lang/aql/lang/go/native/help"
)

// buildCompletionItems returns one CompletionItem per registered
// word. We surface every word in the help registry; refining the
// list to "valid completions at the cursor" would require parsing
// context we don't have, so the client-side fuzzy filter handles
// it.
func (s *server) buildCompletionItems() []CompletionItem {
	words := helppkg.Words()
	items := make([]CompletionItem, 0, len(words))
	for _, w := range words {
		entry := helppkg.Lookup(w)
		ci := CompletionItem{
			Label:      w,
			Kind:       completionKindFunction,
			InsertText: w,
		}
		if entry != nil {
			ci.Detail = entry.Summary
		}
		items = append(items, ci)
	}
	return items
}
