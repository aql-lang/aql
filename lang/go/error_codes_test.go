// error_codes_test.go pins the error CODES that REFERENCE.md's "Common
// codes" table promises, for the conditions that used to raise
// code-lessly.
//
// A code-less error is not a cosmetic defect. The code is the only part
// of an Error a handler can dispatch on —
//
//	do [risky] error [dot code case [not_found/q "…" read_error/q "…"]]
//
// — so `code: None` silently defeats every such `case`, and the author's
// only way to notice is to provoke the condition and print the result.
// Four ordinary failures were in that state: an out-of-range index, a
// strict read of the wrong container shape, a missing file, and a policy
// refusal. See design/verse-report-defects-investigation.0.md §E.
//
// The companion gate is test/go/docexamples/errorcodes_test.go, which
// proves the DOCUMENTED codes are all mintable. This file proves the
// specific conditions actually mint them, which the doc gate cannot.
package lang_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/policy"
)

// codeOf runs src and returns the AqlError code of the failure. An empty
// string means the program either succeeded or failed with a code-less
// (foreign) error — the exact defect these tests exist to catch, so the
// two are reported apart.
func codeOf(t *testing.T, a *lang.AQL, src string) string {
	t.Helper()
	_, err := a.Run(src)
	if err == nil {
		t.Fatalf("expected a failure from:\n%s", src)
	}
	var ae *eng.AqlError
	if !errors.As(err, &ae) {
		t.Fatalf("failure carries no AqlError at all, so it can never have a "+
			"code:\n%s\n  → %v", src, err)
	}
	return ae.Code
}

func TestRuntimeFailuresCarryTheDocumentedCode(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "definitely-not-here.txt")
	cases := []struct{ name, src, want string }{
		// The condition named in the original report: `[1,2] dotr 9` gave
		// `None`, while the CHECK-mode diagnostic for the same index
		// already said index_out_of_range. Two spellings for one condition,
		// one of them empty.
		{"dotr past the end", `[1,2] dotr 9`, "index_out_of_range"},
		{"getr past the end", `[1,2] getr 9`, "index_out_of_range"},
		// A strict read whose CONTAINER is the wrong shape. Its own
		// check-mode mirror was already coded; the runtime path was not.
		{"getr a list with a string key", `[1,2] getr "k"`, "getr_error"},
		{"missing file", `import "aql:io"
IO.read (make Pathon "` + missing + `")`, "read_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatal(err)
			}
			if got := codeOf(t, a, c.src); got != c.want {
				t.Errorf("code = %q, want %q", got, c.want)
			}
		})
	}
}

func inlinePolicy(t *testing.T, src string) policy.Policy {
	t.Helper()
	pol, err := policy.LoadInline(src)
	if err != nil {
		t.Fatal(err)
	}
	return pol
}

// denyFileops refuses every file operation by RULE — the scope is
// installed, the answer is no.
func denyFileops(t *testing.T) policy.Policy {
	t.Helper()
	return inlinePolicy(t, `{
		name: "no-files"
		scopes: { fileops: { words: { default: "deny" } } }
	}`)
}

// uninstallFileops removes the capability instead of refusing a call.
// It is a DIFFERENT answer with a different remedy — install the
// capability, rather than widen a rule — and it is why fileOpError
// carries two denial arms rather than collapsing them into one.
func uninstallFileops(t *testing.T) policy.Policy {
	t.Helper()
	return inlinePolicy(t, `{
		name: "no-fileops-at-all"
		scopes: { fileops: { install: false } }
	}`)
}

// A policy refusal must keep ITS OWN code rather than inheriting the
// word's. The distinction is the whole point: `read_error` says fix the
// file, `permission_denied` says change the policy, and a handler that
// cannot tell them apart cannot do either.
func TestPolicyRefusalKeepsItsOwnCode(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	read := `import "aql:io"
IO.read (make Pathon "` + real + `")`
	write := `import "aql:io"
IO.write (make Pathon "` + filepath.Join(dir, "out.txt") + `") "x"`

	cases := []struct {
		name, src, want string
		pol             func(*testing.T) policy.Policy
	}{
		{"read refused by rule", read, "permission_denied", denyFileops},
		{"write refused by rule", write, "permission_denied", denyFileops},
		{"read with fileops uninstalled", read, "capability_not_installed", uninstallFileops},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New(lang.Options{Policy: c.pol(t)})
			if err != nil {
				t.Fatal(err)
			}
			if got := codeOf(t, a, c.src); got != c.want {
				t.Errorf("code = %q, want %q — a refusal that reports the "+
					"word's own code sends the author to look at the file "+
					"when the answer is the policy", got, c.want)
			}
		})
	}
}

