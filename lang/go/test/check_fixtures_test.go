package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// TestCheckGoldenFixtures runs every *.boru file under
// test/check_fixtures through lang.Boru.Check and compares the
// marshalled CheckResult against the sibling *.golden.json file.
// Set BORU_UPDATE_GOLDEN=1 to overwrite the golden files with the
// current output (use when the diagnostic stream has intentionally
// changed). Otherwise mismatches fail the test.
func TestCheckGoldenFixtures(t *testing.T) {
	dir := "check_fixtures"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	update := os.Getenv("BORU_UPDATE_GOLDEN") == "1"
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".boru") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".boru")
		boruPath := filepath.Join(dir, e.Name())
		goldenPath := filepath.Join(dir, name+".golden.json")

		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(boruPath)
			if err != nil {
				t.Fatalf("read %s: %v", boruPath, err)
			}
			a, err := lang.New()
			if err != nil {
				t.Fatalf("new: %v", err)
			}
			seedBoru(a)
			res, err := a.Check(string(src))
			if err != nil {
				t.Fatalf("check %s: %v", boruPath, err)
			}
			// Avoid nil-vs-empty JSON differences: normalise.
			if res.Diagnostics == nil {
				res.Diagnostics = []lang.CheckDiagnostic{}
			}
			if res.Stack == nil {
				res.Stack = []string{}
			}

			got, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			if update {
				if err := os.WriteFile(goldenPath, append(got, '\n'), 0644); err != nil {
					t.Fatalf("write golden %s: %v", goldenPath, err)
				}
				t.Logf("updated %s", goldenPath)
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with BORU_UPDATE_GOLDEN=1 to create)", goldenPath, err)
			}
			if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
				t.Errorf("golden mismatch for %s\nGOT:\n%s\nWANT:\n%s",
					boruPath, got, want)
			}
		})
	}
}
