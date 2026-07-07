package policy

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Wave-4 coverage: the composed-policy word/scope/limit views, the Denied
// error renderings, the modules.call subscope dispatch, the where-predicate
// matchers, the load entry points (stdin, file, map, user profile dirs,
// tilde expansion), the profile merge arms, and RequireSubset's attenuation
// checks — positives paired with denials throughout.

// w4Inline compiles an inline jsonic profile or fails the test.
func w4Inline(t *testing.T, src string) Policy {
	t.Helper()
	p, err := LoadInline(src)
	if err != nil {
		t.Fatalf("LoadInline: %v", err)
	}
	return p
}

// TestWave4ComposedCheckWord pins the composed CheckWord AND: the parent's
// engine-scope deny applies even when the child allows, and vice versa.
func TestWave4ComposedCheckWord(t *testing.T) {
	denyEval := `{name:"p" scopes:{engine:{words:{default:"allow" rules:[{deny:["eval"]}]}}}}`
	allowAll := `{name:"c" scopes:{engine:{words:{default:"allow"}}}}`

	c := Compose(w4Inline(t, denyEval), w4Inline(t, allowAll))
	if err := c.CheckWord("eval"); err == nil {
		t.Error("parent deny of eval must apply through the composed policy")
	}
	if err := c.CheckWord("add"); err != nil {
		t.Errorf("add should pass both layers: %v", err)
	}
	c = Compose(w4Inline(t, allowAll), w4Inline(t, denyEval))
	if err := c.CheckWord("eval"); err == nil {
		t.Error("child deny of eval must apply through the composed policy")
	}
}

// TestWave4ComposedScopeAndLimits pins the composed Scope view (parent
// install:false stamps the child's snapshot) and the most-restrictive
// limit merge across the zero / non-zero arms.
func TestWave4ComposedScopeAndLimits(t *testing.T) {
	parent := w4Inline(t, `{name:"p" limits:{timeoutMs:5 maxStackDepth:9} scopes:{fileops:{install:false}}}`)
	child := w4Inline(t, `{name:"c" limits:{timeoutMs:9 maxStackDepth:5} scopes:{fileops:{words:{default:"allow"}}}}`)
	c := Compose(parent, child)

	s := c.Scope("fileops")
	if s.Install == nil || *s.Install {
		t.Error("parent install=false must be visible in the composed Scope")
	}
	if s2 := Compose(child, parent).Scope("fileops"); s2.Install == nil || *s2.Install {
		t.Error("install=false on the other layer must be visible too")
	}

	lim := c.Limits()
	if lim.TimeoutMs != 5 || lim.MaxStackDepth != 5 {
		t.Errorf("composed limits = %+v, want the smaller of each pair", lim)
	}
	if got := c.Name(); got != "c in p" {
		t.Errorf("composed name = %q, want 'c in p'", got)
	}

	// minNonZero / minNonZeroInt arm sweep.
	if minNonZero(0, 7) != 7 || minNonZero(7, 0) != 7 || minNonZero(3, 7) != 3 || minNonZero(7, 3) != 3 {
		t.Error("minNonZero arms broken")
	}
	if minNonZeroInt(0, 7) != 7 || minNonZeroInt(7, 0) != 7 || minNonZeroInt(3, 7) != 3 || minNonZeroInt(7, 3) != 3 {
		t.Error("minNonZeroInt arms broken")
	}
}

// TestWave4DeniedError pins each rendering arm of Denied.Error.
func TestWave4DeniedError(t *testing.T) {
	cases := []struct {
		d    Denied
		want string
	}{
		{Denied{Code: CodeCapabilityNotInstalled, Scope: "fileops", Profile: "p", Blame: "b"},
			"is not installed"},
		{Denied{Code: CodeModulesDisabled, Profile: "p"}, "modules disabled"},
		{Denied{Code: CodePolicyAttenuation, Blame: "b"}, "attenuation error"},
		{Denied{Code: CodePermissionDenied, Scope: "fileops", Op: "write", Profile: "p", Blame: "b"},
			"permission denied"},
	}
	for _, c := range cases {
		if got := c.d.Error(); !strings.Contains(got, c.want) {
			t.Errorf("Denied{%s}.Error() = %q, want substring %q", c.d.Code, got, c.want)
		}
	}
}