// The negative half, and the one that keeps the rule meaningful: the two
// codes must not collapse into each other. A helper that stamped
// permission_denied on every I/O failure would pass the test above and be
// just as wrong.
func TestOrdinaryIOFailureIsNotReportedAsAPolicyRefusal(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	src := `import "aql:io"
IO.read (make Pathon "` + filepath.Join(t.TempDir(), "nope.txt") + `")`
	if got := codeOf(t, a, src); got == "permission_denied" {
		t.Error("a missing file is not a policy refusal — no policy is even " +
			"installed here")
	}
}

// And the other direction: an ALLOWED file operation must still work.
// A refusal rule that over-fires breaks working programs, which is worse
// than the missing code it was written to fix.
func TestPermittedFileOpsStillSucceed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.txt")
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Run(`import "aql:io"
IO.write (make Pathon "` + path + `") "hello" drop
IO.read (make Pathon "` + path + `")`)
	if err != nil {
		t.Fatalf("an unrestricted write+read must succeed: %v", err)
	}
	if want := "[hello]"; fmt.Sprint(got) != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// TestEveryGatedCapabilityRefusalCarriesACode is the general half of what
// fileops got first, and the reason it is a table over all five capabilities
// rather than five separate tests is that the gap was a COUNT: the fileops
// adapter was written, the note recorded "the other capabilities' refusals
// are still code-less", and nothing in the suite would have noticed if a
// sixth capability shipped the same way.
//
// Both answers are asserted for each scope, because they are different
// answers with different remedies — a rule said no (widen the rule) versus
// the capability is not installed (install it) — and a classifier that
// collapsed them would satisfy a single-code assertion while telling every
// user the wrong thing.
//
// The words are reached without any host backend registered on purpose: the
// gate runs BEFORE the backend lookup, so the refusal is observable with no
// network, no terminal and no vault present. That is also what makes the
// negative half below meaningful.
func TestEveryGatedCapabilityRefusalCarriesACode(t *testing.T) {
	const fetchSrc = `import "aql:net"
Net.fetch {url:"http://127.0.0.1:1/x"}`
	const vaultSrc = `import "aql:vault"
Vault.status`
	const tuiSrc = `import "aql:tui"
Tui.open {}`

	cases := []struct{ name, pol, src, want string }{
		{"network refused by rule",
			`{name:"n" scopes:{network:{words:{default:"deny"}}}}`,
			fetchSrc, "permission_denied"},
		{"network uninstalled",
			`{name:"n" scopes:{network:{install:false}}}`,
			fetchSrc, "capability_not_installed"},
		{"process refused by rule",
			`{name:"p" scopes:{process:{words:{default:"deny"}}}}`,
			`spawn [1]`, "permission_denied"},
		{"process uninstalled",
			`{name:"p" scopes:{process:{install:false}}}`,
			`spawn [1]`, "capability_not_installed"},
		{"vault refused by rule",
			`{name:"v" scopes:{vault:{words:{default:"deny"}}}}`,
			vaultSrc, "permission_denied"},
		{"vault uninstalled",
			`{name:"v" scopes:{vault:{install:false}}}`,
			vaultSrc, "capability_not_installed"},
		// The tui scope is named `terminal` in policy, not `tui` — an
		// unknown scope name is refused at policy-load time, so a test
		// written against the module's name would fail before it ran.
		{"terminal refused by rule",
			`{name:"t" scopes:{terminal:{words:{default:"deny"}}}}`,
			tuiSrc, "permission_denied"},
		{"terminal uninstalled",
			`{name:"t" scopes:{terminal:{install:false}}}`,
			tuiSrc, "capability_not_installed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New(lang.Options{Policy: inlinePolicy(t, c.pol)})
			if err != nil {
				t.Fatal(err)
			}
			if got := codeOf(t, a, c.src); got != c.want {
				t.Errorf("code = %q, want %q — a refusal with no code cannot "+
					"be dispatched on, so `do [...] error [dot code case [...]]` "+
					"has no arm that can fire", got, c.want)
			}
		})
	}
}

// The negative half, and the one that keeps the classifier honest: the same
// words must NOT report a refusal when nothing was refused. An adapter that
// stamped permission_denied on every failure from a gated word would pass
// every assertion above and be strictly worse than the missing code it
// replaced — it would send authors to edit a policy that is not the problem.
//
// No policy is installed here at all, so every failure below is the
// capability's own: an unreachable host, and a vault with no backend
// registered.
func TestGatedCapabilityFailuresAreNotReportedAsRefusals(t *testing.T) {
	cases := []struct{ name, src string }{
		{"unreachable host", `import "aql:net"
Net.fetch {url:"http://127.0.0.1:1/x"}`},
		{"vault with no backend", `import "aql:vault"
Vault.status`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatal(err)
			}
			switch got := codeOf(t, a, c.src); got {
			case "permission_denied", "capability_not_installed":
				t.Errorf("code = %q, but no policy is installed — nothing was "+
					"refused, and reporting a refusal points the author at a "+
					"policy that is not the cause", got)
			}
		})
	}
}
