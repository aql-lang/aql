// The kernel's SMALL exported seams — the registry's capability table, the
// word-entry merge, the scalar `make` conversions and the signature/type
// helpers. Same reason as value-surface.test.ts: each has a live consumer
// in eng/ts or basic/ts but none in core/ts's own suite, so core/ts's gate
// reads them as uncovered, and an uncovered line is where a divergence
// hides.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import {
  BoruError,
  Registry,
  makeConvert,
  matchValues,
  makeUrlon,
  newAtom,
  newInteger,
  newString,
  newType,
  indexDecl,
  resolveBuiltinTypeName,
  returnsIdentity,
  TAny,
  TInteger,
  TPathon,
} from "./index.ts";

describe("registry capabilities", () => {
  it("distinguishes an absent capability from one stored as undefined", () => {
    const r = new Registry();
    assert.equal(r.hasCapability("io"), false);
    assert.deepEqual(r.capability("io"), [undefined, false]);
    // Storing undefined is a real STORE: present, value undefined. The
    // two-slot return is what keeps that distinguishable.
    r.setCapability("io", undefined);
    assert.equal(r.hasCapability("io"), true);
    assert.deepEqual(r.capability("io"), [undefined, true]);
  });

  it("lists and removes capabilities", () => {
    const r = new Registry();
    r.setCapability("io", { open: true });
    r.setCapability("clock", 0);
    assert.deepEqual(r.capabilityNames(), ["io", "clock"]);
    assert.equal(r.deleteCapability("io"), true);
    assert.equal(r.deleteCapability("io"), false);
    assert.deepEqual(r.capabilityNames(), ["clock"]);
  });
});

describe("registry word entries", () => {
  it("merging a second registration keeps forward precedence sticky", () => {
    const r = new Registry();
    const sig = {
      args: [TInteger],
      returns: [TInteger],
      barrierPos: 1,
      handler: (a: import("./index.ts").Value[]) => [a[0]!],
    };
    r.registerNativeFunc({
      name: "w",
      forwardPrecedence: true,
      signatures: [sig],
    });
    // The second registration does NOT declare it; the entry keeps the
    // flag the first one set, so a later overload cannot silently demote
    // a forward-precedence word to stack form.
    r.registerNativeFunc({
      name: "w",
      signatures: [{ ...sig, args: [TAny], returns: [TAny] }],
    });
    const entry = r.lookup("w");
    assert.equal(entry?.forwardPrecedence, true);
    assert.equal(entry?.signatures.length, 2);
  });
});

describe("scalar make", () => {
  it("refuses a URL string the parser cannot read at all", () => {
    // `new URL` throws on a relative reference; core/go reaches the same
    // refusal by url.Parse succeeding with an empty Scheme. Same code and
    // same text either way.
    assert.throws(
      () => makeUrlon(newString("notaurl")),
      (e: unknown) =>
        e instanceof BoruError &&
        e.code === "type_error" &&
        e.message.includes("Urlon requires an absolute URL"),
    );
  });

  it("refuses a URL that parses but names no host", () => {
    // `mailto:` parses — it has a protocol — and is still not absolute.
    // This is the second guard, not the catch above.
    assert.throws(
      () => makeUrlon(newString("mailto:a@b.example")),
      (e: unknown) =>
        e instanceof BoruError &&
        e.message.includes("requires an absolute URL"),
    );
  });

  it("refuses a non-string source by naming what it got", () => {
    assert.throws(
      () => makeUrlon(newInteger(1n)),
      (e: unknown) =>
        e instanceof BoruError && e.message.includes("must be a string or map"),
    );
  });

  it("converts to Pathon through the same path the make word takes", () => {
    const p = makeConvert(newString("/a/b"), TPathon);
    assert.equal(p.vType.matches(TPathon), true);
  });
});

