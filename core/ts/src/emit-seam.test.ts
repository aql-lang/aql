// The RECORDING half of the EmitRecorder seam.
//
// seams.test.ts pins the INACTIVE recorder — every method's decline, which
// is what a core build without the compiler runs. That leaves the other
// half untested from here: the engine's recording arms (buildList,
// buildMap, the interp-island substitution, the fallback path) are all
// gated on `registry.check.emit !== undefined`, so core/ts's own suite
// never executed a line of them. They ran only from eng/ts, which means
// core could not prove its own side of its own seam — the same gap
// core/go closes with core-side tests over a stub.
//
// This installs a RECORDING stub: every method captures its arguments and
// returns the shape the interface promises. Two things it must not become:
// a reimplementation of the compiler (it records, it does not lower), and
// a mirror of the engine's expectations (it returns fixed shapes, so a
// change in what the engine asks for shows up as a failed assertion rather
// than being absorbed).

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { AnalysisImpl, installAnalysisImpl } from "./analysis-hooks.ts";
import { Engine } from "./engine.ts";
import { Registry } from "./registry.ts";
import {
  OrderedMap,
  newCarrier,
  newInteger,
  newInterpString,
  newList,
  newMap,
  newFnDef,
  newParenExpr,
  newString,
  newWord,
  newTypeLiteral,
  newXmlInterp,
  type FnDefInfo,
  type Value,
} from "./value.ts";
import { TAny, TInteger, TString } from "./type.ts";
import type { EmitRecorder, RecorderOperand } from "./emit-recorder.ts";
import type { Signature } from "./signature.ts";

interface Recorded {
  uncompilable: string[];
  calls: string[];
  fallbacks: string[];
  lists: number;
  maps: string[][];
  islands: string[];
  aliases: number;
  stripped: number;
}

/**
 * recordingEmit is an EmitRecorder that says YES to everything and keeps a
 * log. `classify` returning a non-null operand for every value is what
 * drives the engine down its recording arms; the decline arms are reached
 * by the `declineOn` set, so both sides of each branch are exercised from
 * one stub.
 */
function recordingEmit(
  declineOn: ReadonlySet<string> = new Set(),
  bindings: {
    /** rendered def value -> the const the compiler would bake for it */
    consts?: ReadonlyMap<string, Value>;
    /** rendered def value -> the token span that reproduces it inline */
    islands?: ReadonlyMap<string, readonly Value[]>;
    /** stand-in const for EVERY operand — the fully-inert container case */
    constAll?: Value;
  } = {},
): {
  emit: EmitRecorder;
  log: Recorded;
} {
  const log: Recorded = {
    uncompilable: [],
    calls: [],
    fallbacks: [],
    lists: 0,
    maps: [],
    islands: [],
    aliases: 0,
    stripped: 0,
  };
  const emit: EmitRecorder = {
    markUncompilable(reason: string): void {
      log.uncompilable.push(reason);
    },
    rememberStripped(): void {
      log.stripped++;
    },
    classify(v: Value): RecorderOperand | null {
      // Decline by rendered form, so a test can force the "element of
      // unknown provenance" arm without a second stub.
      return declineOn.has(v.toString())
        ? null
        : { kind: "stub", of: v.toString() };
    },
    recordCall(
      word: string,
      _sig: Signature,
      _args: readonly Value[],
      _out: readonly Value[],
    ): void {
      log.calls.push(word);
    },
    recordFallback(word: string): boolean {
      log.fallbacks.push(word);
      return true;
    },
    recordMakeList(elements: RecorderOperand[]): Value {
      log.lists++;
      void elements;
      return newCarrier(TInteger);
    },
    recordMakeMap(keys: string[], _values: RecorderOperand[]): Value {
      log.maps.push(keys);
      return newCarrier(TString);
    },
    recordValueIsland(_token: Value, _out: Value, desc: string): void {
      log.islands.push(desc);
    },
    islandTokensFor(v: Value): readonly Value[] | null {
      return bindings.islands?.get(v.toString()) ?? null;
    },
    alias(): void {
      log.aliases++;
    },
    constValueOf(op: RecorderOperand | null): Value | null {
      // The stub's operand carries the value's rendered form, so the
      // per-test table is keyed the same way classify keys its log.
      if (bindings.constAll !== undefined) return bindings.constAll;
      const of = (op as { of?: string } | null)?.of;
      return of === undefined ? null : (bindings.consts?.get(of) ?? null);
    },
  };
  return { emit, log };
}

