package tree_sitter_boru_test

import (
	"testing"

	tree_sitter "github.com/smacker/go-tree-sitter"
	"github.com/tree-sitter/tree-sitter-boru"
)

func TestCanLoadGrammar(t *testing.T) {
	language := tree_sitter.NewLanguage(tree_sitter_boru.Language())
	if language == nil {
		t.Errorf("Error loading boru grammar")
	}
}
