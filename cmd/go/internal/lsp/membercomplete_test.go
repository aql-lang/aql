package lsp

import (
	"io"
	"strings"
	"testing"
)

func labelsOf(items []CompletionItem) map[string]int {
	m := map[string]int{}
	for _, it := range items {
		m[it.Label] = it.Kind
	}
	return m
}

// TestMemberCompletion drives completionItemsAt across every receiver shape:
// Micron properties (expression, binding, all leaves), module exports, class
// fields, record + map keys (v2), a chain to a memberless type, and the global
// fallback when not after a dot.
func TestMemberCompletion(t *testing.T) {
	s := newServer(strings.NewReader(""), io.Discard, io.Discard)

	cases := []struct {
		name     string
		src      string
		line, ch int
		want     []string // labels that must be present
		absent   []string // labels that must NOT be present
		exact    int      // if >0, require exactly this many items
	}{
		{
			name: "micron expression (group)",
			src:  `(make Semveron "1.2.3").` + "\n",
			line: 0, ch: 24,
			want: []string{"major", "minor", "patch", "prerelease", "release", "stable", "version"},
		},
		{
			name: "micron binding",
			src:  "def e (make Emailon 'a@b.co')\ne.\n",
			line: 1, ch: 2,
			want: []string{"user", "host", "address"}, exact: 3,
		},
		{
			name: "module namespace",
			src:  "import \"boru:rand\"\nRand.\n",
			line: 1, ch: 5,
			want: []string{"int", "float", "bool", "one-of", "with-seed"},
		},
		{
			name: "class instance",
			src:  "def P class {x:1 y:2}\ndef p (make P {})\np.\n",
			line: 2, ch: 2,
			want: []string{"x", "y"}, exact: 2,
		},
		{
			name: "record instance via make literal (v2)",
			src:  "def R refine Record [a:Integer b:String]\ndef x (make R {a:1 b:\"z\"})\nx.\n",
			line: 2, ch: 2,
			want: []string{"a", "b"}, exact: 2,
		},
		{
			name: "anonymous map keys (v2)",
			src:  "def m {a:1 b:2 c:3}\nm.\n",
			line: 1, ch: 2,
			want: []string{"a", "b", "c"}, exact: 3,
		},
		{
			name: "chain into a memberless String",
			src:  "def u (make Urlon 'https://x.io/p')\nu.host.\n",
			line: 1, ch: 7,
			exact: 0, // no members, and never the global list
		},
		{
			name: "no dot -> global word list",
			src:  "ad\n",
			line: 0, ch: 2,
			want:   []string{"add", "abs"},
			absent: []string{"major", "user"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			items := s.completionItemsAt(c.src, Position{Line: c.line, Character: c.ch})
			got := labelsOf(items)
			for _, w := range c.want {
				if _, ok := got[w]; !ok {
					t.Errorf("missing %q; got %d items: %v", w, len(items), keysOf(got))
				}
			}
			for _, a := range c.absent {
				if _, ok := got[a]; ok {
					t.Errorf("unexpected %q present", a)
				}
			}
			if c.name == "chain into a memberless String" && len(items) != 0 {
				t.Errorf("memberless receiver returned %d items (want 0): %v", len(items), keysOf(got))
			}
			if c.exact > 0 && len(items) != c.exact {
				t.Errorf("got %d items, want exactly %d: %v", len(items), c.exact, keysOf(got))
			}
		})
	}
}

func keysOf(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestMemberCompletionAllMicronLeaves checks a representative property on each
// of the twelve builtin Micron leaves resolves through the completion path.
func TestMemberCompletionAllMicronLeaves(t *testing.T) {
	s := newServer(strings.NewReader(""), io.Discard, io.Discard)
	cases := map[string]struct{ ctor, prop string }{
		"Pathon":   {"make Pathon '/a/b'", "parts"},
		"Emailon":  {"make Emailon 'a@b.co'", "address"},
		"Urlon":    {"make Urlon 'https://x.io/p'", "href"},
		"Ipon":     {"make Ipon '10.0.0.1'", "version"},
		"Hoston":   {"make Hoston 'x.io:80'", "authority"},
		"Semveron": {"make Semveron '1.2.3'", "release"},
		"Cidron":   {"make Cidron '10.0.0.0/8'", "hostbits"},
		"Macon":    {"make Macon '00:1a:2b:3c:4d:5e'", "oui"},
		"Coloron":  {"make Coloron '#ff0000'", "hex"},
		"Mimon":    {"make Mimon 'text/html'", "essence"},
		"Qion":     {"make Qion 'USD 1.00'", "display"},
		"Phonon":   {"make Phonon '+14155552671'", "e164"},
	}
	for leaf, c := range cases {
		src := "(" + c.ctor + ").\n"
		items := s.completionItemsAt(src, Position{Line: 0, Character: len(c.ctor) + 3})
		if _, ok := labelsOf(items)[c.prop]; !ok {
			t.Errorf("%s: property %q not completed; got %v", leaf, c.prop, keysOf(labelsOf(items)))
		}
	}
}

// TestModuleNamespaceIrregular verifies namespaces whose bound name is NOT a
// plain title-case of the id (boru:io → IO) resolve, since we match against the
// module's real Exports keys rather than guessing.
func TestModuleNamespaceIrregular(t *testing.T) {
	s := newServer(strings.NewReader(""), io.Discard, io.Discard)
	items := s.completionItemsAt("import \"boru:io\"\nIO.\n", Position{Line: 1, Character: 3})
	if len(items) == 0 {
		t.Fatal("IO. produced no module-export completions")
	}
	got := labelsOf(items)
	if _, ok := got["printstr"]; !ok {
		t.Errorf("IO. missing 'printstr'; got %v", keysOf(got))
	}
	if k := items[0].Kind; k != completionKindMethod {
		t.Errorf("module export kind = %d, want %d (method)", k, completionKindMethod)
	}
}

func TestTopLevelKeys(t *testing.T) {
	// Nested maps and string values must not leak keys or terminate early.
	keys := topLevelKeys(`{a:1 b:{x:9 y:8} c:'has}brace' d:2}`)
	want := []string{"a", "b", "c", "d"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("key[%d] = %q, want %q", i, keys[i], want[i])
		}
	}
}

func TestReceiverBeforeNonMember(t *testing.T) {
	// A cursor not after a dot is not a member context.
	if _, ok := receiverBefore("add 1 2", Position{Line: 0, Character: 3}); ok {
		t.Error("word without a dot should not be a member context")
	}
	// After a dot with an identifier receiver -> member context.
	if r, ok := receiverBefore("Rand.", Position{Line: 0, Character: 5}); !ok || r.word != "Rand" || !r.simple {
		t.Errorf("Rand. = %+v, ok=%v", r, ok)
	}
}
