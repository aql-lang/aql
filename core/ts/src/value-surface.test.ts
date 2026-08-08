// value.ts's EXPORTED SURFACE — the constructors, accessors and guards
// core/ts publishes but does not itself call.
//
// Every symbol here has a live consumer in eng/ts (its index.ts re-exports
// core's value module wholesale, and spec-fixture.ts builds values with
// these constructors), so none of it is dead code to delete under ADR-008.
// But core/ts's own gate measures core/ts's suite ALONE, so a symbol only
// eng/ts exercises reads as uncovered here — and an uncovered line is
// where a divergence hides. These are direct-call tests: the payload each
// constructor stores, the type it stamps, and the message each accessor
// raises when handed the wrong shape.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import {
  ClassTypeInfo,
  OrderedMap,
  Value,
  asSugar,
  isErrorValue,
  isSugar,
  newAny,
  newAtom,
  newClassType,
  newConstrainedWord,
  newDisjunct,
  newDynamicCarrier,
  newEnum,
  newErrorValue,
  newEnd,
  newMark,
  newMove,
  newFnUndef,
  newForwardMarker,
  newInteger,
  newList,
  newMap,
  newNone,
  newOptions,
  newSplice,
  newString,
  newSugar,
  newWord,
  withQuoted,
  TAny,
  TInteger,
} from "./index.ts";

describe("value surface", () => {
  it("isTypeLiteral separates a bare type from a carrier and from none", () => {
    // A type literal is payload-less, uncarried and not the none value —
    // all three conjuncts matter, so each gets a witness.
    assert.equal(new Value(TInteger, null).isTypeLiteral(), true);
    assert.equal(newInteger(1n).isTypeLiteral(), false);
    assert.equal(newDynamicCarrier(TInteger).isTypeLiteral(), false);
    assert.equal(newNone().isTypeLiteral(), false);
  });

  it("the accessors name the shape they wanted when handed another", () => {
    assert.throws(() => newInteger(1n).asWord(), /AsWord: not a word value/);
    assert.throws(
      () => newInteger(1n).asDisjunct(),
      /AsDisjunct: not a disjunct value/,
    );
    assert.throws(() => new Value(TAny, null).asFnDef(), /AsFnDef: nil data/);
    assert.throws(
      () => newInteger(1n).asFnDef(),
      /AsFnDef: not a function value/,
    );
  });

  it("isOptions reads the payload class, not the type", () => {
    const om = new OrderedMap();
    om.set("a", newInteger(1n));
    assert.equal(newOptions(om).isOptions(), true);
    assert.equal(newMap(om).isOptions(), false);
  });

  it("isErrorValue reads the payload class too", () => {
    assert.equal(isErrorValue(newErrorValue("boom")), true);
    assert.equal(isErrorValue(newString("boom")), false);
  });

  it("renders the engine-internal markers", () => {
    const fwd = newForwardMarker({
      funcName: "addq",
      sig: {
        args: [TInteger, TInteger],
        returns: [TInteger],
        handler: () => [],
      },
      expectedForward: 2,
      collected: [newInteger(1n)],
      stackArgs: [],
    });
    assert.equal(fwd.toString(), "forward(addq,1/2)");
    assert.equal(newMark("m1", []).toString(), "mark(m1)");
    assert.equal(newMove("m1").toString(), "move(m1,)");
    assert.equal(newEnd().toString(), "end");
  });

  it("newConstrainedWord carries the binding-name constraint", () => {
    const w = newConstrainedWord("xs", newList([newInteger(1n)]));
    assert.equal(w.asWord().name, "xs");
    assert.equal(w.asWord().constraint?.asList().length, 1);
    // The plain constructor leaves the slot empty — that difference is
    // what the matcher's /q arm reads to keep a constrained word intact.
    assert.equal(newWord("xs").asWord().constraint, undefined);
  });

  it("newAny wraps an arbitrary payload at the lattice root", () => {
    const v = newAny({ tag: "opaque" });
    assert.equal(v.vType.equal(TAny), true);
    assert.deepEqual(v.data, { tag: "opaque" });
  });

  it("newDynamicCarrier is a carrier whose type is statically unknown", () => {
    const c = newDynamicCarrier(TInteger);
    assert.equal(c.carrier, true);
    assert.equal(c.dynamic, true);
    assert.equal(c.data, null);
  });

  it("OrderedMap keeps insertion order and offers the sorted render order", () => {
    const om = new OrderedMap();
    om.set("b", newInteger(2n));
    om.set("a", newInteger(1n));
    assert.deepEqual(om.keys(), ["b", "a"]);
    assert.deepEqual(om.sortedKeys(), ["a", "b"]);
  });

  it("newClassType stamps a Type-branch value over its ClassTypeInfo", () => {
    const fields = new OrderedMap();
    fields.set("x", newInteger(0n));
    const ct = newClassType(new ClassTypeInfo("Point", "Class", fields));
    assert.equal(ct.vType.toString(), "Type");
    assert.equal((ct.data as ClassTypeInfo).path, "Class/Point");
  });

  it("the type-branch constructors each stamp their own lattice node", () => {
    const alts = [newInteger(1n), newInteger(2n)];
    assert.equal(newDisjunct(alts).vType.toString(), "Type/Disjunct");
    assert.equal(newEnum(alts).vType.toString(), "Type/Disjunct/Enum");
    assert.equal(newFnUndef([]).vType.toString(), "Type/FunctionSignature");
    // Enum is a Disjunct SUBTYPE, so it answers the disjunct guard too.
    assert.equal(newEnum(alts).isDisjunct(), true);
  });

  it("withQuoted clones the flags it must preserve and sets quoted", () => {
    const src = newList([newInteger(1n)], { eval: true });
    const q = withQuoted(src);
    assert.equal(q.quoted, true);
    assert.equal(q.eval, true);
    assert.equal(q.asList().length, 1);
    // The source is untouched — quoting is a copy, not a mutation.
    assert.equal(src.quoted, false);
  });

  it("newSplice wraps its payload behind the splice marker type", () => {
    const sp = newSplice(newInteger(7n));
    assert.equal(sp.vType.toString(), "Word/__SP");
    assert.equal((sp.data as { payload: Value }).payload.asInteger(), 7n);
  });

  it("asSugar returns the info only for a sugar marker", () => {
    const s = newSugar({ kind: "angle", head: newWord("Box"), items: [] });
    assert.equal(isSugar(s), true);
    assert.equal(asSugar(s)?.kind, "angle");
    assert.equal(asSugar(newAtom("angle")), undefined);
  });
});
