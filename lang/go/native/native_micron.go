package native

// Property access for the Scalar/Micron family (structured scalars).
//
// Microns are object-like scalars: `get`/`dot` read their named
// properties (primary fields plus the derived address/href/parts/abs —
// eng.MicronProperty owns the property table), `getr`/`dotr` are the
// strict twins (a miss errors instead of reading none), `has` answers
// presence, and `set` is an EXPLICIT error — Microns are immutable, and
// the erroring signature pins a clear message where sig-absence would
// give an opaque dispatch failure. The signature rows live in
// accessorGetSignatures / accessorGetrSignatures / the has and set sig
// sets, keyed on the TMicron receiver.

import (
	"fmt"

	eng "github.com/aql-lang/aql/eng/go"
)

func getMicronHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	recv := args[1]
	if !IsConcrete(recv) {
		return nil, r.AqlError("get_error", "get: cannot access property on type literal", "get")
	}
	if val, ok := eng.MicronProperty(recv, getKey(key)); ok {
		return []Value{val}, nil
	}
	// Total read: an absent property (unknown key, or an optional
	// field like an Urlon's port that this instance doesn't carry)
	// reads none, like a map miss.
	return []Value{NewTypeLiteral(TNone)}, nil
}

func getrMicronHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	recv := args[1]
	if !IsConcrete(recv) {
		return nil, r.AqlError("getr_error", "getr: cannot access property on type literal", "getr")
	}
	k := getKey(key)
	if val, ok := eng.MicronProperty(recv, k); ok {
		return []Value{val}, nil
	}
	return nil, r.AqlError("not_found",
		fmt.Sprintf("getr: %s has no property %q", recv.Parent.Name, k), "getr")
}

func hasMicronHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	recv := args[1]
	if !IsConcrete(recv) {
		return []Value{NewBoolean(false)}, nil
	}
	_, ok := eng.MicronProperty(recv, getKey(args[0]))
	return []Value{NewBoolean(ok)}, nil
}

// setMicronHandler is the D7 immutability contract: an explicit
// erroring signature, chosen over sig-absence so the paired negative
// spec rows pin a specific message.
func setMicronHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return nil, r.AqlError("type_error", setMicronDetail(args), "set")
}

func setMicronDetail(args []Value) string {
	kind := "Micron"
	if len(args) == 3 && args[2].Parent != nil {
		kind = args[2].Parent.Leaf()
	}
	return fmt.Sprintf("set: %s values are immutable — construct a new one with make", kind)
}

// setMicronReturns is the check-mode mirror of the always-erroring
// handler: a set whose receiver is statically Micron-typed is a
// GUARANTEED runtime error, so the checker flags it with the same
// message.
func setMicronReturns(args []Value, r *Registry) []Value {
	if r != nil && r.Check.IsActive() && len(args) == 3 {
		eng.CheckAddUniqueDiagnostic(r, "type_error", setMicronDetail(args), "set", args[0].Pos)
	}
	return []Value{}
}

// getrMicronReturns is getMicronReturns plus the strict-read contract:
// a statically-known miss — a concrete key that no property of the
// receiver's (known) kind can answer — is a guaranteed runtime
// not_found, so the checker flags it. Optional per-instance fields
// (an Urlon's port) narrow to dynamic in getMicronReturns and are
// never flagged.
func getrMicronReturns(args []Value, r *Registry) []Value {
	out := getMicronReturns(args, r)
	if r != nil && r.Check.IsActive() && len(args) == 2 && IsConcrete(args[0]) &&
		len(out) == 1 && out[0].Carrier && !out[0].Dynamic && out[0].Parent != nil && out[0].Parent.Equal(TNone) {
		kind := "Micron"
		if args[1].Parent != nil {
			kind = args[1].Parent.Leaf()
		}
		eng.CheckAddUniqueDiagnostic(r, "not_found",
			fmt.Sprintf("getr: %s has no property %q", kind, getKey(args[0])), "getr", args[0].Pos)
	}
	return out
}