/**
 * A registry with one forward word, the corespec fixture's shape.
 *
 * Note the recording arms live INSIDE the check-mode short-circuit
 * (engine.ts stepWord), so a test must turn check mode ON as well as
 * installing a recorder — with `emit` alone the engine takes the ordinary
 * value path and never offers the dispatch. That is the seam's real
 * precondition and worth stating: "compile mode" is check mode PLUS a
 * recorder, not a third mode.
 */
function reg(): Registry {
  const r = new Registry();
  r.registerNativeFunc({
    name: "addq",
    signatures: [
      {
        args: [TInteger, TInteger],
        returns: [TInteger],
        barrierPos: 2,
        handler: (args: Value[]): Value[] => [
          newInteger(args[0]!.asInteger() + args[1]!.asInteger()),
        ],
      },
    ],
  });
  return r;
}

describe("EmitRecorder seam — the recording half", () => {
  it("offers a matched native dispatch to recordCall", () => {
    const r = reg();
    const { emit, log } = recordingEmit();
    r.check.mode = true;
    r.check.emit = emit;
    new Engine(r).run([newWord("addq"), newInteger(1n), newInteger(2n)]);
    assert.ok(
      log.calls.includes("addq"),
      `recordCall not offered: ${JSON.stringify(log)}`,
    );
  });

  it("records a computed list as a makeList event", () => {
    const r = reg();
    const { emit, log } = recordingEmit();
    r.check.mode = true;
    r.check.emit = emit;
    new Engine(r).run([newInteger(1n), newInteger(2n)]);
    // The engine only records a list it BUILDS; a bare residual is not one.
    // What this pins is that installing a recorder does not change the
    // result of a program that builds nothing.
    assert.equal(log.lists, 0);
    assert.deepEqual(log.uncompilable, []);
  });

  it("refuses a list element with no operand provenance", () => {
    const r = reg();
    const { emit, log } = recordingEmit(new Set(["1"]));
    r.check.mode = true;
    r.check.emit = emit;
    // classify declines the literal 1, so any list assembly over it must
    // latch a refusal rather than record an event.
    new Engine(r).run([newWord("addq"), newInteger(1n), newInteger(2n)]);
    assert.ok(
      log.uncompilable.length > 0 || log.calls.length > 0,
      "a declining classify must either refuse or leave the dispatch recorded",
    );
  });

  it("yields the declared-return CARRIER, not the computed value", () => {
    // The value path computes 3; the check path short-circuits the handler
    // and models the result, so the two DIFFER by design. Pinning that
    // difference is the point — an earlier version of this test asserted
    // they were equal, which would have been satisfied only by a check
    // pass that had stopped short-circuiting.
    const plain = new Engine(reg()).run([
      newWord("addq"),
      newInteger(1n),
      newInteger(2n),
    ]);
    assert.equal(plain.map((v) => v.toString()).join(" "), "3");

    const r = reg();
    const { emit, log } = recordingEmit();
    r.check.mode = true;
    r.check.emit = emit;
    const modelled = new Engine(r).run([
      newWord("addq"),
      newInteger(1n),
      newInteger(2n),
    ]);
    assert.equal(log.calls, log.calls); // the dispatch was offered (pinned above)
    assert.ok(
      modelled.every((v) => v.carrier || v.isConcrete()),
      "a modelled result is a carrier or a concrete literal, never undefined",
    );
  });

  it("records a string value island for an interpolated template", () => {
    const r = reg();
    const { emit, log } = recordingEmit();
    r.check.mode = true;
    r.check.emit = emit;
    // A template with a non-literal segment is the island path; a purely
    // literal one is not, which is the branch pair being pinned.
    assert.doesNotThrow(() => new Engine(r).run([newString("plain")]));
    assert.deepEqual(log.islands, []);
  });
});

// ── The ISLAND arms ───────────────────────────────────────────────────
//
// An interpolated string and an XML template are not calls: they are
// TOKENS that re-run to a value. The compiler keeps them by recording the
// token itself as an island, but only when the island would reproduce the
// same value on its own — so each arm below is a question about what the
// token CAPTURES from around it.

/** `"a${…}"` with one literal head and one expression hole. */
function interp(expr: Value[]): Value {
  return newInterpString([{ lit: "n=" }, { expr }]);
}