// TestWave4CompiledScope pins the Compiled.Scope snapshot: a present scope
// is returned, an absent one is the zero Scope.
func TestWave4CompiledScope(t *testing.T) {
	p := w4Inline(t, `{name:"p" scopes:{fileops:{words:{default:"deny"}}}}`)
	c, ok := p.(*Compiled)
	if !ok {
		t.Fatalf("LoadInline returned %T, want *Compiled", p)
	}
	if got := c.Scope("fileops"); got.Words.Default != EffectDeny {
		t.Errorf("Scope(fileops).Words.Default = %q, want deny", got.Words.Default)
	}
	if got := c.Scope("no-such"); got.Words.Default != "" || got.Install != nil {
		t.Errorf("Scope(absent) = %+v, want zero", got)
	}
}

// TestWave4ModuleCallSubscopes pins checkModuleCall: an absent subscope
// inherits allow, install:false denies with the subscope blame, and a
// per-export deny rule denies with the rule blame.
func TestWave4ModuleCallSubscopes(t *testing.T) {
	p := w4Inline(t, `{name:"p" scopes:{modules:{
		words:{default:"allow"}
		scopes:{
			"aql:off":{install:false}
			"aql:part":{words:{default:"allow" rules:[{deny:["secret*"]}]}}
		}}}}`)

	if err := p.Check("modules", "call", Args{"module": "aql:other", "export": "x"}); err != nil {
		t.Errorf("absent subscope should inherit allow: %v", err)
	}
	err := p.Check("modules", "call", Args{"module": "aql:off", "export": "x"})
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("install=false subscope: %v, want not-installed", err)
	}
	err = p.Check("modules", "call", Args{"module": "aql:part", "export": "secret-word"})
	if err == nil || !strings.Contains(err.Error(), "rule #0") {
		t.Errorf("per-export deny: %v, want rule #0 blame", err)
	}
	if err := p.Check("modules", "call", Args{"module": "aql:part", "export": "open"}); err != nil {
		t.Errorf("non-matching export should pass: %v", err)
	}
}

// TestWave4CheckGlobalRuleBlame pins CheckGlobal's rule-indexed blame arm
// and Compiled.CheckWord's engine delegation.
func TestWave4CheckGlobalRuleBlame(t *testing.T) {
	p := w4Inline(t, `{name:"p" scopes:{global:{words:{default:"allow" rules:[{deny:["network"]}]}}}}`)
	err := p.CheckGlobal("network")
	if err == nil || !strings.Contains(err.Error(), "rule #0") {
		t.Errorf("CheckGlobal(network) = %v, want rule #0 blame", err)
	}
	if err := p.CheckGlobal("clock"); err != nil {
		t.Errorf("clock should pass: %v", err)
	}

	pw := w4Inline(t, `{name:"p" scopes:{engine:{words:{default:"deny"}}}}`)
	c := pw.(*Compiled)
	if err := c.CheckWord("anything"); err == nil {
		t.Error("engine default-deny must reach CheckWord")
	}
}

// TestWave4WhereMatchers pins whereKeyMatches / matchOne / numEq across
// every type arm.
func TestWave4WhereMatchers(t *testing.T) {
	if !whereKeyMatches([]any{"x"}, nil) {
		t.Error("nil actual must pass vacuously")
	}
	if !whereKeyMatches([]any{"a*"}, "ab") || whereKeyMatches([]any{"a*"}, "zb") {
		t.Error("glob arm broken")
	}
	if matchOne("a", 5) {
		t.Error("string pattern vs non-string must refuse")
	}
	if !matchOne(true, true) || matchOne(true, false) || matchOne(true, "x") {
		t.Error("bool arm broken")
	}
	if !matchOne(5, 5) || !matchOne(int64(5), int64(5)) || !matchOne(5.0, 5) {
		t.Error("numeric arms broken")
	}
	if matchOne([]any{}, "x") {
		t.Error("unknown pattern type must refuse")
	}
	if !numEq(5, 5) || !numEq(5, int64(5)) || !numEq(5, 5.0) || numEq(5, "5") || numEq(5, 6) {
		t.Error("numEq arms broken")
	}
}

// TestWave4GlobSeparators pins the single-star separator wall, the
// trailing-star form and the ? wildcard.
func TestWave4GlobSeparators(t *testing.T) {
	cases := []struct {
		pat, s string
		want   bool
	}{
		{"a*", "ab", true},
		{"a*", "a/b", false},   // trailing star cannot span /
		{"a*c", "ab/c", false}, // mid star hits the separator wall
		{"a*c", "abc", true},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"a/**", "a/b/c", true},
		{"**/x", "a/b/x", true},
		{"**", "anything/at/all", true},
		{"", "", true},
		{"", "x", false},
	}
	for _, c := range cases {
		if got := Glob(c.pat, c.s); got != c.want {
			t.Errorf("Glob(%q, %q) = %v, want %v", c.pat, c.s, got, c.want)
		}
	}
}

