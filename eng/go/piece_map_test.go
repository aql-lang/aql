package eng

// TestPieceMap is the four-piece split's Stage-0 "virtual package" lint
// (design/ENG-FOUR-PIECE.0.md): every production file is assigned to a
// piece (core / check / compiler / eng), and files assigned to core
// must not reach UP the target dependency chain
// (eng -> compiler -> check -> core) through the known entanglement
// surfaces. The reference lists below shrink to empty as the seam
// stages land; a NEW wrong-direction reference in an unlisted file
// fails immediately, so the split only ratchets forward.

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readPieceMap(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open("piece_map.tsv")
	if err != nil {
		t.Fatalf("piece_map.tsv: %v", err)
	}
	defer f.Close()
	m := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			t.Fatalf("piece_map.tsv: bad line %q", line)
		}
		m[parts[0]] = parts[1]
	}
	return m
}

// upwardRefs are the entanglement probes per pattern family; a core
// file matching one is a wrong-direction reference UNLESS listed in
// the per-family accommodation set (the recon's F1/F2 inventory —
// these sets only shrink).
var upwardRefs = []struct {
	name    string
	re      *regexp.Regexp
	allowed map[string]bool
}{
	{
		name: "check-state consultation (F1)",
		re:   regexp.MustCompile(`\.Check\.`),
		allowed: map[string]bool{
			// The recon-inventoried F1 sites, burned down by Stage 2.
			"engine.go": true, "core_helpers.go": true, "method_shape.go": true,
			"registry.go": true, "core_make.go": true, "core_ref.go": true,
			"word_extend.go": true, "interp_entry.go": true, "core_type.go": true,
			"util.go": true, "fn_params.go": true, "invoke.go": true,
			"depscalar.go": true, "effects.go": true, "policy_hook.go": true,
			"module_export_growth.go": true, "engine_pool.go": true,
			"core_boolean.go": true, "typed_bind.go": true, "value.go": true,
			"macro_expand.go": true, "define_type.go": true, "fn_def.go": true,
			"coverage.go": true, "equal.go": true, "compare.go": true,
		},
	},
	{
		name: "emit recorder reach (F2)",
		re:   regexp.MustCompile(`Recorder\(\)`),
		allowed: map[string]bool{
			"engine.go": true, "core_helpers.go": true, "method_shape.go": true,
			"registry.go": true, "depscalar.go": true, "interp_entry.go": true,
			"macro_expand.go": true, "fn_def.go": true, "invoke.go": true,
			"word_extend.go": true, "value.go": true, "engine_pool.go": true,
		},
	},
	{
		name: "concrete EmitState assert (F2-hard)",
		re:   regexp.MustCompile(`\.\(\*EmitState\)`),
		allowed: map[string]bool{
			"engine.go": true, "core_helpers.go": true, "method_shape.go": true,
		},
	},
}

func TestPieceMap(t *testing.T) {
	pieces := readPieceMap(t)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		piece, ok := pieces[f]
		if !ok {
			t.Errorf("%s: not assigned in piece_map.tsv — assign it to core/check/compiler/eng", f)
			continue
		}
		if piece != "core" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, probe := range upwardRefs {
			if probe.re.Match(src) && !probe.allowed[f] {
				t.Errorf("%s (core): new wrong-direction reference [%s] — route it through a seam (design/ENG-FOUR-PIECE.0.md)", f, probe.name)
			}
		}
	}
}
