package modules

import (
	"fmt"

	"github.com/aql-lang/aql/lang/go/native"
)

// The VALUE constructors — the host-side successors of the removed
// registration APIs (RegisterHostParser / RegisterFormatParser's parse
// side). The kind namespaces are FIXED, so a Go-implemented custom language
// is handed to programs as a Function VALUE: bind it to a name
// ((*AQL).DefineValue or a def), export it from a module, or pass it to the
// macro word directly. The values carry the same framework shell the
// built-in kinds get (source resolution, pure folding), wrapped as a
// trivial-delegation FnDef over a private sub-registry — the modules/wrap.go
// contract — so they dispatch exactly like a ParseLang.parse_<kind> export.

// NewParseLangFn builds a ParseLang Function VALUE from spec: the
// source-resolving shell (parseSourceShell — a String or {src:…} map
// resolves before the handler runs) over spec.Handler, with the check-time
// pure fold wired when spec.Pure. The value satisfies the `parse` word's
// [source opts] contract (ParseLangFnSigWhy), so `parse <name> '<text>'`
// works once the value is bound to <name>.
func NewParseLangFn(spec ParseLangSpec) (native.Value, error) {
	if spec.Name == "" {
		return native.Value{}, fmt.Errorf("new parser fn: Name must not be empty")
	}
	if spec.Handler == nil {
		return native.Value{}, fmt.Errorf("new parser fn %q: Handler must not be nil", spec.Name)
	}
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.Value{}, err
	}
	returns := spec.Returns
	if returns == nil {
		returns = []*native.Type{native.TAny}
	}
	inner := "parselang-value-" + spec.Name
	shell := parseSourceShell(spec.Handler)
	sig := native.Signature{
		Args:       []*native.Type{native.TAny, native.TMap},
		Returns:    returns,
		BarrierPos: -1,
		Impl:       native.Go(shell),
	}
	if spec.Pure {
		sig.ReturnsFn = pureParseFoldReturns(returns, shell)
	}
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name:       inner,
		Signatures: []native.Signature{sig},
	})
	// RegisterNativeFunc records an invalid word name instead of installing
	// (ADR-005: no panics) — surface it as this constructor's error.
	if subReg.Lookup(inner) == nil {
		return native.Value{}, fmt.Errorf("new parser fn %q: name is not a valid word (use a plain lowercase name)", spec.Name)
	}
	return wrapMiniFnDef(inner, [][]native.FnParam{
		{{Type: native.TAny}, {Type: native.TMap}},
	}, returns, nil, subReg), nil
}

// NewFormatParserFn wraps a read Format's Decode (a DecodeOpter format
// additionally honours opts) as a ParseLang Function VALUE — the value-form
// successor of the removed RegisterFormatParser's parse side. The read side
// is unchanged: register the format with native.RegisterFormat separately
// when it should also be reachable from `read`.
func NewFormatParserFn(name string, f native.Format) (native.Value, error) {
	return NewParseLangFn(ParseLangSpec{
		Name:    name,
		Returns: []*native.Type{native.TAny},
		Handler: formatParseHandler(name, f),
	})
}