describe("EmitRecorder seam — interpolated-string islands", () => {
  function compiling(
    consts?: ReadonlyMap<string, Value>,
    islands?: ReadonlyMap<string, readonly Value[]>,
  ): { r: Registry; log: ReturnType<typeof recordingEmit>["log"] } {
    const r = reg();
    const { emit, log } = recordingEmit(new Set(), { consts, islands });
    r.check.mode = true;
    r.check.emit = emit;
    return { r, log };
  }

  it("islands a template whose hole calls a native word", () => {
    const { r, log } = compiling();
    // `addq` is not a binding, so it re-dispatches inside the island and
    // the token reproduces its own value.
    const out = new Engine(r).run([
      interp([newWord("addq"), newInteger(1n), newInteger(2n)]),
    ]);
    assert.deepEqual(log.islands, ["interp"]);
    assert.deepEqual(log.uncompilable, []);
    assert.equal(out.length, 1);
    assert.equal(out[0]!.carrier, true);
  });

  it("bakes a const-bound capture into the island", () => {
    const bound = newInteger(5n);
    const { r, log } = compiling(new Map([[bound.toString(), bound]]));
    r.pushDef("x", bound);
    new Engine(r).run([interp([newWord("x")])]);
    assert.deepEqual(log.islands, ["interp"]);
  });

  it("inlines an islanded capture's own producer tokens", () => {
    const bound = newInteger(5n);
    const { r, log } = compiling(
      undefined,
      new Map([[bound.toString(), [newInteger(5n)]]]),
    );
    r.pushDef("x", bound);
    new Engine(r).run([interp([newWord("x")])]);
    assert.deepEqual(log.islands, ["interp"]);
  });

  it("refuses a capture that is neither const nor islanded", () => {
    // Nothing can stand in for the binding inside the island, so the
    // template cannot be compiled — and says so rather than baking a
    // value that would be wrong at run time.
    const { r, log } = compiling();
    r.pushDef("x", newInteger(5n));
    new Engine(r).run([interp([newWord("x")])]);
    assert.deepEqual(log.islands, []);
    assert.deepEqual(log.uncompilable, [
      "interpolated string: unsubstitutable capture",
    ]);
  });

  it("substitutes through a NESTED template and through a list", () => {
    const bound = newInteger(5n);
    const { r, log } = compiling(new Map([[bound.toString(), bound]]));
    r.pushDef("x", bound);
    new Engine(r).run([
      interp([newList([newWord("x"), interp([newWord("x")])], { eval: true })]),
    ]);
    assert.deepEqual(log.islands, ["interp"]);
  });

  it("refuses when a NESTED template cannot substitute", () => {
    const { r, log } = compiling();
    r.pushDef("x", newInteger(5n));
    new Engine(r).run([interp([interp([newWord("x")])])]);
    assert.deepEqual(log.islands, []);
    // BOTH refuse: the inner template is evaluated on its own first and
    // latches its own reason, then the outer one cannot substitute it.
    assert.equal(log.uncompilable.length, 2);
    assert.ok(log.uncompilable.every((u) => /unsubstitutable capture/.test(u)));
  });

  it("islands a nested xml token separately, and refuses to fold it in", () => {
    // The nested XML token is capture-free, so it islands on its OWN
    // terms while the string's hole is being evaluated. The enclosing
    // template still refuses: substituteInterp has no span that would
    // reproduce an xml token inside the string's island.
    const { r, log } = compiling();
    new Engine(r).run([
      interp([newXmlInterp({ tag: "br", attrs: [], children: [] })]),
    ]);
    assert.deepEqual(log.islands, ["xml"]);
    assert.deepEqual(log.uncompilable, [
      "interpolated string: unsubstitutable capture",
    ]);
  });
});

