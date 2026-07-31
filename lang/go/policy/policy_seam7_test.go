package policy

import (
	"errors"
	"strings"
	"testing"
)

// Seam-7 coverage: the residual error / boundary arms in evaluate.go,
// glob.go, load.go, profile.go, resolve.go and subset.go. Each block is
// driven by feeding crafted policy inputs directly to the (in-package)
// functions, swapping the existing seams.go vars for the OS/stdlib
// failure arms, or exercising an unexported helper at its boundary.
// Positives are paired with the negative/boundary case throughout.

// TestSeam7CheckModuleCallNilScope pins checkModuleCall's absent-modules
// arm: a policy with no modules scope reaches Check("modules","call") with
// a nil scope and must inherit allow-all.
func TestSeam7CheckModuleCallNilScope(t *testing.T) {
	p, err := LoadInline(`{name:"nomod"}`)
	if err != nil {
		t.Fatal(err)
	}
	// modules scope is absent → checkModuleCall(nil, …) → allow.
	if err := p.Check("modules", "call", Args{"module": "boru:x", "export": "y"}); err != nil {
		t.Errorf("absent modules scope should inherit allow: %v", err)
	}
	// Boundary twin: a present modules scope with a per-export deny still
	// refuses, proving the nil arm is the only thing being exercised above.
	p2, err := LoadInline(`{name:"m" scopes:{modules:{scopes:{"boru:x":{words:{default:"deny"}}}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.Check("modules", "call", Args{"module": "boru:x", "export": "y"}); err == nil {
		t.Error("present modules subscope default-deny must refuse")
	}
}

// TestSeam7CheckGlobalDefaultDenyBlame pins CheckGlobal's default-deny
// (no rule matched) blame arm, paired with the allow twin.
func TestSeam7CheckGlobalDefaultDenyBlame(t *testing.T) {
	p, err := LoadInline(`{name:"gd" scopes:{global:{words:{default:"deny"}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	err = p.CheckGlobal("network")
	if err == nil || !strings.Contains(err.Error(), "default=deny") {
		t.Errorf("CheckGlobal(network) = %v, want default=deny blame", err)
	}
	// Allow twin: default-allow global passes.
	pa, err := LoadInline(`{name:"ga" scopes:{global:{words:{default:"allow"}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := pa.CheckGlobal("network"); err != nil {
		t.Errorf("default-allow global should pass: %v", err)
	}
}

// TestSeam7Itoa pins itoa across its three arms. The negative arm is
// unreachable through Check/CheckGlobal (every call site guards ruleIdx
// >= 0 first), so it is driven directly at the helper boundary.
func TestSeam7Itoa(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{-1, "?"},
		{-42, "?"},
		{0, "0"},
		{7, "7"},
		{42, "42"},
		{1050, "1050"},
	}
	for _, c := range cases {
		if got := itoa(c.n); got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// TestSeam7GlobDoubleStarSuffix pins the `**X` arm (double star followed
// by a non-separator, non-empty tail): both the match (return true from
// the suffix loop) and the no-match (return false) case.
func TestSeam7GlobDoubleStarSuffix(t *testing.T) {
	cases := []struct {
		pat, s string
		want   bool
	}{
		{"**x", "abcx", true},    // suffix loop finds "x" → return true
		{"**x", "abc", false},    // no suffix matches → return false
		{"**x", "x", true},       // zero-length prefix
		{"a**z", "a/b/cz", true}, // ** spans separators before the tail
		{"a**z", "a/b/c", false},
	}
	for _, c := range cases {
		if got := Glob(c.pat, c.s); got != c.want {
			t.Errorf("Glob(%q, %q) = %v, want %v", c.pat, c.s, got, c.want)
		}
	}
}

// TestSeam7LoadInlineStdinError pins LoadInline("@-")'s stdin read-error
// arm by swapping the readAllStdin seam. Paired with a nil-error read that
// then compiles (proving the seam feeds through).
func TestSeam7LoadInlineStdinError(t *testing.T) {
	orig := readAllStdin
	t.Cleanup(func() { readAllStdin = orig })

	readAllStdin = func() ([]byte, error) { return nil, errors.New("stdin boom") }
	if _, err := LoadInline("@-"); err == nil ||
		!strings.Contains(err.Error(), "read stdin") {
		t.Errorf("LoadInline(@-) with stdin error = %v, want 'read stdin'", err)
	}

	// Twin: a successful read of valid jsonic compiles.
	readAllStdin = func() ([]byte, error) { return []byte(`{name:"fromstdin"}`), nil }
	p, err := LoadInline("@-")
	if err != nil || p.Name() != "fromstdin" {
		t.Errorf("LoadInline(@-) happy = %v (%v)", p, err)
	}
}

// TestSeam7ParseProfileSafeParseError pins parseProfile's SafeParse error
// arm with malformed jsonic, paired with a well-formed parse.
func TestSeam7ParseProfileSafeParseError(t *testing.T) {
	if _, err := parseProfile([]byte("}{"), "<probe>"); err == nil ||
		!strings.Contains(err.Error(), "<probe>") {
		t.Errorf("parseProfile(malformed) = %v, want a <probe> parse error", err)
	}
	if p, err := parseProfile([]byte(`{name:"ok"}`), "<probe>"); err != nil || p == nil ||
		p.Name != "ok" {
		t.Errorf("parseProfile(valid) = %v (%v)", p, err)
	}
}

// TestSeam7ParseProfileMarshalError pins parseProfile's jsonMarshal error
// arm by swapping the jsonMarshal seam — jsonic output is always
// marshalable, so only the seam can observe the propagation.
func TestSeam7ParseProfileMarshalError(t *testing.T) {
	orig := jsonMarshal
	t.Cleanup(func() { jsonMarshal = orig })

	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }
	if _, err := parseProfile([]byte(`{name:"x"}`), "<probe>"); err == nil ||
		!strings.Contains(err.Error(), "marshal") {
		t.Errorf("parseProfile with marshal error = %v, want 'marshal'", err)
	}
}

// TestSeam7ExpandTildeNoHome pins expandTilde's no-home arm: when
// userHomeDir fails, the leading-~ path is returned unchanged. Paired with
// a non-tilde path that is always returned verbatim.
func TestSeam7ExpandTildeNoHome(t *testing.T) {
	orig := userHomeDir
	t.Cleanup(func() { userHomeDir = orig })

	userHomeDir = func() (string, error) { return "", errors.New("no home") }
	if got := expandTilde("~/sub"); got != "~/sub" {
		t.Errorf("expandTilde(~/sub) with no home = %q, want unchanged", got)
	}
	if got := expandTilde("~"); got != "~" {
		t.Errorf("expandTilde(~) with no home = %q, want unchanged", got)
	}
	// Twin: a non-tilde path never consults userHomeDir at all.
	if got := expandTilde("/abs/path"); got != "/abs/path" {
		t.Errorf("expandTilde(/abs/path) = %q, want verbatim", got)
	}
}

// TestSeam7ValidateNilProfile pins Validate's nil-receiver arm.
func TestSeam7ValidateNilProfile(t *testing.T) {
	if err := (*Profile)(nil).Validate(); err != nil {
		t.Errorf("(*Profile)(nil).Validate() = %v, want nil", err)
	}
}

// TestSeam7ValidateNilScope pins Validate's nil-scope continue arm: a known
// scope name mapping to a nil *Scope is skipped rather than dereferenced.
func TestSeam7ValidateNilScope(t *testing.T) {
	p := &Profile{Scopes: map[string]*Scope{"engine": nil}}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() with a nil scope = %v, want nil", err)
	}
	// Boundary twin: an unknown scope name still fails before the nil check.
	pu := &Profile{Scopes: map[string]*Scope{"not-a-scope": nil}}
	if err := pu.Validate(); err == nil ||
		!strings.Contains(err.Error(), "unknown scope") {
		t.Errorf("Validate() with unknown scope = %v, want rejection", err)
	}
}

// TestSeam7ValidateModulesSubscopeInvalid pins Validate's per-module
// subscope validation error arm (a subscope words block that is itself
// invalid), paired with a valid subscope.
func TestSeam7ValidateModulesSubscopeInvalid(t *testing.T) {
	bad := &Profile{Scopes: map[string]*Scope{
		"modules": {Scopes: map[string]*Scope{
			"boru:x": {Words: WordsBlock{Default: "maybe"}},
		}},
	}}
	if err := bad.Validate(); err == nil ||
		!strings.Contains(err.Error(), "default") {
		t.Errorf("Validate() with invalid subscope words = %v, want 'default' error", err)
	}
	good := &Profile{Scopes: map[string]*Scope{
		"modules": {Scopes: map[string]*Scope{
			"boru:x": {Words: WordsBlock{Default: EffectAllow}},
		}},
	}}
	if err := good.Validate(); err != nil {
		t.Errorf("Validate() with valid subscope = %v, want nil", err)
	}
}

// TestSeam7ResolveChainResolverError pins resolveChain's resolver-error
// arm: an extends whose parent lookup fails is wrapped, not swallowed.
func TestSeam7ResolveChainResolverError(t *testing.T) {
	p := &Profile{Name: "c", Extends: "base"}
	_, err := p.Compile(func(name string) (*Profile, error) {
		return nil, profileError("resolver boom for %q", name)
	})
	if err == nil || !strings.Contains(err.Error(), "extends") {
		t.Errorf("Compile with erroring resolver = %v, want 'extends' wrap", err)
	}
	// Twin: a resolver that succeeds resolves the chain cleanly.
	ok, err := (&Profile{Name: "c2", Extends: "base"}).Compile(
		func(name string) (*Profile, error) { return &Profile{Name: name}, nil })
	if err != nil || ok == nil {
		t.Errorf("Compile with good resolver = %v (%v)", ok, err)
	}
}

// TestSeam7RequireSubsetChildEmptyDefault pins RequireSubset's
// empty-child-default arm: a child scope that is present and installed but
// omits its words.default defaults to allow, which widens a parent's
// default-deny and must be flagged as attenuation.
func TestSeam7RequireSubsetChildEmptyDefault(t *testing.T) {
	parent, err := LoadInline(`{name:"pdf" scopes:{fileops:{words:{default:"deny"}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	// Child fileops present, installed, but no explicit default.
	childEmpty, err := LoadInline(`{name:"cef" scopes:{fileops:{}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireSubset(childEmpty, parent); err == nil ||
		!strings.Contains(err.Error(), "default-allows") {
		t.Errorf("empty-default child widening = %v, want attenuation denial", err)
	}
	// Twin: an explicit default-deny child is not broader — passes.
	childDeny, err := LoadInline(`{name:"cdf" scopes:{fileops:{words:{default:"deny"}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireSubset(childDeny, parent); err != nil {
		t.Errorf("matching default-deny child should pass: %v", err)
	}
}
