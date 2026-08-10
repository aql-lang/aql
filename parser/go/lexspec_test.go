package parser_test

// The Go half of parser/spec/lex.tsv. The TypeScript runner has its own
// reader and renderer: keeping both implementations independent prevents a
// shared harness bug from making two different token streams look equal.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/parser/go"
)

const lexSpecRowCount = 30

type lexSpecRow struct {
	line     int
	src      string
	status   string
	expected string
}

type lexSpecToken struct {
	Name string `json:"name"`
	Src  string `json:"src"`
	SI   int    `json:"si"`
}

func readLexSpec(t *testing.T) []lexSpecRow {
	t.Helper()
	const name = "lex.tsv"
	path := filepath.Join("..", "spec", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	seen := make(map[string]int)
	var rows []lexSpecRow
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || (strings.HasPrefix(line, "#") && !strings.Contains(line, "\t")) {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("%s:%d: need exactly 3 tab-separated columns (source, status, tokens), got %d", name, n, len(parts))
		}
		src := decodeSpecEscapes(parts[0])
		if first, exists := seen[src]; exists {
			t.Fatalf("%s:%d: duplicate source (first at line %d): %q", name, n, first, src)
		}
		seen[src] = n
		status := decodeSpecEscapes(parts[1])
		if status != "OK" && status != "ERR" {
			t.Fatalf("%s:%d: status %q is not OK or ERR", name, n, status)
		}
		expected := decodeSpecEscapes(parts[2])
		validateLexSpecTokens(t, name, n, expected)
		rows = append(rows, lexSpecRow{line: n, src: src, status: status, expected: expected})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	if len(rows) != lexSpecRowCount {
		t.Fatalf("%s: got %d data rows, want exact ratchet %d", path, len(rows), lexSpecRowCount)
	}
	return rows
}

func validateLexSpecTokens(t *testing.T, name string, line int, expected string) {
	t.Helper()
	var tokens []lexSpecToken
	dec := json.NewDecoder(strings.NewReader(expected))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tokens); err != nil {
		t.Fatalf("%s:%d: tokens are not a canonical token array: %v", name, line, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Fatalf("%s:%d: trailing data after token array", name, line)
	}
	for i, token := range tokens {
		if !strings.HasPrefix(token.Name, "#") || token.SI < 0 {
			t.Fatalf("%s:%d: invalid token %d: %+v", name, line, i, token)
		}
	}
	canonical, err := marshalLexSpecTokens(tokens)
	if err != nil {
		t.Fatalf("%s:%d: marshal expected tokens: %v", name, line, err)
	}
	if canonical != expected {
		t.Fatalf("%s:%d: token JSON is not canonical; want %s", name, line, canonical)
	}
}

// encoding/json's package helper HTML-escapes `>` while JSON.stringify does
// not. Disable that optional transport hardening so both independent token
// renderers use the same canonical source spelling (notably for `=>`).
func marshalLexSpecTokens(tokens []lexSpecToken) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(tokens); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func renderLexSpec(tokens []parser.LexToken) string {
	out := make([]lexSpecToken, len(tokens))
	for i, token := range tokens {
		out[i] = lexSpecToken{Name: token.Name, Src: token.Src, SI: token.SI}
	}
	rendered, err := marshalLexSpecTokens(out)
	if err != nil {
		panic(fmt.Sprintf("render lex tokens: %v", err))
	}
	return rendered
}

func TestParserSpecLex(t *testing.T) {
	for _, row := range readLexSpec(t) {
		row := row
		t.Run(fmt.Sprintf("line-%d", row.line), func(t *testing.T) {
			tokens, ok := parser.LexTokens(row.src)
			status := "ERR"
			if ok {
				status = "OK"
			}
			if status != row.status {
				t.Errorf("lex.tsv:%d: %q status: want %s, got %s", row.line, row.src, row.status, status)
			}
			if got := renderLexSpec(tokens); got != row.expected {
				t.Errorf("lex.tsv:%d: %q tokens:\n  want: %s\n  got : %s", row.line, row.src, row.expected, got)
			}
			// Every returned raw token must begin at its advertised UTF-8 byte
			// offset. This assertion is independent of the checked-in render.
			for _, token := range tokens {
				if token.SI < 0 || token.SI > len(row.src) ||
					!bytes.HasPrefix([]byte(row.src)[token.SI:], []byte(token.Src)) {
					t.Errorf("lex.tsv:%d: token %+v does not index source bytes", row.line, token)
				}
			}
		})
	}
}
