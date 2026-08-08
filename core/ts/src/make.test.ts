// Unit campaign for make.ts — all eight exported functions, none of which
// any test called before this file existed (0.00% function coverage at
// 38.28% lines).
//
// Expectations come from the Go mirrors each function names: core_make.go's
// MakeConvert / MakeScalarHandler / MakeScalarOptsHandler / makePathon /
// makeEmailon / makeUrlon, and registry.go's ValToString. Two contracts here
// are load-bearing beyond this file:
//
//   - Boolean conversion is a COERCION, not a parse. make String 'false'
//     to Boolean is TRUE, because "false" is a non-empty string. Parsing the
//     literal words is a separate operation the kernel does not offer.
//   - The unsupported-target refusal wording ("unsupported target type <path>")
//     is shared with Go verbatim, because the crossdiff error-code comparison
//     and the eng/spec rows both read it.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { BoruError } from "./error.ts";
import {
  makeConvert,
  makeEmailon,
  makeIdealHandler,
  makePathon,
  makeScalarHandler,
  makeScalarOptsHandler,
  makeUrlon,
} from "./make.ts";
import { valToString } from "./canon.ts";
import {
  TAtom,
  TBoolean,
  TEmailon,
  TFloat,
  TInteger,
  TPathon,
  TString,
  TUrlon,
  newType,
} from "./type.ts";
import {
  OrderedMap,
  newAtom,
  newBoolean,
  newFloat,
  newInteger,
  newList,
  newMap,
  newString,
  newTypeLiteral,
  newWord,
} from "./value.ts";

const map = (entries: Array<[string, ReturnType<typeof newBoolean>]>) => {
  const m = new OrderedMap();
  for (const [k, v] of entries) m.set(k, v);
  return newMap(m);
};

describe("valToString", () => {
  it("renders each concrete scalar as its text", () => {
    assert.equal(valToString(newString("hi")), "hi");
    assert.equal(valToString(newAtom("a")), "a");
    assert.equal(valToString(newInteger(42n)), "42");
    assert.equal(valToString(newFloat(1.5)), "1.5");
    assert.equal(valToString(newBoolean(true)), "true");
    assert.equal(valToString(newBoolean(false)), "false");
    assert.equal(valToString(newWord("w")), "w");
  });

  it("falls back to toString for a non-concrete value", () => {
    assert.equal(valToString(newTypeLiteral(TInteger)), "Integer");
  });
});

describe("makeConvert", () => {
  it("converts to String via valToString", () => {
    assert.equal(makeConvert(newInteger(7n), TString).asString(), "7");
    assert.equal(makeConvert(newBoolean(false), TString).asString(), "false");
  });

  it("converts to Float, rejecting non-numeric text", () => {
    assert.equal(makeConvert(newString("1.5"), TFloat).asFloat(), 1.5);
    assert.throws(() => makeConvert(newString("nope"), TFloat), BoruError);
    assert.throws(() => makeConvert(newString("   "), TFloat), BoruError);
  });

  it("converts to Integer, truncating a float and rejecting junk", () => {
    assert.equal(makeConvert(newString("7"), TInteger).asInteger(), 7n);
    assert.equal(makeConvert(newString("-7"), TInteger).asInteger(), -7n);
    assert.equal(makeConvert(newFloat(2.9), TInteger).asInteger(), 2n);
    assert.throws(() => makeConvert(newString("nope"), TInteger), BoruError);
  });

  it("COERCES to Boolean rather than parsing it", () => {
    assert.equal(makeConvert(newString("false"), TBoolean).asBoolean(), true);
    assert.equal(makeConvert(newString(""), TBoolean).asBoolean(), false);
    assert.equal(makeConvert(newInteger(0n), TBoolean).asBoolean(), false);
    assert.equal(makeConvert(newInteger(1n), TBoolean).asBoolean(), true);
  });

  it("converts to Atom, and refuses an unsupported target", () => {
    assert.equal(makeConvert(newString("a"), TAtom).asAtom(), "a");
    assert.throws(
      () => makeConvert(newString("x"), newType("Node")),
      (e: unknown) =>
        e instanceof BoruError && e.message.includes("unsupported target type"),
    );
  });
});

