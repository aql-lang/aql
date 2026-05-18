// Spec-runner test for the production-language spec suite at
// aql/lang/spec/. Each TSV row is parsed with the AQL parser
// (eng/go/parser) and run against a fresh production registry
// (engine.DefaultRegistry + native.Register) — the full language
// layer, so these specs can exercise any registered word (record /
// object / make / get / length / …) and the builtin Resource / Entity
// types installed by installResourceTypes.
//
// The kernel-only spec suite (q-suffixed fixtures, eng.RegisterCoreWords,
// specs at eng/spec/) lives next door at test/go/engspec — it tests the
// engine kernel in isolation. The shared TSV scaffolding lives in
// test/go/specrunner.
//
// Both spec tests live in the test module so neither eng nor lang has a
// dep on test — the dep arrows point one way: test → eng, test → lang.
package langspec

import (
	"path/filepath"
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/engine"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/test/go/specrunner"
)

// TestSpecProd runs the .tsv spec files under aql/lang/spec/ against a
// production-aql registry (engine.DefaultRegistry + native.Register).
// These specs cover the production language layer — words and types
// that aren't part of the eng kernel (record, object, make, get/set on
// Stores, Resource / Entity, …). They sit at lang/spec/ to mirror the
// engine kernel's eng/spec/ layout.
func TestSpecProd(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	specrunner.RunDir(t, specDir, func(input string) ([]eng.Value, error) {
		values, err := parser.Parse(input)
		if err != nil {
			return nil, err
		}
		reg, err := engine.DefaultRegistry(native.Register)
		if err != nil {
			return nil, err
		}
		// Install the shared q-suffixed spec fixtures so tsv files
		// originally written for engspec (object, record, inspect, …)
		// can run under the production setup too.
		specrunner.RegisterQFixtures(reg)
		return engine.NewTop(reg).Run(values)
	})
}