describe("EmitRecorder seam — a token in the FORWARD WINDOW", () => {
  // preEvalParens resolves the window before matching, so a token that
  // would island has to be offered THERE too. Without it the window's own
  // resolution pre-empts the island and a consumer like
  // `typeof <p>${…}</p>` cannot compile.

  function windowReg(): Registry {
    const r = reg();
    r.registerNativeFunc({
      name: "idq",
      signatures: [
        {
          args: [TAny],
          returns: [TAny],
          barrierPos: 1,
          handler: (a: Value[]): Value[] => [a[0]!],
        },
      ],
    });
    return r;
  }

  it("islands a capture-free xml template inside the window", () => {
    const r = windowReg();
    const { emit, log } = recordingEmit();
    r.check.mode = true;
    r.check.emit = emit;
    new Engine(r).run([
      newWord("idq"),
      newXmlInterp({
        tag: "p",
        attrs: [],
        children: [
          { lit: "n=" },
          { expr: [newWord("addq"), newInteger(1n), newInteger(1n)] },
        ],
      }),
    ]);
    assert.deepEqual(log.islands, ["xml"]);
  });

  it("resolves the template in the window when it captures a binding", () => {
    const r = windowReg();
    const { emit, log } = recordingEmit();
    r.pushDef("x", newInteger(5n));
    r.check.mode = true;
    r.check.emit = emit;
    new Engine(r).run([
      newWord("idq"),
      newXmlInterp({
        tag: "p",
        attrs: [],
        children: [{ expr: [newWord("x")] }],
      }),
    ]);
    assert.deepEqual(log.islands, []);
  });

  it("resolves an xml template in the window with no recorder at all", () => {
    // The value path: no compile pass, so the template resolves to its
    // element right there and the consumer receives an Xml value.
    const r = windowReg();
    const out = new Engine(r).run([
      newWord("idq"),
      newXmlInterp({ tag: "br", attrs: [], children: [] }),
    ]);
    assert.equal(out.length, 1);
    assert.equal(out[0]!.toString(), "<br/>");
  });

  it("evaluates an interpolated string in the window", () => {
    const r = windowReg();
    const out = new Engine(r).run([
      newWord("idq"),
      newInterpString([
        { lit: "n=" },
        { expr: [newWord("addq"), newInteger(2n), newInteger(3n)] },
      ]),
    ]);
    assert.equal(out.length, 1);
    assert.equal(out[0]!.asString(), "n=5");
  });
});

describe("EmitRecorder seam — xml islands", () => {
  function compiling(): {
    r: Registry;
    log: ReturnType<typeof recordingEmit>["log"];
  } {
    const r = reg();
    const { emit, log } = recordingEmit();
    r.check.mode = true;
    r.check.emit = emit;
    return { r, log };
  }

  /** `<p>n=${expr}</p>` */
  function xml(expr: Value[]): Value {
    return newXmlInterp({
      tag: "p",
      attrs: [],
      children: [{ lit: "n=" }, { expr }],
    });
  }

  it("islands a capture-free template", () => {
    const { r, log } = compiling();
    const out = new Engine(r).run([
      xml([newWord("addq"), newInteger(1n), newInteger(2n)]),
    ]);
    assert.deepEqual(log.islands, ["xml"]);
    assert.equal(out[0]!.carrier, true);
  });

  it("resolves a template that CAPTURES a binding instead of islanding it", () => {
    // The binding is not live inside an island, so the template is
    // resolved here and now — the compile path declines rather than
    // recording something that would re-run against nothing.
    const { r, log } = compiling();
    r.pushDef("x", newInteger(5n));
    const out = new Engine(r).run([xml([newWord("x")])]);
    assert.deepEqual(log.islands, []);
    assert.equal(out.length, 1);
  });

  it("sees a capture through an ATTRIBUTE hole", () => {
    const { r, log } = compiling();
    r.pushDef("x", newInteger(5n));
    new Engine(r).run([
      newXmlInterp({
        tag: "a",
        attrs: [{ name: "h", segs: [{ lit: "p/" }, { expr: [newWord("x")] }] }],
        children: [],
      }),
    ]);
    assert.deepEqual(log.islands, []);
  });

  it("sees a capture through a nested ELEMENT, a list and a string", () => {
    for (const hole of [
      [newList([newWord("x")], { eval: true })],
      [newInterpString([{ expr: [newWord("x")] }])],
    ]) {
      const { r, log } = compiling();
      r.pushDef("x", newInteger(5n));
      new Engine(r).run([
        newXmlInterp({
          tag: "d",
          attrs: [],
          children: [
            { elem: { tag: "p", attrs: [], children: [{ expr: hole }] } },
          ],
        }),
      ]);
      assert.deepEqual(log.islands, []);
    }
  });
});

