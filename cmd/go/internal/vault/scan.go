package vault

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// secretPattern is one named regex over file contents. The Name
// is reported in scanner output; Pattern is anchored against each
// line of every scanned file.
type secretPattern struct {
	Name    string
	Pattern *regexp.Regexp
}

// secretPatterns covers high-confidence provider tokens. The set
// is intentionally conservative: we want zero or near-zero false
// positives so developers do not learn to ignore the tool. Less
// distinctive patterns (generic high-entropy strings, hex blobs)
// are intentionally omitted — the right place for those is a
// dedicated linter like gitleaks or trufflehog.
var secretPatterns = []secretPattern{
	// OpenAI keys ("sk-..." and project-scoped "sk-proj-..."). The
	// 32+ char tail filters out the literal placeholder "sk-...".
	{"openai-api-key", regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{32,}\b`)},
	// Anthropic ("sk-ant-...").
	{"anthropic-api-key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{32,}\b`)},
	// GitHub PATs (classic) and OAuth/SHA/refresh tokens.
	{"github-token", regexp.MustCompile(`\bgh[opsur]_[A-Za-z0-9]{36,}\b`)},
	// GitHub fine-grained PAT.
	{"github-fine-grained-pat", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{60,}\b`)},
	// AWS access key id (the matching secret key needs context to
	// disambiguate from random base64; we leave that to dedicated
	// scanners).
	{"aws-access-key-id", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	// Slack bot/app/refresh/user/legacy tokens.
	{"slack-token", regexp.MustCompile(`\bxox[abprs]-[0-9A-Za-z-]{10,72}\b`)},
	// Stripe live/test keys.
	{"stripe-key", regexp.MustCompile(`\b(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{24,}\b`)},
	// Google Cloud API key.
	{"google-api-key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	// Compact JSON Web Tokens. Three dot-separated base64url
	// chunks of plausible length.
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
}

// scanResult is one finding from a scanned file.
type scanResult struct {
	Path    string
	Line    int
	Column  int
	Pattern string
	Preview string // first 6 chars of the match, rest masked
	Alias   string // populated if the value matches a stored alias
}

// runScan implements `aql vault scan`. By default it walks the
// current directory; one or more paths may be passed positionally
// to scan a focused subtree (e.g. `vault scan .env src/`).
func runScan(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	quiet := fs.Bool("quiet", false, "suppress per-finding output; rely on exit code")
	matchVault := fs.Bool("match-vault", false, "cross-reference findings against vault aliases")
	maxBytes := fs.Int64("max-bytes", 5<<20, "skip files larger than this many bytes")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Optional vault cross-reference. Failing to load the vault
	// degrades gracefully: matching just won't be available.
	var vaultIndex map[string]string
	if *matchVault {
		idx, err := buildVaultSecretIndex(homeDir)
		if err != nil {
			fmt.Fprintf(stderr, "warning: vault lookup unavailable: %s\n", err)
		} else {
			vaultIndex = idx
		}
	}

	var findings []scanResult
	for _, p := range paths {
		more, err := scanPath(p, *maxBytes, vaultIndex)
		if err != nil {
			fmt.Fprintf(stderr, "warning: scanning %s: %s\n", p, err)
		}
		findings = append(findings, more...)
	}

	if !*quiet {
		printFindings(stdout, findings)
	}
	if len(findings) > 0 {
		return 2
	}
	return 0
}

// printFindings writes a stable, machine-friendly summary. The
// preview never shows more than 6 leading characters of the
// secret — enough to identify provenance, not enough to weaponize.
func printFindings(w io.Writer, findings []scanResult) {
	if len(findings) == 0 {
		fmt.Fprintln(w, "vault scan: no findings")
		return
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
	for _, f := range findings {
		extra := ""
		if f.Alias != "" {
			extra = " (matches alias " + f.Alias + ")"
		}
		fmt.Fprintf(w, "%s:%d:%d: %s %s%s\n",
			f.Path, f.Line, f.Column, f.Pattern, f.Preview, extra)
	}
	fmt.Fprintf(w, "vault scan: %d finding(s)\n", len(findings))
}

// scanPath walks p, scanning every regular file it finds while
// skipping the conventional excludes (.git, node_modules, vendor,
// the vault's own state).
func scanPath(p string, maxBytes int64, vaultIndex map[string]string) ([]scanResult, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return scanFile(p, maxBytes, vaultIndex)
	}
	var out []scanResult
	err = filepath.WalkDir(p, func(path string, d iofs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		base := filepath.Base(path)
		if d.IsDir() {
			if isSkippedDir(base) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		res, ferr := scanFile(path, maxBytes, vaultIndex)
		if ferr == nil {
			out = append(out, res...)
		}
		return nil
	})
	return out, err
}

// isSkippedDir reports directories the scanner always ignores.
// These are either VCS metadata, dependency caches, or the vault's
// own state directory (which is encrypted and would otherwise
// trigger false-ish positives if a backup were dropped alongside).
func isSkippedDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", ".venv", "venv",
		"__pycache__", ".idea", ".vscode", ".aql":
		return true
	}
	return false
}

// scanFile reads p line by line and applies every pattern. Lines
// are limited to 64 KiB to bound memory; longer lines are split at
// arbitrary points (acceptable: secrets are short).
func scanFile(p string, maxBytes int64, vaultIndex map[string]string) ([]scanResult, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxBytes {
		return nil, nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []scanResult
	r := bufio.NewReader(f)
	lineNo := 0
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			lineNo++
			out = append(out, scanLine(p, lineNo, line, vaultIndex)...)
		}
		if err != nil {
			break
		}
	}
	return out, nil
}

func scanLine(path string, lineNo int, line string, vaultIndex map[string]string) []scanResult {
	if isLikelyExample(line) {
		return nil
	}
	var out []scanResult
	for _, sp := range secretPatterns {
		for _, idx := range sp.Pattern.FindAllStringIndex(line, -1) {
			value := line[idx[0]:idx[1]]
			r := scanResult{
				Path:    path,
				Line:    lineNo,
				Column:  idx[0] + 1,
				Pattern: sp.Name,
				Preview: preview(value),
			}
			if alias, ok := vaultIndex[value]; ok {
				r.Alias = alias
			}
			out = append(out, r)
		}
	}
	return out
}

// isLikelyExample skips lines that obviously contain a placeholder
// rather than a real secret. The signal is the presence of "..."
// or angle-bracketed placeholder text within the line.
func isLikelyExample(line string) bool {
	low := strings.ToLower(line)
	if strings.Contains(line, "...") || strings.Contains(line, "<your") ||
		strings.Contains(line, "<insert") || strings.Contains(line, "<replace") {
		return true
	}
	// Common .env.example markers.
	if strings.Contains(low, "example") || strings.Contains(low, "placeholder") {
		return true
	}
	return false
}

// preview masks all but the first 6 characters of a matched
// secret. For very short matches we keep nothing — the pattern
// name alone is enough to act on the finding.
func preview(value string) string {
	if len(value) <= 8 {
		return "****"
	}
	head := value[:6]
	return head + "***" + fmt.Sprintf(" (%d chars)", len(value))
}

// buildVaultSecretIndex returns a map from raw secret value to
// alias name, for cross-referencing scan findings. The vault must
// be initialized and unlocked for this to succeed.
func buildVaultSecretIndex(homeDir string) (map[string]string, error) {
	s, err := LoadStore(homeDir)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("vault not initialized")
	}
	if s.Locked {
		return nil, fmt.Errorf("vault is locked")
	}
	kr, err := openKeyring(s, homeDir, nil, io.Discard, "")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, a := range s.Aliases {
		v, err := kr.Get(a.Name)
		if err != nil {
			continue
		}
		out[v] = a.Name
	}
	return out, nil
}
