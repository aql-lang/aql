// Unit campaign for resolve.ts and capability.ts — three exported functions
// between them, none of which any test called (both files reported 0.00%
// function coverage).
//
// resolve.ts is ADR-012 rule 4's canonical cascade, and its file header warns
// that re-implementing either arm inline is how the Go per-site cascades
// drifted apart (NUR059). These tests pin the cascade ORDER, which is the
// part a re-implementation gets wrong: keywords, then the registry's binding
// store, then the builtin name table, then Atom.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { cap } from "./capability.ts";
import { Registry } from "./registry.ts";
import { resolveWordValue, resolveWordsDeep } from "./resolve.ts";
import { TInteger, TString } from "./type.ts";
import {
  OrderedMap,
  newDisjunct,
  newInteger,
  newList,
  newMap,
  newString,
  newTypeLiteral,
  newTypedList,
  newWord,
} from "./value.ts";

describe("resolveWordValue", () => {
  it("passes a non-word through untouched", () => {
    const v = newInteger(1n);
    assert.equal(resolveWordValue(v), v);
  });

  it("resolves the three keywords", () => {
    assert.equal(resolveWordValue(newWord("true")).asBoolean(), true);
    assert.equal(resolveWordValue(newWord("false")).asBoolean(), false);
    assert.equal(resolveWordValue(newWord("none")).isNone(), true);
  });

  it("resolves a builtin type name to its type literal", () => {
    assert.equal(resolveWordValue(newWord("Integer")).toString(), "Integer");
    assert.equal(resolveWordValue(newWord("String")).toString(), "String");
  });

  it("falls through to an Atom for an unknown name", () => {
    assert.equal(resolveWordValue(newWord("nope")).asAtom(), "nope");
  });

  it("lets a registry binding win over the builtin table", () => {
    // Arm order is the contract: the binding store is consulted BEFORE the
    // builtin names, so a user type shadows a builtin of the same name.
    const r = new Registry();
    r.pushDef("Integer", newString("shadowed"));
    assert.equal(
      resolveWordValue(newWord("Integer"), r).asString(),
      "shadowed",
    );
    // ...but the keywords are decided before the registry is consulted.
    r.pushDef("true", newString("nope"));
    assert.equal(resolveWordValue(newWord("true"), r).asBoolean(), true);
  });

  it("still reaches the builtin table for a name the registry does not bind", () => {
    const r = new Registry();
    assert.equal(resolveWordValue(newWord("Integer"), r).toString(), "Integer");
    assert.equal(resolveWordValue(newWord("nope"), r).asAtom(), "nope");
  });
});

describe("resolveWordsDeep", () => {
  it("resolves list elements", () => {
    const out = resolveWordsDeep(
      newList([newWord("true"), newWord("Integer")]),
    );
    assert.equal(out.toString(), "[true Integer]");
  });

  it("resolves map values and keeps insertion order", () => {
    const m = new OrderedMap();
    m.set("a", newWord("true"));
    m.set("b", newWord("nope"));
    assert.equal(resolveWordsDeep(newMap(m)).toString(), "{a:true b:nope}");
  });

  it("resolves a typed container child and its elements", () => {
    const tl = newTypedList(newWord("Integer"), [newWord("true")]);
    assert.equal(resolveWordsDeep(tl).toString(), "[:Integer true]");
  });

  it("preserves list eval/quoted flags through the walk", () => {
    const q = newList([newWord("true")], { quoted: true });
    assert.equal(resolveWordsDeep(q).quoted, true);
    const e = newList([newWord("true")], { eval: true });
    assert.equal(resolveWordsDeep(e).eval, true);
  });

  it("flattens a nested disjunct and drops duplicate alternatives", () => {
    // The reason simplifyDisjunctAlts exists: an alternative that RESOLVED to
    // a named union must contribute its members, not ride as one opaque value
    // that membership then compares by equality.
    const inner = newDisjunct([
      newTypeLiteral(TInteger),
      newTypeLiteral(TString),
    ]);
    const outer = newDisjunct([inner, newTypeLiteral(TInteger)]);
    const out = resolveWordsDeep(outer);
    assert.equal(out.toString(), "Integer tor String");
  });

  it("leaves an unresolvable value alone", () => {
    const v = newInteger(1n);
    assert.equal(resolveWordsDeep(v), v);
  });
});

describe("cap", () => {
  it("returns the stored capability and true", () => {
    const r = new Registry();
    r.setCapability("fileops", { read: true });
    const [v, ok] = cap<{ read: boolean }>(r, "fileops");
    assert.equal(ok, true);
    assert.deepEqual(v, { read: true });
  });

  it("returns undefined and false when the capability is missing", () => {
    const [v, ok] = cap(new Registry(), "absent");
    assert.equal(ok, false);
    assert.equal(v, undefined);
  });
});