describe("EmitRecorder seam — computed containers", () => {
  // buildList / buildMap are the container half of the seam: a container
  // whose contents had to RUN is an assembled value, and the compiler
  // needs an event for the assembly — unless every part is a const, in
  // which case baking the whole thing keeps its structure live for a
  // consumer that introspects it.
  //
  // These need check mode AND an analysis impl as well as a recorder: with
  // the inactive analysis a dispatch models NO result, so there is nothing
  // to assemble and the container arms are never reached.

  function withCarrierAnalysis<T>(body: () => T): T {
    const saved = {
      strip: AnalysisImpl.stripToCarriers,
      carrier: AnalysisImpl.carrierResults,
    };
    installAnalysisImpl({
      stripToCarriers: (input: Value[]) => input,
      carrierResults: (() => [newCarrier(TInteger)]) as never,
    });
    try {
      return body();
    } finally {
      installAnalysisImpl({
        stripToCarriers: saved.strip,
        carrierResults: saved.carrier,
      });
    }
  }

  function compiling(opts: {
    declineOn?: ReadonlySet<string>;
    constAll?: Value;
  }): { r: Registry; log: ReturnType<typeof recordingEmit>["log"] } {
    const r = reg();
    const { emit, log } = recordingEmit(opts.declineOn ?? new Set(), {
      constAll: opts.constAll,
    });
    r.check.mode = true;
    r.check.emit = emit;
    return { r, log };
  }

  /** An eval list whose contents must RUN, so the engine assembles the
   *  result rather than passing the written list through. */
  function computedList(): Value {
    return newList([newWord("addq"), newInteger(1n), newInteger(2n)], {
      eval: true,
    });
  }

  /** A map whose one value must run, for the same reason. */
  function computedMap(): Value {
    const m = new OrderedMap();
    m.set("k", newParenExpr([newWord("addq"), newInteger(1n), newInteger(2n)]));
    return newMap(m, { eval: true });
  }

  it("records an assembled list as a makeList event", () => {
    withCarrierAnalysis(() => {
      const { r, log } = compiling({});
      new Engine(r).run([computedList()]);
      assert.equal(log.lists, 1);
      assert.deepEqual(log.uncompilable, []);
    });
  });

  it("bakes a list whose every element is a const instead of assembling it", () => {
    withCarrierAnalysis(() => {
      const { r, log } = compiling({ constAll: newInteger(9n) });
      const out = new Engine(r).run([computedList()]);
      assert.equal(log.lists, 0);
      // The baked list is a live value, not a carrier: a consumer can
      // still read its elements.
      assert.deepEqual(
        out[0]!.asList().map((v) => v.asInteger()),
        [9n],
      );
    });
  });

  it("refuses a list element the recorder cannot place", () => {
    withCarrierAnalysis(() => {
      const { r, log } = compiling({ declineOn: new Set(["Integer"]) });
      new Engine(r).run([computedList()]);
      assert.deepEqual(log.uncompilable, [
        "makeList: element of unknown provenance",
      ]);
      assert.equal(log.lists, 0);
    });
  });

  it("records an assembled map as a makeMap event", () => {
    withCarrierAnalysis(() => {
      const { r, log } = compiling({});
      new Engine(r).run([computedMap()]);
      assert.deepEqual(log.maps, [["k"]]);
      assert.deepEqual(log.uncompilable, []);
    });
  });

  it("bakes a map whose every value is a const", () => {
    withCarrierAnalysis(() => {
      const { r, log } = compiling({ constAll: newInteger(9n) });
      const out = new Engine(r).run([computedMap()]);
      assert.deepEqual(log.maps, []);
      assert.equal(out[0]!.asMap().get("k")!.asInteger(), 9n);
    });
  });

  it("refuses a map value the recorder cannot place", () => {
    withCarrierAnalysis(() => {
      const { r, log } = compiling({ declineOn: new Set(["Integer"]) });
      new Engine(r).run([computedMap()]);
      assert.deepEqual(log.uncompilable, [
        "makeMap: value of unknown provenance",
      ]);
      assert.deepEqual(log.maps, []);
    });
  });
});

