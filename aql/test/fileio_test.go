package test

import (
	"strings"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// runWithFiles creates a registry with in-memory files and runs AQL.
func runWithFiles(t *testing.T, files map[string]string, expr string) (string, error) {
	t.Helper()
	mem := fileops.NewMem()
	for path, content := range files {
		mem.Files[path] = []byte(content)
	}

	reg := engine.DefaultRegistry()
	reg.SetFileOps(mem)

	values, err := parser.Parse(expr)
	if err != nil {
		return "", err
	}

	eng := engine.New(reg)
	result, err := eng.Run(values)
	if err != nil {
		return "", err
	}

	return formatStack(result), nil
}

// runWithMem creates a registry with an in-memory FS, runs AQL, and returns
// the MemFileOps so tests can inspect written files.
func runWithMem(t *testing.T, files map[string]string, expr string) (*fileops.MemFileOps, string, error) {
	t.Helper()
	mem := fileops.NewMem()
	for path, content := range files {
		mem.Files[path] = []byte(content)
	}

	reg := engine.DefaultRegistry()
	reg.SetFileOps(mem)

	values, err := parser.Parse(expr)
	if err != nil {
		return mem, "", err
	}

	eng := engine.New(reg)
	result, err := eng.Run(values)
	if err != nil {
		return mem, "", err
	}

	return mem, formatStack(result), nil
}

// --- read tests ---

func TestReadBasic(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"hello.txt": "hello world",
	}, `read "hello.txt"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'hello world'" {
		t.Errorf("got %s, want 'hello world'", got)
	}
}

func TestReadPrefix(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"data.txt": "abc",
	}, `"data.txt" read`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'abc'" {
		t.Errorf("got %s, want 'abc'", got)
	}
}

func TestReadMissingFile(t *testing.T) {
	_, err := runWithFiles(t, nil, `read "nope.txt"`)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not exist") {
		t.Errorf("expected 'not exist' in error, got: %v", err)
	}
}

// --- line ending normalization ---

func TestReadNormalizesLineEndings(t *testing.T) {
	// Default nl:"lf" should convert \r\n to \n.
	got, err := runWithFiles(t, map[string]string{
		"crlf.txt": "line1\r\nline2\r\n",
	}, `read "crlf.txt"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'line1\nline2\n'" {
		t.Errorf("got %q, want 'line1\\nline2\\n'", got)
	}
}

func TestReadNormalizesCR(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"cr.txt": "a\rb\r",
	}, `read "cr.txt"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'a\nb\n'" {
		t.Errorf("got %q, want 'a\\nb\\n'", got)
	}
}

func TestReadRawLineEndings(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"raw.txt": "a\r\nb\r",
	}, `read "raw.txt" {nl:"raw"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'a\r\nb\r'" {
		t.Errorf("got %q, want 'a\\r\\nb\\r'", got)
	}
}

// --- fmt:"lines" ---

func TestReadLines(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"lines.txt": "aaa\nbbb\nccc",
	}, `read "lines.txt" {fmt:"lines"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "['aaa','bbb','ccc']" {
		t.Errorf("got %s, want ['aaa','bbb','ccc']", got)
	}
}

// --- fmt:"json" ---

func TestReadJSON(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"data.json": `{"x":1,"y":"hello"}`,
	}, `read "data.json" {fmt:"json"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "{x:1,y:'hello'}" {
		t.Errorf("got %s, want {x:1,y:'hello'}", got)
	}
}

func TestReadJSONArray(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"arr.json": `[1,2,3]`,
	}, `read "arr.json" {fmt:"json"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[1,2,3]" {
		t.Errorf("got %s, want [1,2,3]", got)
	}
}

// --- fmt:"jsonic" ---

func TestReadJsonic(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"config.jsonic": `{x:1, y:hello}`,
	}, `read "config.jsonic" {fmt:"jsonic"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "{x:1,y:'hello'}" {
		t.Errorf("got %s, want {x:1,y:'hello'}", got)
	}
}

// --- write tests ---

func TestWriteBasic(t *testing.T) {
	mem, got, err := runWithMem(t, nil, `write "out.txt" "hello"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'out.txt'" {
		t.Errorf("return: got %s, want 'out.txt'", got)
	}
	content := string(mem.Files["out.txt"])
	if content != "hello" {
		t.Errorf("file content: got %q, want %q", content, "hello")
	}
}

func TestWriteSuffix(t *testing.T) {
	// write path content — both suffix
	mem, got, err := runWithMem(t, nil, `write "out.txt" "hello"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'out.txt'" {
		t.Errorf("return: got %s, want 'out.txt'", got)
	}
	content := string(mem.Files["out.txt"])
	if content != "hello" {
		t.Errorf("file content: got %q, want %q", content, "hello")
	}
}

func TestWriteWithExprContent(t *testing.T) {
	// write path (expression) — content from paren expression
	mem, got, err := runWithMem(t, nil, `write "out.txt" (upper "hello")`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'out.txt'" {
		t.Errorf("return: got %s, want 'out.txt'", got)
	}
	content := string(mem.Files["out.txt"])
	if content != "HELLO" {
		t.Errorf("file content: got %q, want %q", content, "HELLO")
	}
}

func TestWriteAppend(t *testing.T) {
	mem, _, err := runWithMem(t, map[string]string{
		"log.txt": "line1\n",
	}, `write "log.txt" "line2\n" {mode:"append"}`)
	if err != nil {
		t.Fatal(err)
	}
	content := string(mem.Files["log.txt"])
	if content != "line1\nline2\n" {
		t.Errorf("file content: got %q, want %q", content, "line1\nline2\n")
	}
}

func TestWriteReturnsPath(t *testing.T) {
	_, got, err := runWithMem(t, nil, `write "result.txt" "data"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'result.txt'" {
		t.Errorf("got %s, want 'result.txt'", got)
	}
}

// --- read/write roundtrip ---

func TestReadWriteRoundtrip(t *testing.T) {
	mem := fileops.NewMem()
	mem.Files["src.txt"] = []byte("the content")

	reg := engine.DefaultRegistry()
	reg.SetFileOps(mem)

	// Write with all suffix args to be explicit
	values, err := parser.Parse(`write "dst.txt" (read "src.txt")`)
	if err != nil {
		t.Fatal(err)
	}

	eng := engine.New(reg)
	result, err := eng.Run(values)
	if err != nil {
		t.Fatal(err)
	}

	got := formatStack(result)
	if got != "'dst.txt'" {
		t.Errorf("return: got %s, want 'dst.txt'", got)
	}

	content := string(mem.Files["dst.txt"])
	if content != "the content" {
		t.Errorf("file content: got %q, want %q", content, "the content")
	}
}

// --- write with crlf ---

func TestWriteCRLF(t *testing.T) {
	mem, _, err := runWithMem(t, nil, `write "out.txt" "a\nb\n" {nl:"crlf"}`)
	if err != nil {
		t.Fatal(err)
	}
	content := string(mem.Files["out.txt"])
	if content != "a\r\nb\r\n" {
		t.Errorf("file content: got %q, want %q", content, "a\r\nb\r\n")
	}
}
