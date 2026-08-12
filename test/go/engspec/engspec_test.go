// Spec-runner test for the engine kernel — runs the shared corpus at
// boru/eng/spec/*.tsv (sibling of eng/go/ and eng/ts/, so Go and TypeScript
// ports run the same .tsv files). Each row is parsed with the boru parser
// (eng/go/parser) and run against a fresh core.Registry pre-populated with
// kernel-only spec-runner fixtures (q-suffixed plus minimal copies of
// the words eng/spec rows exercise — def, fn, dup, …). After the
// eng→lang migration eng itself ships no word registrations; engspec
// keeps its own fixtures so the kernel can still be tested in
// isolation against the same .tsv corpus.
//
// The "q" suffix on most fixtures marks them as SPEC-RUNNER FIXTURES,
// distinct from production boru words of the same root name. Language-
// fundamental keywords (def, fn, quote, args, refine, typeof,
// is, none, end, …) keep their bare names because what's being tested
// IS the keyword itself, not a fixture for it.
//
// This file lives in the test module (not eng/go) so eng/go has no
// dependency on test — the dep arrow points one way: test → eng.
package engspec

import (
	"path/filepath"
	"testing"

	basic "github.com/boru-lang/boru/basic/go"
	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/parser/go"
	"github.com/boru-lang/boru/test/specfix"
)

func TestSpec(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "eng", "spec")
	specfix.RunDir(t, specDir, func(input string) ([]core.Value, error) {
		values, err := parser.Parse(input)
		if err != nil {
			return nil, err
		}
		r, err := core.NewRegistry()
		if err != nil {
			return nil, err
		}
		specfix.RegisterSpecWords(r)
		basic.InstallMicronIdeals(r)
		r.InitRootContext()
		return core.NewTop(r).Run(values)
	})
}
