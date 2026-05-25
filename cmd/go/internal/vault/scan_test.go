package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test fixtures for matched secret strings are constructed at
// runtime by concatenation so the literal token shape never
// appears in source. This prevents both GitHub Secret Scanning
// push protection from rejecting the file and accidental real
// leaks from copy-paste of these constants.
var (
	fakeOpenAI    = "sk-" + "proj-" + strings.Repeat("a", 40) + "AB"
	fakeAnthropic = "sk-" + "ant-" + strings.Repeat("a", 40) + "AB"
	fakeGitHub    = "gh" + "p_" + strings.Repeat("a", 36) + "AA"
	fakeGitHubPAT = "github_" + "pat_" + strings.Repeat("A", 70)
	fakeAWS       = "AK" + "IA" + strings.Repeat("A", 16)
	fakeGoogle    = "AI" + "za" + strings.Repeat("A", 35)
	fakeSlack     = "xo" + "xb-1234567890-1234567890-" + strings.Repeat("a", 25)
	fakeStripe    = "sk_" + "live_" + strings.Repeat("A", 24)
	fakeJWT       = "eyJ" + strings.Repeat("a", 10) + "." + "eyJ" + strings.Repeat("a", 10) + "." + strings.Repeat("a", 12)
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestScanDetectsKnownPatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"),
		"OPENAI_API_KEY="+fakeOpenAI+"\n"+
			"ANTHROPIC_API_KEY="+fakeAnthropic+"\n"+
			"GITHUB_TOKEN="+fakeGitHub+"\n"+
			"AWS_ACCESS_KEY_ID="+fakeAWS+"\n")

	testHome(t)
	code, out, _ := runVault(t, "", "scan", dir)
	if code != 2 {
		t.Fatalf("expected exit code 2 (findings), got %d", code)
	}
	for _, want := range []string{"openai-api-key", "anthropic-api-key", "github-token"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output: %q", want, out)
		}
	}
	if strings.Contains(out, fakeAnthropic) {
		t.Errorf("scan output leaked full secret: %q", out)
	}
}

func TestScanCleanRepoExitsZero(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	writeFile(t, filepath.Join(dir, "README.md"), "# Title\nNothing secret here.\n")

	testHome(t)
	code, out, _ := runVault(t, "", "scan", dir)
	if code != 0 {
		t.Fatalf("expected exit 0 on clean repo, got %d (out=%q)", code, out)
	}
	if !strings.Contains(out, "no findings") {
		t.Errorf("expected 'no findings', got %q", out)
	}
}

func TestScanSkipsExampleLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env.example"),
		"OPENAI_API_KEY="+fakeOpenAI+" # <your real key>\n"+
			"# Replace ... with your real key\n"+
			"GITHUB_TOKEN="+fakeGitHub+" # placeholder\n")
	testHome(t)
	code, _, _ := runVault(t, "", "scan", dir)
	if code != 0 {
		t.Errorf("expected example placeholders to be skipped, got exit %d", code)
	}
}

func TestScanSkipsConventionalDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "node_modules", "leak.txt"), fakeOpenAI+"\n")
	writeFile(t, filepath.Join(dir, "src", "ok.go"), "package src\n")
	testHome(t)
	code, _, _ := runVault(t, "", "scan", dir)
	if code != 0 {
		t.Errorf("expected node_modules to be pruned (exit 0), got %d", code)
	}
}

func TestScanQuietSuppressesOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "OPENAI_API_KEY="+fakeOpenAI+"\n")
	testHome(t)
	code, out, _ := runVault(t, "", "scan", "--quiet", dir)
	if code != 2 {
		t.Errorf("expected exit 2 with findings, got %d", code)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("--quiet should produce no stdout, got %q", out)
	}
}

func TestScanMatchVaultCrossReferences(t *testing.T) {
	testHome(t)
	mustInit(t)

	if code, _, errOut := runVault(t, fakeOpenAI+"\n", "add",
		"--from-stdin", "--provider=openai", "myalias"); code != 0 {
		t.Fatalf("add: %s", errOut)
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "OPENAI_API_KEY="+fakeOpenAI+"\n")

	code, out, _ := runVault(t, "", "scan", "--match-vault", dir)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
	if !strings.Contains(out, "matches alias myalias") {
		t.Errorf("missing alias cross-reference in %q", out)
	}
}

func TestScanRespectsMaxBytes(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", 10) + fakeOpenAI
	writeFile(t, filepath.Join(dir, "big.env"), big)

	testHome(t)
	code, _, _ := runVault(t, "", "scan", "--max-bytes=10", dir)
	if code != 0 {
		t.Errorf("expected file > max-bytes to be skipped, got exit %d", code)
	}
}

func TestPreviewMasksSecret(t *testing.T) {
	for _, in := range []string{"", "shortval", fakeOpenAI, fakeAWS} {
		got := preview(in)
		if in != "" && len(in) > 8 && strings.Contains(got, in) {
			t.Errorf("preview(%q) = %q leaks original", in, got)
		}
	}
}

func TestIsSkippedDir(t *testing.T) {
	skip := []string{".git", "node_modules", "vendor", ".aql", "dist"}
	keep := []string{"src", "cmd", "internal", "deep_dir"}
	for _, d := range skip {
		if !isSkippedDir(d) {
			t.Errorf("expected %q skipped", d)
		}
	}
	for _, d := range keep {
		if isSkippedDir(d) {
			t.Errorf("expected %q kept", d)
		}
	}
}

func TestSecretPatterns(t *testing.T) {
	tests := []struct {
		pattern, sample string
	}{
		{"openai-api-key", fakeOpenAI},
		{"anthropic-api-key", fakeAnthropic},
		{"github-token", fakeGitHub},
		{"github-fine-grained-pat", fakeGitHubPAT},
		{"aws-access-key-id", fakeAWS},
		{"google-api-key", fakeGoogle},
		{"slack-token", fakeSlack},
		{"stripe-key", fakeStripe},
		{"jwt", fakeJWT},
	}
	for _, tc := range tests {
		var matched bool
		for _, sp := range secretPatterns {
			if sp.Name == tc.pattern && sp.Pattern.MatchString(tc.sample) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("pattern %q did not match sample %q", tc.pattern, tc.sample)
		}
	}
}
