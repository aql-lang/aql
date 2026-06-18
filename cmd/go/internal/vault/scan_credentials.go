package vault

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// credentialPattern matches a line in a known credential file that
// carries a plaintext secret. Field names the credential kind for the
// report; Pattern must capture the secret value in group 1 so it can be
// masked and (optionally) cross-referenced against the vault.
type credentialPattern struct {
	Field   string
	Pattern *regexp.Regexp
}

// credentialFile is a well-known dotfile that conventionally stores a
// plaintext credential, paired with the patterns that detect a
// populated secret in that file's own format. Unlike the content
// scanner (secretPatterns, which matches provider-token *shapes*
// anywhere), this matches the *format* of each tool's credential store,
// so it flags tokens the generic regexes miss — npm `_authToken`, an
// AWS secret access key, a RubyGems API key, a `.netrc` password, etc.
type credentialFile struct {
	Rel      string // path relative to the home directory
	Tool     string // reported as the finding's source
	Patterns []credentialPattern
}

// credentialFiles is the curated set of credential stores `vault scan
// --home` inspects. The point is plaintext credentials that belong in a
// secret manager (registry / cloud / API tokens), not every dotfile.
// SSH private keys are intentionally out of scope — they are expected
// to live on disk and warrant a different (passphrase) check.
var credentialFiles = []credentialFile{
	{Rel: ".npmrc", Tool: "npm", Patterns: []credentialPattern{
		{"authToken", regexp.MustCompile(`(?i)_authtoken\s*=\s*(\S+)`)},
		{"auth", regexp.MustCompile(`(?i)(?:^|[/:])_(?:auth|password)\s*=\s*(\S+)`)},
	}},
	{Rel: ".yarnrc.yml", Tool: "yarn", Patterns: []credentialPattern{
		{"npmAuthToken", regexp.MustCompile(`(?i)npmAuthToken:\s*(\S+)`)},
		{"npmAuthIdent", regexp.MustCompile(`(?i)npmAuthIdent:\s*(\S+)`)},
	}},
	{Rel: ".netrc", Tool: "netrc", Patterns: []credentialPattern{
		{"password", regexp.MustCompile(`(?i)\bpassword\s+(\S+)`)},
	}},
	{Rel: ".authinfo", Tool: "authinfo", Patterns: []credentialPattern{
		{"password", regexp.MustCompile(`(?i)\bpassword\s+(\S+)`)},
	}},
	{Rel: ".aws/credentials", Tool: "aws", Patterns: []credentialPattern{
		{"secret_access_key", regexp.MustCompile(`(?i)^\s*aws_secret_access_key\s*=\s*(\S+)`)},
		{"session_token", regexp.MustCompile(`(?i)^\s*aws_session_token\s*=\s*(\S+)`)},
	}},
	{Rel: ".pypirc", Tool: "pypi", Patterns: []credentialPattern{
		{"password", regexp.MustCompile(`(?i)^\s*password\s*[:=]\s*(\S+)`)},
	}},
	{Rel: ".gem/credentials", Tool: "rubygems", Patterns: []credentialPattern{
		{"api_key", regexp.MustCompile(`(?i)^\s*:?[a-z0-9_]*api_key:\s*(\S+)`)},
	}},
	{Rel: ".git-credentials", Tool: "git", Patterns: []credentialPattern{
		{"url-password", regexp.MustCompile(`://[^/\s:@]+:([^/\s@]+)@`)},
	}},
	{Rel: ".docker/config.json", Tool: "docker", Patterns: []credentialPattern{
		{"auth", regexp.MustCompile(`"auth"\s*:\s*"([A-Za-z0-9+/=]+)"`)},
		{"identitytoken", regexp.MustCompile(`(?i)"identitytoken"\s*:\s*"([^"]+)"`)},
	}},
	{Rel: ".config/gh/hosts.yml", Tool: "github-cli", Patterns: []credentialPattern{
		{"oauth_token", regexp.MustCompile(`(?i)oauth_token:\s*(\S+)`)},
	}},
	{Rel: ".config/hub", Tool: "hub", Patterns: []credentialPattern{
		{"oauth_token", regexp.MustCompile(`(?i)oauth_token:\s*(\S+)`)},
	}},
	{Rel: ".cargo/credentials.toml", Tool: "cargo", Patterns: []credentialPattern{
		{"token", regexp.MustCompile(`(?i)^\s*token\s*=\s*(\S+)`)},
	}},
	{Rel: ".cargo/credentials", Tool: "cargo", Patterns: []credentialPattern{
		{"token", regexp.MustCompile(`(?i)^\s*token\s*=\s*(\S+)`)},
	}},
	{Rel: ".terraform.d/credentials.tfrc.json", Tool: "terraform", Patterns: []credentialPattern{
		{"token", regexp.MustCompile(`(?i)"token"\s*:\s*"([^"]+)"`)},
	}},
	{Rel: ".terraformrc", Tool: "terraform", Patterns: []credentialPattern{
		{"token", regexp.MustCompile(`(?i)\btoken\s*=\s*"([^"]+)"`)},
	}},
	{Rel: ".config/composer/auth.json", Tool: "composer", Patterns: []credentialPattern{
		{"password", regexp.MustCompile(`(?i)"password"\s*:\s*"([^"]+)"`)},
	}},
}