describe("makePathon", () => {
  it("builds from a slash-separated string, collapsing runs", () => {
    assert.equal(
      makePathon(newString("a/b/c"), undefined)[0]!.toString(),
      "a/b/c",
    );
    assert.equal(
      makePathon(newString("a//b"), undefined)[0]!.toString(),
      "a/b",
    );
  });

  it("reads a leading slash as absolute", () => {
    assert.equal(
      makePathon(newString("/a/b"), undefined)[0]!.toString(),
      "/a/b",
    );
  });

  it("builds from a list of parts", () => {
    const parts = newList([newString("a"), newString("b")]);
    assert.equal(makePathon(parts, undefined)[0]!.toString(), "a/b");
  });

  it("lets an explicit abs override win either way", () => {
    assert.equal(makePathon(newString("a/b"), true)[0]!.toString(), "/a/b");
    assert.equal(makePathon(newString("/a/b"), false)[0]!.toString(), "a/b");
  });

  it("refuses a source that is neither list nor string", () => {
    assert.throws(() => makePathon(newInteger(1n), undefined), BoruError);
  });
});

describe("makeEmailon", () => {
  it("accepts a plain address", () => {
    assert.equal(makeEmailon(newString("u@h.com"))[0]!.toString(), "u@h.com");
  });

  it("rejects a second @ rather than splitting at the last one", () => {
    // The validation regex is [^@\s<>]+@[^@\s<>]+ — anchored and @-free on
    // both sides — so a two-@ address never reaches the lastIndexOf split.
    assert.throws(() => makeEmailon(newString("a@b@example.com")), BoruError);
  });

  it("refuses a malformed address and a non-string source", () => {
    assert.throws(() => makeEmailon(newString("nope")), BoruError);
    assert.throws(() => makeEmailon(newString("a b@c")), BoruError);
    assert.throws(() => makeEmailon(newInteger(1n)), BoruError);
  });
});

describe("makeUrlon", () => {
  it("accepts an absolute URL and keeps its parts", () => {
    const u = makeUrlon(newString("https://h.example:8080/p?q=1#f"))[0]!;
    const s = u.toString();
    assert.match(s, /https/);
    assert.match(s, /h\.example/);
  });

  it("refuses a relative URL and a non-string source", () => {
    assert.throws(() => makeUrlon(newString("/just/a/path")), BoruError);
    assert.throws(() => makeUrlon(newInteger(1n)), BoruError);
  });
});

describe("makeScalarHandler", () => {
  it("passes a value through when it already matches the target", () => {
    const out = makeScalarHandler([newTypeLiteral(TInteger), newInteger(5n)]);
    assert.equal(out[0]!.asInteger(), 5n);
  });

  it("converts when it does not", () => {
    const out = makeScalarHandler([newTypeLiteral(TString), newInteger(5n)]);
    assert.equal(out[0]!.asString(), "5");
  });

  it("routes the three structured scalars to their builders", () => {
    assert.equal(
      makeScalarHandler([
        newTypeLiteral(TPathon),
        newString("a/b"),
      ])[0]!.toString(),
      "a/b",
    );
    assert.match(
      makeScalarHandler([
        newTypeLiteral(TEmailon),
        newString("u@h.com"),
      ])[0]!.toString(),
      /u@h\.com/,
    );
    assert.match(
      makeScalarHandler([
        newTypeLiteral(TUrlon),
        newString("https://h.com"),
      ])[0]!.toString(),
      /h\.com/,
    );
  });

  it("refuses a non-type-literal target", () => {
    assert.throws(
      () => makeScalarHandler([newInteger(1n), newInteger(2n)]),
      BoruError,
    );
  });
});

describe("makeScalarOptsHandler", () => {
  it("reads abs from the options map for a Path target", () => {
    const opts = map([["abs", newBoolean(true)]]);
    const out = makeScalarOptsHandler([
      newTypeLiteral(TPathon),
      opts,
      newString("a/b"),
    ]);
    assert.equal(out[0]!.toString(), "/a/b");
  });

  it("ignores a non-boolean or missing abs", () => {
    const empty = map([]);
    assert.equal(
      makeScalarOptsHandler([
        newTypeLiteral(TPathon),
        empty,
        newString("a/b"),
      ])[0]!.toString(),
      "a/b",
    );
  });

  it("ignores the options entirely for a non-Path target", () => {
    const opts = map([["abs", newBoolean(true)]]);
    const out = makeScalarOptsHandler([
      newTypeLiteral(TString),
      opts,
      newInteger(3n),
    ]);
    assert.equal(out[0]!.asString(), "3");
  });
});

describe("makeIdealHandler", () => {
  it("builds an Options instance from a source map", () => {
    const src = map([["a", newBoolean(true)]]);
    const out = makeIdealHandler([
      newTypeLiteral(newType("Ideal/Options")),
      src,
    ]);
    assert.equal(out.length, 1);
  });

  it("refuses every other Ideal target with the shared wording", () => {
    assert.throws(
      () => makeIdealHandler([newTypeLiteral(newType("Ideal/Store")), map([])]),
      (e: unknown) =>
        e instanceof BoruError && e.message.includes("unsupported target type"),
    );
  });
});
