package policy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	jsonic "github.com/jsonicjs/jsonic/go"
)

// Load resolves name to a Policy. Resolution order:
//
//  1. Built-in profile (full, trusted, sandbox, …).
//  2. User profile at $XDG_CONFIG_HOME/aql/policies/<name>.jsonic
//     (falls back to $HOME/.config/aql/policies/<name>.jsonic).
//
// Returns an error if the name cannot be resolved or if the resolved
// profile fails to compile.
func Load(name string) (Policy, error) {
	p, err := loadProfileByName(name)
	if err != nil {
		return nil, err
	}
	return p.Compile(loadProfileByName)
}

// LoadFile loads a profile from a filesystem path.
func LoadFile(path string) (Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("policy: read %s: %w", path, err)
	}
	p, err := parseProfile(data, path)
	if err != nil {
		return nil, err
	}
	return p.Compile(loadProfileByName)
}

// LoadInline parses a profile from an inline jsonic string. The "@-"
// prefix reads from stdin; the "@<path>" prefix reads from a file
// (equivalent to LoadFile but available through the same entry
// point for CLI ergonomics).
func LoadInline(src string) (Policy, error) {
	switch {
	case src == "@-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("policy: read stdin: %w", err)
		}
		p, err := parseProfile(data, "<stdin>")
		if err != nil {
			return nil, err
		}
		return p.Compile(loadProfileByName)
	case strings.HasPrefix(src, "@"):
		return LoadFile(src[1:])
	default:
		p, err := parseProfile([]byte(src), "<inline>")
		if err != nil {
			return nil, err
		}
		return p.Compile(loadProfileByName)
	}
}

// LoadAuto resolves a single user-supplied string, auto-detecting
// whether it is a profile name, a file path, or an inline jsonic
// document. Detection rules:
//
//   - starts with '{' or '['          → inline jsonic
//   - starts with '@'                 → file or stdin (LoadInline rules)
//   - contains '/' or has known ext   → file path
//   - otherwise                       → profile name
func LoadAuto(src string) (Policy, error) {
	s := strings.TrimLeftFunc(src, isSpace)
	if s == "" {
		return Load("full")
	}
	switch {
	case s[0] == '{' || s[0] == '[':
		return LoadInline(src)
	case s[0] == '@':
		return LoadInline(src)
	case strings.ContainsRune(src, '/') ||
		strings.HasSuffix(src, ".jsonic") ||
		strings.HasSuffix(src, ".json"):
		return LoadFile(src)
	}
	return Load(src)
}

// FromMap constructs a Policy from a map (as produced by jsonic
// parsing or by handcrafting in Go). Used by the aql:vm module to
// accept policies as runtime data.
func FromMap(m map[string]any) (Policy, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("policy: marshal map: %w", err)
	}
	p, err := parseProfile(data, "<map>")
	if err != nil {
		return nil, err
	}
	return p.Compile(loadProfileByName)
}

// parseProfile decodes a jsonic byte slice into a *Profile.
func parseProfile(data []byte, sourceName string) (*Profile, error) {
	j := jsonic.Make()
	raw, err := j.Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("policy: %s: %w", sourceName, err)
	}
	// Convert the parsed any → JSON → Profile to reuse encoding/json's
	// struct tag handling. The Profile shape is plain enough that
	// this round-trip is a single allocation each side.
	canon, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("policy: %s: marshal: %w", sourceName, err)
	}
	var p Profile
	dec := json.NewDecoder(strings.NewReader(string(canon)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("policy: %s: decode: %w", sourceName, err)
	}
	return &p, nil
}

// loadProfileByName looks up a profile by short name. Used both as
// the entry point for Load and as the resolver callback for
// compile-time extends resolution.
func loadProfileByName(name string) (*Profile, error) {
	if data, ok := builtinProfileData(name); ok {
		return parseProfile(data, "builtin:"+name)
	}
	// User profile directory.
	for _, dir := range userPolicyDirs() {
		path := filepath.Join(dir, name+".jsonic")
		if data, err := os.ReadFile(path); err == nil {
			return parseProfile(data, path)
		}
		path = filepath.Join(dir, name+".json")
		if data, err := os.ReadFile(path); err == nil {
			return parseProfile(data, path)
		}
	}
	return nil, fmt.Errorf("policy: profile %q not found (built-in or in user policies dir)", name)
}

// userPolicyDirs returns the candidate directories searched for
// user-defined profiles. Order: $XDG_CONFIG_HOME/aql/policies first,
// then $HOME/.config/aql/policies as the fallback.
func userPolicyDirs() []string {
	var dirs []string
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		dirs = append(dirs, filepath.Join(x, "aql", "policies"))
	}
	if h, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(h, ".config", "aql", "policies"))
	}
	return dirs
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