// scanCredentialFiles inspects the curated credential stores under home
// for populated plaintext secrets. Missing files are skipped silently —
// most users have only a few of these.
func scanCredentialFiles(home string, maxBytes int64, vaultIndex map[string]string) []scanResult {
	var out []scanResult
	for _, cf := range credentialFiles {
		full := filepath.Join(home, cf.Rel)
		info, err := os.Stat(full)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
			continue
		}
		out = append(out, scanCredentialFile(full, cf.Tool, cf.Patterns, vaultIndex)...)
	}
	return out
}

// scanCredentialFile reads one credential file line by line and applies
// that file's format-aware patterns.
func scanCredentialFile(path, tool string, pats []credentialPattern, vaultIndex map[string]string) []scanResult {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []scanResult
	r := bufio.NewReader(f)
	lineNo := 0
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			lineNo++
			out = append(out, scanCredentialLine(path, tool, lineNo, line, pats, vaultIndex)...)
		}
		if err != nil {
			break
		}
	}
	return out
}

// isEnvRef reports whether a captured value is a shell/env-var
// reference rather than a literal secret (e.g. `${NPM_TOKEN}`, `$TOKEN`).
func isEnvRef(v string) bool {
	return strings.HasPrefix(v, "$") || strings.Contains(v, "${")
}

// isPlaceholderValue reports whether a captured credential value is a
// placeholder rather than a real secret. It inspects the VALUE, not the
// whole line, so a legitimate `example.com` host in a .netrc entry does
// not suppress the password sitting next to it.
func isPlaceholderValue(v string) bool {
	if isEnvRef(v) || strings.Contains(v, "...") {
		return true
	}
	if strings.HasPrefix(v, "<") && strings.HasSuffix(v, ">") {
		return true
	}
	low := strings.ToLower(v)
	for _, p := range []string{"example", "placeholder", "your-", "yourtoken", "changeme", "redacted", "xxxx"} {
		if strings.Contains(low, p) {
			return true
		}
	}
	return false
}

// scanCredentialLine applies a file's patterns to one line, masking the
// captured secret and tagging it with the stored alias when it matches.
func scanCredentialLine(path, tool string, lineNo int, line string, pats []credentialPattern, vaultIndex map[string]string) []scanResult {
	var out []scanResult
	for _, cp := range pats {
		for _, m := range cp.Pattern.FindAllStringSubmatchIndex(line, -1) {
			// Group 1 is the secret value; fall back to the whole match.
			lo, hi := m[0], m[1]
			if len(m) >= 4 && m[2] >= 0 {
				lo, hi = m[2], m[3]
			}
			value := strings.Trim(line[lo:hi], `"' `)
			// Skip empties and placeholders. An env-var reference
			// (`${NPM_TOKEN}`) is in fact the *recommended* secure form — the
			// secret is not on disk — so flagging it would train users to
			// ignore us.
			if value == "" || isPlaceholderValue(value) {
				continue
			}
			r := scanResult{
				Path:    path,
				Line:    lineNo,
				Column:  lo + 1,
				Pattern: tool + ":" + cp.Field,
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
