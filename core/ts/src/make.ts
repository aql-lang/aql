// Scalar `make` coercion, ported from eng/go/core_make.go +
// registry.go::ValToString. Covers the scalar target types the
// eng/spec make.tsv rows exercise: String, Integer, Number, Float,
// Boolean, Atom. Class / Options overloads are added by a later
// increment.
import { valToString } from "./canon.ts";
import { coerceBoolean } from "./coretype.ts";
import { BoruError } from "./error.ts";
import {
  BoruType,
  TAtom,
  TBoolean,
  TFloat,
  TInteger,
  TNumber,
  TEmailon,
  TPathon,
  TString,
  TUrlon,
} from "./type.ts";
import {
  newAtom,
  newBoolean,
  newFloat,
  newInteger,
  newOptions,
  newPath,
  newString,
  OrderedMap,
  Value,
  newEmailon,
  newUrlon,
} from "./value.ts";
import type { UrlonInfo } from "./value.ts";

/**
 * The 2-arg [Ideal, Map] make handler. Options instances are built from
 * the source map; other Ideal kinds aren't ported yet.
 */
export function makeIdealHandler(args: Value[]): Value[] {
  const target = args[0]!;
  const src = args[1]!;
  if (
    target.data === null &&
    target.vType.leaf() === "Options" &&
    src.data instanceof OrderedMap
  ) {
    return [newOptions(src.data)];
  }
  // NUR018 refusal form, matching Go's makeHandler: Stores are minted by
  // the context machinery and Errors by `raise` — never by make. The
  // detail wording must match the Go engine's (`unsupported target
  // type <path>`) so the shared spec rows and the crossdiff error-code
  // comparison agree.
  throw new BoruError(
    "unsupported",
    `make: unsupported target type ${target.vType.toString()}`,
  );
}

/**
 * Build a path value from a string (slash-separated) or a list of
 * parts. Slash runs collapse, a leading slash means absolute, and an
 * explicit `abs` override wins. Mirrors makePathon in eng/go.
 */
export function makePathon(
  src: Value,
  absOverride: boolean | undefined,
): Value[] {
  let text: string;
  if (Array.isArray(src.data)) {
    text = (src.data as Value[]).map((p) => valToString(p)).join("/");
  } else if (src.vType.matches(TString)) {
    text = valToString(src);
  } else {
    throw new BoruError(
      "type_error",
      `make: Pathon source must be a list or string, got ${src.toString()}`,
    );
  }
  let abs = text.startsWith("/");
  const segments = text.split("/").filter((s) => s !== "");
  if (absOverride !== undefined) abs = absOverride;
  return [newPath(segments, abs)];
}

/**
 * Build an Emailon from a plain user@host address string. Mirrors the
 * Go engine's makeEmailon (string form; the map form is Go-only for
 * now — lang/spec/micron.tsv owns the full battery).
 */
export function makeEmailon(src: Value): Value[] {
  if (!src.vType.matches(TString) || src.data === null) {
    throw new BoruError(
      "type_error",
      `make: Emailon source must be a string or map, got ${src.toString()}`,
    );
  }
  const s = valToString(src);
  if (!/^[^@\s<>]+@[^@\s<>]+$/.test(s)) {
    throw new BoruError(
      "type_error",
      `make: invalid email address ${JSON.stringify(s)}`,
    );
  }
  const at = s.lastIndexOf("@");
  return [newEmailon(s.slice(0, at), s.slice(at + 1))];
}

/**
 * Build an Urlon from an absolute URL string. Mirrors the Go engine's
 * makeUrlon (string form).
 */
