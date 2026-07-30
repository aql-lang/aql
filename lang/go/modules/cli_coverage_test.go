package modules

import (
	_ "embed"
	"testing"
)

//go:embed cli_test.aql
var cliTestAQL string

// cliCoverAllow lists cli.aql rows that cli_test.aql cannot reach — the AQL
// analogue of //covergate:allow. Each entry is asserted to actually be
// uncovered, so the list cannot rot.
//
// It is EMPTY, and that is a finding rather than an accident. The first cut of
// this module shipped nine allowlisted rows — all of Cli.main — with the
// rationale that every arm ends in IO.exit, which `aql test` reports as a
// file-level failure that ends the file, so the first arm exercised would kill
// the suite before the second. That reasoning was wrong: `IO.exit` raises the
// reserved aql/exit, and `Assert.throws` observes a raise. (A plain
// `do […] error […]` genuinely does NOT catch an exit — that is deliberate, so
// a program cannot swallow its own termination — which is what made the wrong
// conclusion plausible.) An adversarial review reproduced the counter-example,
// and the fix was seven test cases.
//
// Keep it empty. An entry here should have to argue against a demonstrated
// technique, not merely assert that something looks unreachable.
var cliCoverAllow = map[int]string{}

// TestCliAQLCoverage runs the aql:test suite for cli under the coverage hook
// and asserts every case passes and every executable row of cli.aql is covered
// — with an EMPTY allowlist.
func TestCliAQLCoverage(t *testing.T) {
	assertAQLCoverage(t, "aql:cli", cliSource, cliTestAQL, cliCoverAllow)
}