// TestWave4LoadEntryPoints pins LoadFile's missing-file arm, LoadInline's
// stdin form (happy and malformed), the inline decode rejection, LoadAuto's
// empty-input default, and FromMap's three arms.
func TestWave4LoadEntryPoints(t *testing.T) {
	if _, err := LoadFile(filepath.Join(t.TempDir(), "missing.jsonic")); err == nil {
		t.Error("LoadFile of a missing path must error")
	}

	// LoadInline "@-" reads the profile from stdin.
	feedStdin := func(t *testing.T, data string) func() {
		t.Helper()
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.WriteString(data); err != nil {
			t.Fatal(err)
		}
		w.Close()
		old := os.Stdin
		os.Stdin = r
		return func() { os.Stdin = old; r.Close() }
	}
	restore := feedStdin(t, `{name:"stdin-prof" scopes:{fileops:{words:{default:"deny"}}}}`)
	p, err := LoadInline("@-")
	restore()
	if err != nil || p.Name() != "stdin-prof" {
		t.Errorf("stdin profile = %v (%v), want stdin-prof", p, err)
	}
	restore = feedStdin(t, `{zzz-not-a-field: 1}`)
	_, err = LoadInline("@-")
	restore()
	if err == nil {
		t.Error("malformed stdin profile must error")
	}

	if _, err := LoadInline(`{zzz-not-a-field: 1}`); err == nil {
		t.Error("unknown profile field must be rejected")
	}
	if _, err := LoadInline("@" + filepath.Join(t.TempDir(), "nope.jsonic")); err == nil {
		t.Error("@path to a missing file must error")
	}

	p, err = LoadAuto("   ")
	if err != nil || p == nil {
		t.Errorf("empty LoadAuto should load the full profile: %v", err)
	}
	if _, err := LoadAuto("@" + filepath.Join(t.TempDir(), "nope.jsonic")); err == nil {
		t.Error("LoadAuto @missing must error")
	}

	// FromMap: happy, unmarshalable, and unknown-field arms.
	p, err = FromMap(map[string]any{"name": "mapped", "scopes": map[string]any{
		"fileops": map[string]any{"words": map[string]any{"default": "deny"}},
	}})
	if err != nil || p.Name() != "mapped" {
		t.Errorf("FromMap = %v (%v), want mapped", p, err)
	}
	if _, err := FromMap(map[string]any{"x": math.NaN()}); err == nil {
		t.Error("FromMap with an unmarshalable value must error")
	}
	if _, err := FromMap(map[string]any{"zzz-not-a-field": 1}); err == nil {
		t.Error("FromMap with an unknown field must error")
	}
}