export function makeUrlon(src: Value): Value[] {
  if (!src.vType.matches(TString) || src.data === null) {
    throw new BoruError(
      "type_error",
      `make: Urlon source must be a string or map, got ${src.toString()}`,
    );
  }
  const s = valToString(src);
  let u: URL;
  try {
    u = new URL(s);
  } catch {
    throw new BoruError(
      "type_error",
      `make: Urlon requires an absolute URL (scheme://host…), got ${JSON.stringify(s)}`,
    );
  }
  if (!u.protocol || !u.hostname) {
    throw new BoruError(
      "type_error",
      `make: Urlon requires an absolute URL (scheme://host…), got ${JSON.stringify(s)}`,
    );
  }
  const info: UrlonInfo = {
    scheme: u.protocol.replace(/:$/, ""),
    host: u.hostname,
  };
  if (u.port !== "") info.port = Number(u.port);
  // WHATWG normalises an empty path to '/'; mirror Go's net/url, which
  // keeps it empty, so both engines render the same href.
  if (
    u.pathname &&
    !(u.pathname === "/" && !s.slice(s.indexOf("://") + 3).includes("/"))
  ) {
    info.path = u.pathname;
  }
  if (u.search) info.query = u.search.slice(1);
  if (u.hash) info.fragment = u.hash.slice(1);
  return [newUrlon(info)];
}

/** Convert a source value to a target scalar type. Mirrors MakeConvert. */
export function makeConvert(src: Value, targetType: BoruType): Value {
  if (targetType.equal(TPathon)) {
    return makePathon(src, undefined)[0]!;
  }
  if (targetType.matches(TString)) {
    return newString(valToString(src));
  }
  if (targetType.matches(TFloat)) {
    const text = valToString(src);
    const f = Number(text);
    if (text.trim() === "" || Number.isNaN(f)) {
      throw new BoruError(
        "type_error",
        `make: cannot convert ${JSON.stringify(text)} to float`,
      );
    }
    return newFloat(f);
  }
  if (targetType.matches(TNumber) || targetType.matches(TInteger)) {
    const text = valToString(src);
    if (/^-?\d+$/.test(text)) return newInteger(BigInt(text));
    const f = Number(text);
    if (text.trim() === "" || Number.isNaN(f)) {
      throw new BoruError(
        "type_error",
        `make: cannot convert ${JSON.stringify(text)} to number`,
      );
    }
    return newInteger(BigInt(Math.trunc(f)));
  }
  if (targetType.matches(TBoolean)) {
    // Boolean is a COERCION, not a parse — identical to coerceBoolean and
    // to the `convert`/`if` rule: false iff absent/empty (empty String, 0,
    // none, empty collection), true otherwise. String CONTENT is never
    // inspected, so "false"/"true" are ordinary non-empty strings that both
    // coerce to true. Parsing the literal words is a separate operation.
    return newBoolean(coerceBoolean(src));
  }
  if (targetType.equal(TAtom)) {
    return newAtom(valToString(src));
  }
  throw new BoruError(
    "type_error",
    `make: unsupported target type ${targetType.toString()}`,
  );
}

/**
 * The 2-arg [Scalar, Any] make handler: coerce src to the target
 * scalar type. Mirrors MakeScalarHandler (scalar subset — no Path,
 * no user-minted refinement reparent yet).
 */
export function makeScalarHandler(args: Value[]): Value[] {
  const target = args[0]!;
  const src = args[1]!;
  if (target.data !== null) {
    throw new BoruError(
      "type_error",
      `make: expected a type literal, got ${target.toString()}`,
    );
  }
  const targetType = target.vType;
  if (targetType.equal(TPathon)) return makePathon(src, undefined);
  if (targetType.equal(TEmailon)) return makeEmailon(src);
  if (targetType.equal(TUrlon)) return makeUrlon(src);
  if (src.vType.matches(targetType)) return [src];
  return [makeConvert(src, targetType)];
}

/**
 * The 3-arg [Scalar, Map, Any] make handler: scalar coercion with an
 * options map. Only Path consults the options (`abs`); other scalars
 * ignore them. Mirrors MakeScalarOptsHandler.
 */
export function makeScalarOptsHandler(args: Value[]): Value[] {
  const target = args[0]!;
  const opts = args[1]!;
  const src = args[2]!;
  if (target.data === null && target.vType.equal(TPathon)) {
    let abs: boolean | undefined;
    if (opts.data instanceof OrderedMap) {
      const a = opts.asMap().get("abs");
      if (a !== undefined && a.vType.matches(TBoolean)) abs = a.asBoolean();
    }
    return makePathon(src, abs);
  }
  return makeScalarHandler([target, src]);
}
