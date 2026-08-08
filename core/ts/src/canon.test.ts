// Unit campaign for the canonical renderer's tails: the escape-fallback
// string form, the exponential float forms and their plain-decimal
// expansion, the typed-container render, and the fn-def render — the
// arms the corpus reaches only through rows that happen to carry those
// shapes. Mirrors eng/go/canon.go / value.go::FormatFloat.

import { describe, it } from "node:test";
import { strict as assert } from "node:assert";

import { canonString, canonValue, formatFloat } from "./canon.ts";
import { TInteger, TString } from "./type.ts";
import {
  newFnDef,
  newInteger,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
} from "./value.ts";

describe("canonString escape fallback", () => {
  it("keeps plain content single-quoted", () => {
    assert.equal(canonString("abc"), "'abc'");
  });
  it("switches to double quotes for a single quote", () => {
    assert.equal(canonString("it's"), '"it\'s"');
  });
  it("escapes content carrying both quote kinds", () => {
    assert.equal(canonString(`a'b"c`), `'a\\'b"c'`);
  });
  it("escapes backslashes and control characters", () => {
    assert.equal(canonString("a\\b"), "'a\\\\b'");
    assert.equal(canonString("l1\nl2"), "'l1\\nl2'");
    assert.equal(canonString("a\tb"), "'a\\tb'");
    assert.equal(canonString("a\rb"), "'a\\rb'");
  });
});

describe("formatFloat exponential forms", () => {
  it("renders the everyday range with a decimal point", () => {
    assert.equal(formatFloat(2.5), "2.5");
    assert.equal(formatFloat(3), "3.0");
  });
  it("special values", () => {
    assert.equal(formatFloat(Number.NaN), "nan");
    assert.equal(formatFloat(Infinity), "inf");
    assert.equal(formatFloat(-Infinity), "-inf");
    assert.equal(formatFloat(-0), "-0.0");
  });
  it("switches to the exponent form at the magnitude bounds", () => {
    assert.equal(formatFloat(1e21), "1e+21");
    assert.equal(formatFloat(-2.5e22), "-2.5e+22");
    assert.equal(formatFloat(1e-11), "1e-11");
  });
  it("expands mid-range JS exponentials to plain decimal", () => {
    // String(1e-7) is "1e-7" in JS, but the value is inside the
    // non-scientific window — the expansion arm renders 0.0000001.
    assert.equal(formatFloat(1e-7), "0.0000001");
    assert.equal(formatFloat(-1.25e-7), "-0.000000125");
  });
});

describe("canonValue typed containers and fn defs", () => {
  it("renders a typed list with its child tag and elements", () => {
    const tl = newTypedList(newTypeLiteral(TInteger), [
      newInteger(1),
      newInteger(2),
    ]);
    assert.equal(canonValue(tl), "[:Integer 1 2]");
  });
  it("renders a typed map with its child tag", () => {
    const tm = newTypedMap(newTypeLiteral(TString));
    assert.equal(canonValue(tm), "{:String}");
  });
  it("renders a fn value through its signature triple", () => {
    const fd = newFnDef({
      sigs: [
        {
          params: [{ name: "x", type: TInteger }],
          returns: [TInteger],
          body: [newInteger(1)],
        },
      ],
    });
    assert.match(canonValue(fd), /^fn \[\[x:.*\]\[.*\]\[1\]\]$/);
  });
});
