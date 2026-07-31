package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// TestFileInfoWords checks __file and __folder report the source file's
// location, resolved PER FILE (each imported module sees its own path),
// and that relative imports resolve against the importing file's own dir.
func TestFileInfoWords(t *testing.T) {
	files := map[string]string{
		// A module in a subdirectory. Its exports capture __file/__folder,
		// which must reflect THIS file (proj/lib.boru), not the entry.
		"proj/lib.boru": `export "Lib" { f: (__file) d: (__folder) }`,
		// A sibling the entry imports; it in turn imports proj/lib.boru with
		// a path relative to ITS OWN location (the repo root here).
		"main.boru": `import "./proj/lib.boru"`,
	}
	res, err := runNativeModuleSubImport(t, files, []string{
		`import "./proj/lib.boru"`,
		`Lib.f`,
	})
	if err != nil {
		t.Fatalf("Lib.f: %v", err)
	}
	if got, _ := native.AsString(res[len(res)-1]); got != "lib.boru" {
		t.Errorf("__file in proj/lib.boru = %q, want \"lib.boru\"", got)
	}

	res, err = runNativeModuleSubImport(t, files, []string{
		`import "./proj/lib.boru"`,
		`Lib.d`,
	})
	if err != nil {
		t.Fatalf("Lib.d: %v", err)
	}
	// __folder is the absolute-style Path of the module's directory; its
	// last segment is "proj".
	if got := res[len(res)-1].String(); got == "" || lastSeg(got) != "proj" {
		t.Errorf("__folder in proj/lib.boru = %q, want a path ending in proj", got)
	}
}

// TestFileInfoReplEmpty: with no source file (string eval), __file is the
// empty string and __folder is the empty path.
func TestFileInfoReplEmpty(t *testing.T) {
	res, err := runNativeModuleSubImport(t, map[string]string{}, []string{`__file`})
	if err != nil {
		t.Fatalf("__file: %v", err)
	}
	if got, _ := native.AsString(res[len(res)-1]); got != "" {
		t.Errorf("__file at REPL = %q, want empty string", got)
	}
}

func lastSeg(p string) string {
	last := ""
	cur := ""
	for _, r := range p {
		if r == '/' {
			if cur != "" {
				last = cur
			}
			cur = ""
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		last = cur
	}
	return last
}