describe("signature helpers", () => {
  it("returnsIdentity falls back to an Any carrier for an out-of-range index", () => {
    const f = returnsIdentity(0, 3, -1);
    const out = f([newInteger(7n)], undefined as never);
    assert.equal(out[0]!.asInteger(), 7n);
    // Indices past the args (and negative ones) cannot name a value, so
    // the slot becomes a type-only carrier rather than throwing.
    for (const i of [1, 2]) {
      assert.equal(out[i]!.carrier, true);
      assert.equal(out[i]!.vType.equal(TAny), true);
    }
  });
});

describe("type name indexing", () => {
  it("a leaf name claimed by two paths stops expanding as a short name", () => {
    // The leaf index maps a short name to its ONE path, and newType uses
    // it to expand `Integer` to `Scalar/Number/Integer`. A second type
    // claiming the same leaf blanks the entry, so the short name is
    // ambiguous and expands to nothing rather than to whichever path
    // registered first. A THIRD registration must leave it blanked.
    indexDecl({ path: "Scalar/KernelSurfaceDup" });
    assert.equal(
      newType("KernelSurfaceDup").toString(),
      "Scalar/KernelSurfaceDup",
    );
    indexDecl({ path: "Node/KernelSurfaceDup" });
    assert.equal(newType("KernelSurfaceDup").toString(), "KernelSurfaceDup");
    indexDecl({ path: "Ideal/KernelSurfaceDup" });
    assert.equal(newType("KernelSurfaceDup").toString(), "KernelSurfaceDup");
    // Each full path still resolves — only the short name is ambiguous.
    assert.equal(
      resolveBuiltinTypeName("Node/KernelSurfaceDup")?.toString(),
      "Node/KernelSurfaceDup",
    );
  });

  it("an alias points at its path outright, with no ambiguity rule", () => {
    indexDecl({ path: "Scalar/KernelSurfaceAliased", alias: "KSAlias" });
    assert.equal(newType("KSAlias").toString(), "Scalar/KernelSurfaceAliased");
  });

  it("an internal type is reachable by path but never by bare name", () => {
    assert.equal(resolveBuiltinTypeName("__ED"), undefined);
    assert.equal(newAtom("__ED").vType.toString(), "Scalar/Atom");
  });
});

describe("matchValues — the all-stack poly path", () => {
  // eng/ts's VM calls this for OpCallNativePoly; core/ts's own engine never
  // does, so it is another eng-only export that reads as uncovered here.
  const entry = () => {
    const r = new Registry();
    r.registerNativeFunc({
      name: "w",
      signatures: [
        {
          args: [TInteger],
          returns: [TInteger],
          barrierPos: 0,
          handler: (a) => [a[0]!],
        },
        {
          args: [TAny, TAny],
          returns: [TAny],
          barrierPos: 0,
          handler: (a) => [a[0]!],
        },
      ],
    });
    return r.lookup("w")!;
  };

  it("binds the overload whose ARITY and types both fit", () => {
    const m = matchValues(entry(), [newInteger(1n)]);
    assert.equal(m?.sig.args.length, 1);
    assert.equal(m?.forwardCount, 0);
    assert.equal(m?.prefixCount, 1);
  });

  it("skips an overload of the wrong arity rather than truncating", () => {
    const m = matchValues(entry(), [newString("x"), newString("y")]);
    assert.equal(m?.sig.args.length, 2);
  });

  it("returns null when no overload matches", () => {
    assert.equal(matchValues(entry(), [newString("x")]), null);
    assert.equal(matchValues(entry(), []), null);
  });

  it("enforces patterns on the all-stack window", () => {
    const r = new Registry();
    r.registerNativeFunc({
      name: "p",
      signatures: [
        {
          args: [TInteger],
          returns: [TInteger],
          patterns: new Map([[0, newInteger(7n)]]),
          barrierPos: 0,
          handler: (a) => [a[0]!],
        },
      ],
    });
    const fn = r.lookup("p")!;
    assert.notEqual(matchValues(fn, [newInteger(7n)]), null);
    assert.equal(matchValues(fn, [newInteger(8n)]), null);
  });
});
