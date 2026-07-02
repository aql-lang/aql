package eng

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The severity table (checkCodeSeverity, registry.go) is the SINGLE source
// of truth for diagnostic severities: AddDiagnostic fills an empty Severity
// from it, and SeverityFor answers external consumers. A code emitted
// anywhere without a table entry silently defaults to Info for SeverityFor
// while the emit site may set something else inline — the two-sources-of-
// truth drift this gate closes. Every `Code:` in a CheckDiagnostic literal
// (eng/go + lang/go/native) must have a table entry; per-site Severity
// OVERRIDES remain legal (e.g. the fn-return-only unbound_param demotion to
// Warning), but the code itself must be classified.
func TestCheckSeverityTableComplete(t *testing.T) {
	codeRe := regexp.MustCompile(`Code:\s*"([a-z_]+)"`)
	litRe := regexp.MustCompile(`CheckDiagnostic\{`)
	var missing []string
	seen := map[string]bool{}
	scan := func(root string) {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src := string(data)
			for _, m := range litRe.FindAllStringIndex(src, -1) {
				// Scan the literal's next few lines for the Code field.
				window := src[m[1]:]
				if end := strings.Index(window, "}"); end >= 0 {
					window = window[:end]
				}
				if cm := codeRe.FindStringSubmatch(window); cm != nil {
					code := cm[1]
					if seen[code] {
						continue
					}
					seen[code] = true
					if _, ok := checkCodeSeverity[code]; !ok {
						missing = append(missing, code+" ("+path+")")
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	scan(".")
	scan(filepath.Join("..", "..", "lang", "go", "native"))
	if len(missing) > 0 {
		t.Errorf("diagnostic codes emitted without a checkCodeSeverity entry (add them to the table in registry.go):\n  %s",
			strings.Join(missing, "\n  "))
	}
}
