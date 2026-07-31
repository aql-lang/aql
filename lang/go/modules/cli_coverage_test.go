package modules

import (
	_ "embed"
	"testing"
)

//go:embed cli_test.boru
var cliTestBORU string

// cliCoverAllow lists cli.boru rows that cli_test.boru cannot reach — the BORU
// analogue of //covergate:allow. Each entry is asserted to actually be
// uncovered, so the list cannot rot.
//
// It is EMPTY, and that is a finding rather than an accident. The first cut of
// this module shipped nine allowlisted rows — all of Cli.main — with the
// rationale that every arm ends in IO.exit, which `boru test` reports as a
// file-level failure that ends the file, so the first arm exercised would kill
// the suite before the second. That reasoning was wrong: `IO.exit` raises the
// reserved boru/exit, and `Assert.throws` observes a raise. (A plain
// `do […] error […]` genuinely does NOT catch an exit — that is deliberate, so
// a program cannot swallow its own termination — which is what made the wrong
// conclusion plausible.) An adversarial review reproduced the counter-example,
// and the fix was seven test cases.
//
// Keep it empty. An entry here should have to argue against a demonstrated
// technique, not merely assert that something looks unreachable.
var cliCoverAllow = map[int]string{}

// TestCliBORUCoverage runs the boru:test suite for cli under the coverage hook
// and asserts every case passes and every executable row of cli.boru is covered
// — with an EMPTY allowlist.
func TestCliBORUCoverage(t *testing.T) {
	assertBORUCoverage(t, "boru:cli", cliSource, cliTestBORU, cliCoverAllow)
}
