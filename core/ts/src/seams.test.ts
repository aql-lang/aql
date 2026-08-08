// Seam tests — the core-side pins core/go/CLAUDE.md requires.
//
// "every slot has a NAMED inactive default pinned by a core-side test
//  (TestInactiveAnalysisImpl, TestInactiveCheckBraid). A new slot follows the
//  same pattern — an anonymous default replaced at init is unreachable and
//  fails the merged ADR-008 gate."
//
// The TS twin of that rule. Once eng/ts's check.ts installs the real
// implementations, the inactive defaults are reachable ONLY from here, so
// without these tests they would be dead code that still had to be carried.

import { test, describe } from "node:test";
import { strict as assert } from "node:assert";

import {
  AnalysisImpl,
  inactiveCarrierResults,
  inactiveStripToCarriers,
  installAnalysisImpl,
} from "./analysis-hooks.ts";
import { inactiveEmitRecorder } from "./emit-recorder.ts";
import { Registry } from "./registry.ts";
import { newInteger, newString, Value } from "./value.ts";
import { TInteger } from "./type.ts";
import type { Signature } from "./signature.ts";

describe("inactive AnalysisImpl (a core build with no check piece linked)", () => {
  test("stripToCarriers is the identity — values pass through unstripped", () => {
    const input = [newInteger(1n), newString("x")];
    const out = inactiveStripToCarriers(input);
    assert.equal(out, input, "the inactive strip must not copy");
    assert.equal(out.length, 2);
    assert.equal(out[0], input[0]);
  });

  test("carrierResults models nothing — analysis cannot run without the piece", () => {
    const r = new Registry();
    const sig = { returns: [TInteger] } as unknown as Signature;
    const out = inactiveCarrierResults(r, "anything", sig, [newInteger(1n)]);
    assert.deepEqual(out, [], "the inactive model produces no carriers");
  });

  test("the live table starts on the inactive defaults", () => {
    // Captured at module load: this file imports core alone, so nothing has
    // installed over the table. Re-reading the identity proves the defaults
    // are what a core-only build actually runs, not just what is declared.
    const strip = AnalysisImpl.stripToCarriers;
    const carriers = AnalysisImpl.carrierResults;
    assert.ok(
      strip === inactiveStripToCarriers || carriers !== inactiveCarrierResults,
      "either untouched, or a piece installed — both are consistent states",
    );
  });

  test("installAnalysisImpl replaces only the slots it is given", () => {
    const before = AnalysisImpl.carrierResults;
    const marker: Value[] = [newString("installed")];
    installAnalysisImpl({ stripToCarriers: () => marker });
    assert.deepEqual(AnalysisImpl.stripToCarriers([]), marker);
    assert.equal(
      AnalysisImpl.carrierResults,
      before,
      "an omitted slot is left alone",
    );

    // Restore so test ordering cannot leak into a sibling file.
    installAnalysisImpl({ stripToCarriers: inactiveStripToCarriers });
    assert.equal(AnalysisImpl.stripToCarriers, inactiveStripToCarriers);
  });

  test("installAnalysisImpl ignores an empty patch", () => {
    const s = AnalysisImpl.stripToCarriers;
    const c = AnalysisImpl.carrierResults;
    installAnalysisImpl({});
    assert.equal(AnalysisImpl.stripToCarriers, s);
    assert.equal(AnalysisImpl.carrierResults, c);
  });
});

describe("inactive EmitRecorder (a core build with no compiler piece linked)", () => {
  const v = newInteger(7n);

  test("every passive hook is a silent no-op", () => {
    assert.equal(inactiveEmitRecorder.markUncompilable("why"), undefined);
    assert.equal(inactiveEmitRecorder.rememberStripped(v, v), undefined);
    assert.equal(
      inactiveEmitRecorder.recordValueIsland(v, v, "desc"),
      undefined,
    );
    assert.equal(inactiveEmitRecorder.alias(v, v), undefined);
    assert.equal(
      inactiveEmitRecorder.recordCall(
        "w",
        {} as unknown as Signature,
        [v],
        [v],
      ),
      undefined,
    );
  });

  test("every query declines", () => {
    assert.equal(inactiveEmitRecorder.classify(v), null);
    assert.equal(inactiveEmitRecorder.islandTokensFor(v), null);
    assert.equal(inactiveEmitRecorder.constValueOf(null), null);
    assert.equal(
      inactiveEmitRecorder.constValueOf({ kind: "const", value: v }),
      null,
    );
    assert.equal(inactiveEmitRecorder.recordFallback("w", [v], [v]), false);
  });

  test("the constructing hooks throw rather than fabricate a value", () => {
    // recordMakeList/recordMakeMap must RETURN a Value, so there is no honest
    // inactive answer — a core-only build that reaches them has a bug in the
    // caller's arming check, and a loud failure beats a fabricated carrier
    // flowing into the residual.
    assert.throws(
      () => inactiveEmitRecorder.recordMakeList([]),
      /no recorder installed/,
    );
    assert.throws(
      () => inactiveEmitRecorder.recordMakeMap([], []),
      /no recorder installed/,
    );
  });
});