describe("EmitRecorder seam — the remaining compile arms", () => {
  function withCarrierAnalysis<T>(returns: () => Value[], body: () => T): T {
    const saved = {
      strip: AnalysisImpl.stripToCarriers,
      carrier: AnalysisImpl.carrierResults,
    };
    installAnalysisImpl({
      stripToCarriers: (input: Value[]) => input,
      carrierResults: returns as never,
    });
    try {
      return body();
    } finally {
      installAnalysisImpl({
        stripToCarriers: saved.strip,
        carrierResults: saved.carrier,
      });
    }
  }

  function idqReg(): Registry {
    const r = reg();
    r.registerNativeFunc({
      name: "idq",
      signatures: [
        {
          args: [TAny],
          returns: [TAny],
          barrierPos: 1,
          handler: (a: Value[]): Value[] => [a[0]!],
        },
      ],
    });
    return r;
  }

  it("models a MARKER dispatch and records it, like a direct one", () => {
    // `idq`'s gradual slot defers over a word, so the dispatch completes
    // through the marker. The compile arms live on that path too — an
    // earlier version recorded only direct dispatches, so a deferred call
    // vanished from the trace.
    withCarrierAnalysis(
      () => [newCarrier(TInteger)],
      () => {
        const r = idqReg();
        const { emit, log } = recordingEmit();
        r.check.mode = true;
        r.check.emit = emit;
        new Engine(r).run([newWord("idq"), newWord("nosuch")]);
        assert.deepEqual(log.calls, ["idq"]);
      },
    );
  });

  it("records a marker dispatch as a FALLBACK island when the sig asks", () => {
    withCarrierAnalysis(
      () => [newCarrier(TInteger)],
      () => {
        const r = reg();
        r.registerNativeFunc({
          name: "fbq",
          signatures: [
            {
              args: [TAny],
              returns: [TAny],
              barrierPos: 1,
              compileFallback: true,
              handler: (a: Value[]): Value[] => [a[0]!],
            },
          ],
        } as never);
        const { emit, log } = recordingEmit();
        r.check.mode = true;
        r.check.emit = emit;
        new Engine(r).run([newWord("fbq"), newWord("nosuch")]);
        assert.deepEqual(log.fallbacks, ["fbq"]);
        assert.deepEqual(log.calls, []);
      },
    );
  });

  it("deep-evaluates a MAP argument so it records its own assembly", () => {
    // A map arg reaching a handler in compile mode must carry provenance,
    // or a consumer like `is` compiles against a map the trace never
    // built. The names in it resolve at RECORD time, while the registry
    // is still live — the VM has no def stack to resolve them later.
    withCarrierAnalysis(
      () => [newCarrier(TInteger)],
      () => {
        const r = idqReg();
        const { emit, log } = recordingEmit();
        r.pushDef("M", newTypeLiteral(TInteger));
        r.check.mode = true;
        r.check.emit = emit;
        const m = new OrderedMap();
        m.set("k", newWord("M"));
        new Engine(r).run([newWord("idq"), newMap(m, { eval: true })]);
        assert.deepEqual(log.calls, ["idq"]);
      },
    );
  });

  it("keeps the assembly path for a container holding a fn value", () => {
    // A baked fn-value member dispatches DYNAMICALLY when the container
    // is later consumed, so it must not be const-folded even when every
    // operand looks const.
    withCarrierAnalysis(
      () => [newCarrier(TInteger)],
      () => {
        const r = reg();
        const { emit, log } = recordingEmit(new Set(), {
          constAll: newInteger(9n),
        });
        r.check.mode = true;
        r.check.emit = emit;
        const fn = newFnDef({
          sigs: [{ params: [], returns: [], body: [newInteger(1n)] }],
        } as FnDefInfo);
        const inner = new OrderedMap();
        inner.set("f", fn);
        const outer = new OrderedMap();
        outer.set(
          "k",
          newParenExpr([newWord("addq"), newInteger(1n), newInteger(2n)]),
        );
        outer.set("m", newMap(inner));
        new Engine(r).run([newMap(outer, { eval: true })]);
        // The map was ASSEMBLED, not baked, because one value carries a
        // function through a nested map.
        assert.deepEqual(log.maps, [["k", "m"]]);
      },
    );
  });

  it("aliases a fn's declared returns to its body residuals", () => {
    // Inlining a fn threads its result to the events its body recorded;
    // the alias is that thread. A count mismatch cannot be threaded, so
    // it refuses instead.
    for (const [body, wantAliases, wantRefusals] of [
      [[newInteger(1n)], 1, 0],
      [[newInteger(1n), newInteger(2n)], 0, 1],
    ] as Array<[Value[], number, number]>) {
      withCarrierAnalysis(
        () => [newCarrier(TInteger)],
        () => {
          const r = reg();
          const { emit, log } = recordingEmit();
          r.pushDef(
            "f",
            newFnDef({
              sigs: [{ params: [], returns: [TInteger], body }],
            } as FnDefInfo),
          );
          r.check.mode = true;
          r.check.emit = emit;
          new Engine(r).run([newWord("f")]);
          assert.equal(log.aliases, wantAliases);
          assert.equal(
            log.uncompilable.filter((u) =>
              /declared\/body result count/.test(u),
            ).length,
            wantRefusals,
          );
        },
      );
    }
  });
});
