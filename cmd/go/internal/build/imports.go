package build

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// importRe matches `import "<ref>"` / `import '<ref>'`. This is a deliberately
// simple textual scan rather than a full parse: it catches every import at any
// nesting depth, and the only cost of a rare false positive is a clearer error
// when the referenced path does not resolve.
var importRe = regexp.MustCompile(`(?m)\bimport\b\s*["']([^"']+)["']`)

// collectImports walks the file-import graph reachable from src (the contents
// of the file at fileAbs) and adds every transitively-imported .aql file to
// files, keyed by absolute path so the run-time in-memory file system resolves
// it identically to the on-disk loader. Built-in `aql:` imports are skipped —
// they are compiled into the runtime. files is also the visited set, so import
// cycles terminate.
func collectImports(fileAbs string, src []byte, files map[string][]byte) error {
	dir := filepath.Dir(fileAbs)
	for _, m := range importRe.FindAllStringSubmatch(string(src), -1) {
		ref := strings.TrimSpace(m[1])
		if ref == "" || strings.HasPrefix(ref, "aql:") {
			continue // built-in module — already in the runtime
		}

		ext := filepath.Ext(ref)
		if ext != ".aql" && ext != ".lang" {
			return fmt.Errorf(
				"cannot bundle import %q in %s: aql build supports built-in aql: modules and explicit .aql/.lang file imports; "+
					"inline the code or give an explicit file path",
				ref, fileAbs)
		}

		resolved := ref
		if !filepath.IsAbs(ref) {
			resolved = filepath.Join(dir, ref)
		}
		resolved = filepath.Clean(resolved)

		if _, seen := files[resolved]; seen {
			continue
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return fmt.Errorf("bundle import %q (from %s): %w", ref, fileAbs, err)
		}
		files[resolved] = data
		if err := collectImports(resolved, data, files); err != nil {
			return err
		}
	}
	return nil
}
