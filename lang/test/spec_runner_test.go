package test

// Spec-runner test for the production-language spec suite at
// lang/spec/. Each TSV row is parsed with the AQL parser (eng/parser)
// and run against a fresh production registry (engine.DefaultRegistry
// + native.Register) — the full language layer, so these specs can
// exercise any registered word (record / object / make / get /
// length / …) and the builtin Resource / Entity types installed by
// installResourceTypes.
//
// The kernel-only spec suite (q-suffixed fixtures, eng.RegisterCoreWords,
// specs at eng/spec/) lives in eng/go/spec_test.go — it tests the
// engine kernel in isolation. The shared TSV scaffolding (file walk,
// row parsing, ERROR-prefix handling, value rendering) lives in
// util/go/specrunner.

import (
	"path/filepath"
	"testing"

	"github.com/aql-lang/aql/eng"
	"github.com/aql-lang/aql/eng/parser"
	"github.com/aql-lang/aql/lang/engine"
	"github.com/aql-lang/aql/lang/native"
	"github.com/aql-lang/aql/util/go/specrunner"
)

// TestSpecProd runs the .tsv spec files under lang/spec/ against a
// production-aql registry (engine.DefaultRegistry + native.Register).
// These specs cover the production language layer — words and types
// that aren't part of the eng kernel (record, object, make, get/set
// on Stores, Resource / Entity, …). They sit at lang/spec/ to mirror
// the engine kernel's eng/spec/ layout.
func TestSpecProd(t *testing.T) {
	specrunner.RunDir(t, filepath.Join("..", "spec"), func(input string) ([]eng.Value, error) {
		values, err := parser.Parse(input)
		if err != nil {
			return nil, err
		}
		reg, err := engine.DefaultRegistry(native.Register)
		if err != nil {
			return nil, err
		}
		return engine.NewTop(reg).Run(values)
	})
}