// getMicronReturns is the check-mode narrowing for Micron property
// reads (modeled on getObjectReturns). A concrete receiver answers
// precisely from the instance itself; a carrier receiver narrows from
// the static per-kind field tables (user kinds from their schema via
// eng.MicronSchemaFor). Optional Urlon fields (port/path/query/
// fragment) are per-instance, so a carrier read of those stays
// dynamic.
func getMicronReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if len(args) != 2 || !IsConcrete(args[0]) || args[1].Parent == nil {
		return dyn
	}
	recv := args[1]
	key := getKey(args[0])
	if IsConcrete(recv) {
		if val, ok := eng.MicronProperty(recv, key); ok {
			if ft := ValueType(val); ft != nil {
				return []Value{NewCarrier(ft)}
			}
			return dyn
		}
		return []Value{NewCarrier(TNone)}
	}
	t := recv.Parent
	switch {
	case t.ConformsTo(TPathon):
		switch key {
		case "parts":
			return []Value{NewCarrierTypedList(TString)}
		case "abs":
			return []Value{NewCarrier(TBoolean)}
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(TEmailon):
		switch key {
		case "user", "host", "address":
			return []Value{NewCarrier(TString)}
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(TUrlon):
		switch key {
		case "scheme", "host", "href":
			return []Value{NewCarrier(TString)}
		case "port", "path", "query", "fragment":
			// Present-or-absent per instance — String/Integer or none.
			return dyn
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(TIpon):
		switch key {
		case "addr":
			return []Value{NewCarrier(TString)}
		case "version":
			return []Value{NewCarrier(TInteger)}
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(THoston):
		switch key {
		case "host", "authority":
			return []Value{NewCarrier(TString)}
		case "port":
			return dyn // optional per instance — Integer or none
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(TSemveron):
		switch key {
		case "major", "minor", "patch":
			return []Value{NewCarrier(TInteger)}
		case "release", "version":
			return []Value{NewCarrier(TString)}
		case "stable":
			return []Value{NewCarrier(TBoolean)}
		case "prereleaseParts", "buildParts":
			return []Value{NewCarrierTypedList(TString)}
		case "prerelease", "build":
			return dyn // optional per instance — String or none
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(TCidron):
		switch key {
		case "cidr", "addr":
			return []Value{NewCarrier(TString)}
		case "prefix", "version", "hostbits":
			return []Value{NewCarrier(TInteger)}
		case "count":
			return dyn // Integer or BigInteger per instance
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(TMacon):
		switch key {
		case "addr", "oui":
			return []Value{NewCarrier(TString)}
		case "bits":
			return []Value{NewCarrier(TInteger)}
		case "eui64", "multicast", "local", "broadcast":
			return []Value{NewCarrier(TBoolean)}
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(TColoron):
		switch key {
		case "r", "g", "b", "a":
			return []Value{NewCarrier(TInteger)}
		case "hex", "css":
			return []Value{NewCarrier(TString)}
		case "opaque":
			return []Value{NewCarrier(TBoolean)}
		case "alpha":
			return []Value{NewCarrier(TFloat)}
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(TMimon):
		switch key {
		case "type", "subtype", "essence", "mime":
			return []Value{NewCarrier(TString)}
		case "params", "suffix", "facet":
			return dyn // optional per instance — Map/String or none
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(TQion):
		switch key {
		case "code", "display":
			return []Value{NewCarrier(TString)}
		case "amount":
			return []Value{NewCarrier(TBigDecimal)}
		case "minor":
			return []Value{NewCarrier(TInteger)}
		case "negative":
			return []Value{NewCarrier(TBoolean)}
		case "units":
			return dyn // Integer or none (overflow) per instance
		}
		return []Value{NewCarrier(TNone)}
	case t.ConformsTo(TPhonon):
		switch key {
		case "e164", "digits":
			return []Value{NewCarrier(TString)}
		case "length":
			return []Value{NewCarrier(TInteger)}
		}
		return []Value{NewCarrier(TNone)}
	}
	if schema, ok := eng.MicronSchemaFor(t); ok {
		if fv, ok := schema.Get(key); ok {
			ft := ValueType(fv)
			if ft == nil || ft.ConformsTo(TFunction) || ft.ConformsTo(TFnDef) {
				return dyn
			}
			return []Value{NewCarrier(ft)}
		}
		return []Value{NewCarrier(TNone)}
	}
	return dyn
}
