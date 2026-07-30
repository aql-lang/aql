package native

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/policy"
)

// TestPolicyRefusalClassification pins the adapter's three answers,
// including the one that has no end-to-end route: a refusal whose code no
// capability GATE can produce.
//
// The two unmapped constants are unmapped on evidence, not oversight.
// Nothing in the tree constructs `CodeModulesDisabled` at all, and
// `CodePolicyAttenuation` is minted by child-policy composition
// (policy/subset.go), which is not a gate — so neither can arrive through
// `pol.Check`. Mapping them would add arms no program can reach; guessing a
// code for them would be worse, because the caller's own error is at least
// true. This test reaches that exit directly, which is the only way it can be
// reached, and records why.
func TestPolicyRefusalClassification(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		err      error
		wantCode string // "" = must pass through uncoded
	}{
		{"a rule said no",
			&policy.Denied{Code: policy.CodePermissionDenied, Scope: "network", Op: "connect"},
			"permission_denied"},
		{"the capability is not installed",
			&policy.Denied{Code: policy.CodeCapabilityNotInstalled, Scope: "vault", Op: "read"},
			"capability_not_installed"},
		// Not a refusal at all — the caller keeps its own error. This is the
		// arm that makes fileOpError's "read: no such file" stay a read_error
		// rather than becoming a policy story.
		{"an ordinary error", errors.New("no such file"), ""},
		// A refusal a gate cannot produce: no code is invented for it.
		{"attenuation, which no gate mints",
			&policy.Denied{Code: policy.CodePolicyAttenuation, Scope: "fileops", Op: "read"},
			""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PolicyRefusal(r, "w", c.err)
			var ae *AqlError
			coded := errors.As(got, &ae)
			switch {
			case c.wantCode == "" && coded:
				t.Errorf("must pass through uncoded, got code %q", ae.Code)
			case c.wantCode == "" && got != c.err:
				t.Errorf("a non-refusal must pass through as the SAME error value, got %v", got)
			case c.wantCode != "" && !coded:
				t.Errorf("a refusal must become an AqlError so `dot code` can read it, got %T", got)
			case c.wantCode != "" && ae.Code != c.wantCode:
				t.Errorf("code = %q, want %q", ae.Code, c.wantCode)
			}
		})
	}
}

// TestPolicyRefusalPassesNonRefusalsThrough is the wrapper's contract: it is
// safe to call on ANY gate result, which is what lets it sit inside the gate
// function rather than at each of the nine call sites. If it replaced or
// wrapped ordinary errors, every gate would have to pre-filter and the
// forgettable step would be back.
func TestPolicyRefusalPassesNonRefusalsThrough(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	if got := PolicyRefusal(r, "w", nil); got != nil {
		t.Errorf("nil must stay nil, got %v", got)
	}

	plain := errors.New("dial tcp: connection refused")
	if got := PolicyRefusal(r, "w", plain); got != plain {
		t.Errorf("a non-refusal must pass through UNCHANGED (same error value), got %v", got)
	}

	denied := &policy.Denied{
		Code: policy.CodePermissionDenied, Scope: "network", Op: "connect",
		Profile: "p", Blame: "network.words default=deny",
	}
	coded := PolicyRefusal(r, "fetch", denied)
	var ae *AqlError
	if !errors.As(coded, &ae) {
		t.Fatalf("a refusal must become an AqlError so `dot code` can read it, got %T", coded)
	}
	if ae.Code != "permission_denied" {
		t.Errorf("code = %q, want permission_denied", ae.Code)
	}
	// The blame trail must survive: the code makes the failure dispatchable,
	// the detail is what makes it diagnosable. Dropping it would leave the
	// author knowing a policy said no and nothing about which rule.
	if !strings.Contains(fmt.Sprint(coded), "network.words default=deny") {
		t.Errorf("the refusal's blame trail was dropped from the detail: %v", coded)
	}
}