// TestWave4UserProfileDirs pins loadProfileByName's user-directory search
// (.jsonic first, .json fallback) via XDG_CONFIG_HOME.
func TestWave4UserProfileDirs(t *testing.T) {
	dir := t.TempDir()
	polDir := filepath.Join(dir, "aql", "policies")
	if err := os.MkdirAll(polDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := os.WriteFile(filepath.Join(polDir, "myjsonic.jsonic"),
		[]byte(`{name:"myjsonic"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(polDir, "myjson.json"),
		[]byte(`{"name":"myjson"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Load("myjsonic")
	if err != nil || p.Name() != "myjsonic" {
		t.Errorf("Load(myjsonic) = %v (%v)", p, err)
	}
	p, err = Load("myjson")
	if err != nil || p.Name() != "myjson" {
		t.Errorf("Load(myjson) = %v (%v)", p, err)
	}
	if _, err := Load("definitely-not-a-profile"); err == nil {
		t.Error("unknown profile name must error")
	}
}

// TestWave4ExpandTilde pins every expandTilde arm.
func TestWave4ExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := expandTilde(""); got != "" {
		t.Errorf("expandTilde('') = %q", got)
	}
	if got := expandTilde("/abs/path"); got != "/abs/path" {
		t.Errorf("expandTilde(abs) = %q", got)
	}
	if got := expandTilde("~"); got != home {
		t.Errorf("expandTilde(~) = %q, want %q", got, home)
	}
	if got := expandTilde("~/sub"); got != filepath.Join(home, "sub") {
		t.Errorf("expandTilde(~/sub) = %q", got)
	}
	if got := expandTilde("~otheruser/x"); got != "~otheruser/x" {
		t.Errorf("expandTilde(~user) = %q, want untouched", got)
	}
}

// TestWave4GlobalsFor pins the fixed scope/op → global-cap table.
func TestWave4GlobalsFor(t *testing.T) {
	cases := []struct {
		scope, op string
		want      []string
	}{
		{"fileops", "read", []string{"disk.read"}},
		{"fileops", "write", []string{"disk.write"}},
		{"fileops", "mkdir", []string{"disk.write"}},
		{"fileops", "stat", nil},
		{"sqlite", "open-ro", []string{"disk.read"}},
		{"sqlite", "query", []string{"disk.read"}},
		{"sqlite", "open-rw", []string{"disk.read", "disk.write"}},
		{"sqlite", "exec", []string{"disk.write"}},
		{"sqlite", "other", nil},
		{"network", "fetch", []string{"network"}},
		{"process", "spawn", []string{"process"}},
		{"env", "get", []string{"env"}},
		{"clock", "now", []string{"clock"}},
		{"unknown", "x", nil},
	}
	for _, c := range cases {
		got := GlobalsFor(c.scope, c.op)
		if len(got) != len(c.want) {
			t.Errorf("GlobalsFor(%s, %s) = %v, want %v", c.scope, c.op, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("GlobalsFor(%s, %s) = %v, want %v", c.scope, c.op, got, c.want)
			}
		}
	}
}

// TestWave4ValidateRejections pins Validate's schema errors: install on an
// intrinsic scope, subscopes on a non-modules scope, and unknown global
// ops — alongside the accepted twin.
func TestWave4ValidateRejections(t *testing.T) {
	if _, err := LoadInline(`{name:"p" scopes:{engine:{install:false}}}`); err == nil ||
		!strings.Contains(err.Error(), "install") {
		t.Errorf("install on engine: %v, want rejection", err)
	}
	if _, err := LoadInline(`{name:"p" scopes:{fileops:{scopes:{sub:{}}}}}`); err == nil ||
		!strings.Contains(err.Error(), "subscopes") {
		t.Errorf("subscope on fileops: %v, want rejection", err)
	}
	if _, err := LoadInline(`{name:"p" scopes:{global:{words:{rules:[{deny:["not-a-global"]}]}}}}`); err == nil ||
		!strings.Contains(err.Error(), "unknown global op") {
		t.Errorf("unknown global op: %v, want rejection", err)
	}
	if _, err := LoadInline(`{name:"p" scopes:{fileops:{words:{default:"maybe"}}}}`); err == nil ||
		!strings.Contains(err.Error(), "default") {
		t.Errorf("bad default: %v, want rejection", err)
	}
	if _, err := LoadInline(`{name:"ok" scopes:{modules:{scopes:{"aql:x":{words:{default:"allow"}}}}}}`); err != nil {
		t.Errorf("valid modules subscope refused: %v", err)
	}
}

// TestWave4ResolveMerge pins the profile merge plumbing directly: the nil
// chain, version/name inheritance, limit overrides, scope merge recursion
// and cloneScope's nil arm.
func TestWave4ResolveMerge(t *testing.T) {
	if p, err := resolveChain(nil, nil, nil); err != nil || p == nil {
		t.Errorf("resolveChain(nil) = %v (%v)", p, err)
	}
	// A nil visited map is allocated on demand.
	resolver := func(name string) (*Profile, error) { return &Profile{Name: name}, nil }
	if p, err := resolveChain(&Profile{Name: "c", Extends: "base"}, resolver, nil); err != nil ||
		p.Name != "c" {
		t.Errorf("resolveChain nil-visited = %v (%v)", p, err)
	}

	on := true
	parent := &Profile{
		Version: 1,
		Name:    "parent",
		Limits:  Limits{TimeoutMs: 1, MaxStepBudget: 2, MaxStackDepth: 3, MaxMemoryBytes: 4, MaxOutputBytes: 5, MaxSubEngineDepth: 6},
		Scopes: map[string]*Scope{
			"fileops": {Words: WordsBlock{Default: EffectDeny}},
			"modules": {Scopes: map[string]*Scope{"aql:a": {Install: &on}}},
		},
	}
	child := &Profile{
		Limits: Limits{TimeoutMs: 10, MaxStepBudget: 20, MaxStackDepth: 30, MaxMemoryBytes: 40, MaxOutputBytes: 50, MaxSubEngineDepth: 60},
		Scopes: map[string]*Scope{
			"fileops": {Words: WordsBlock{Default: EffectAllow, Rules: []Rule{{Deny: []string{"x"}}}}},
			"modules": {Scopes: map[string]*Scope{"aql:a": {Words: WordsBlock{Default: EffectDeny}}, "aql:b": {}}},
			"network": {Words: WordsBlock{Default: EffectDeny}},
		},
	}
	merged := mergeProfile(parent, child)
	if merged.Version != 1 || merged.Name != "parent" {
		t.Errorf("merge should inherit version and name: %+v", merged)
	}
	if merged.Limits.TimeoutMs != 10 || merged.Limits.MaxStepBudget != 20 ||
		merged.Limits.MaxStackDepth != 30 || merged.Limits.MaxMemoryBytes != 40 ||
		merged.Limits.MaxOutputBytes != 50 || merged.Limits.MaxSubEngineDepth != 60 {
		t.Errorf("child limits must override: %+v", merged.Limits)
	}
	fo := merged.Scopes["fileops"]
	if fo.Words.Default != EffectAllow || len(fo.Words.Rules) != 1 {
		t.Errorf("fileops merge = %+v", fo)
	}
	mo := merged.Scopes["modules"]
	if mo.Scopes["aql:a"] == nil || mo.Scopes["aql:a"].Words.Default != EffectDeny ||
		mo.Scopes["aql:a"].Install == nil || mo.Scopes["aql:b"] == nil {
		t.Errorf("modules subscope merge = %+v", mo.Scopes)
	}
	if merged.Scopes["network"] == nil {
		t.Error("child-only scope must survive the merge")
	}

	if mergeScope(nil, child.Scopes["fileops"]) == nil {
		t.Error("mergeScope(nil, child) should clone the child")
	}
	if mergeScope(parent.Scopes["fileops"], nil) == nil {
		t.Error("mergeScope(parent, nil) should clone the parent")
	}
	if cloneScope(nil) != nil {
		t.Error("cloneScope(nil) must be nil")
	}
}

// TestWave4RequireSubset pins the deprecated attenuation pre-check: the
// non-Compiled rejections, the global widening, the install widening and
// the default widening arms.
func TestWave4RequireSubset(t *testing.T) {
	full := w4Inline(t, `{name:"full"}`)
	composedP := Compose(full, w4Inline(t, `{name:"x"}`))

	if err := RequireSubset(composedP, full); err == nil ||
		!strings.Contains(err.Error(), "child is not") {
		t.Errorf("non-Compiled child: %v", err)
	}
	if err := RequireSubset(full, composedP); err == nil ||
		!strings.Contains(err.Error(), "parent is not") {
		t.Errorf("non-Compiled parent: %v", err)
	}

	parentNoNet := w4Inline(t, `{name:"pnn" scopes:{global:{words:{default:"allow" rules:[{deny:["network"]}]}}}}`)
	if err := RequireSubset(full, parentNoNet); err == nil ||
		!strings.Contains(err.Error(), "global.network") {
		t.Errorf("global widening: %v, want attenuation denial", err)
	}

	parentNoFS := w4Inline(t, `{name:"pnf" scopes:{fileops:{install:false}}}`)
	if err := RequireSubset(full, parentNoFS); err == nil ||
		!strings.Contains(err.Error(), "install=false") {
		t.Errorf("install widening: %v, want attenuation denial", err)
	}

	parentDenyFS := w4Inline(t, `{name:"pdf" scopes:{fileops:{words:{default:"deny"}}}}`)
	if err := RequireSubset(full, parentDenyFS); err == nil ||
		!strings.Contains(err.Error(), "default-allows") {
		t.Errorf("child-absent default widening: %v", err)
	}
	childAllowFS := w4Inline(t, `{name:"caf" scopes:{fileops:{words:{default:"allow"}}}}`)
	if err := RequireSubset(childAllowFS, parentDenyFS); err == nil ||
		!strings.Contains(err.Error(), "default-allows") {
		t.Errorf("explicit default widening: %v", err)
	}
	childDenyFS := w4Inline(t, `{name:"cdf" scopes:{fileops:{words:{default:"deny"}}}}`)
	if err := RequireSubset(childDenyFS, parentDenyFS); err != nil {
		t.Errorf("matching default-deny should pass: %v", err)
	}
	childOffFS := w4Inline(t, `{name:"cof" scopes:{fileops:{install:false}}}`)
	if err := RequireSubset(childOffFS, parentDenyFS); err != nil {
		t.Errorf("child install=false is more restrictive, should pass: %v", err)
	}
}
