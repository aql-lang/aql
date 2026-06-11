package native

import "fmt"

// This file holds the user-facing error words (design/ERRORS.8.md §2,
// retiring DX report T9.6):
//
//	raise "boom"                          code user_error
//	raise bad_input "expected a list"     explicit code (bare word ok)
//	raise {code: bad_input, message: "…", got: 42}
//
// `raise` constructs the same AqlError native handlers return, so the
// engine's existing abort/catch machinery applies unchanged: uncaught
// it formats as [aql/<code>], and `do [body] error [handler]` catches
// it as the same Ideal/Error value native errors produce. Extra spec-
// map keys ride along on the Error value (ErrorInfo.Data) for
// programmatic handlers; the formatter prints code + message only.
//
// The Error value's fields are readable in handlers via dot access /
// get: `e.code` (atom), `e.message` (string), plus any raise payload
// keys; `convert Map e` projects the same fields.
var errorNatives = []NativeFunc{
	{
		Name: "raise",

		Signatures: []NativeSig{
			// raise <code> <message> — the /q'd Atom position lets a
			// bare word name the code (raise bad_input "…").
			{
				Args:      []*Type{TAtom, TString},
				QuoteArgs: map[int]bool{0: true},
				Handler:   raiseCodeMessageHandler,
				Returns:   []*Type{}, BarrierPos: -1,
			},
			// raise <message> — code defaults to user_error.
			{
				Args:    []*Type{TString},
				Handler: raiseMessageHandler,
				Returns: []*Type{}, BarrierPos: -1,
			},
			// raise <spec> — {code:… message:…} required; remaining
			// keys are preserved on the Error value for the handler.
			{
				Args:    []*Type{TMap},
				Handler: raiseSpecHandler,
				Returns: []*Type{}, BarrierPos: -1,
			},
		},
	},
	{
		// get on an Error value — code/message plus any raise payload
		// keys. Registered here (not native_storage.go) to keep the
		// whole Error surface in one file.
		Name: "get",

		Signatures: []NativeSig{
			{Args: []*Type{TString, TError}, Handler: getErrorFieldHandler, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAtom, TError}, QuoteArgs: map[int]bool{0: true}, Handler: getErrorFieldHandler, Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
}

func raiseMessageHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	msg, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("raise_error", "raise: message must be a concrete string", "raise")
	}
	return nil, r.AqlError("user_error", msg, "raise")
}

func raiseCodeMessageHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	code, err := args[0].AsConcreteAtom()
	if err != nil {
		return nil, r.AqlError("raise_error", "raise: code must be an atom (a bare word works: raise bad_input \"…\")", "raise")
	}
	msg, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("raise_error", "raise: message must be a concrete string", "raise")
	}
	return nil, r.AqlError(code, msg, "raise")
}

func raiseSpecHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, err := RequireConcreteMap(args[0], "raise")
	if err != nil {
		return nil, r.AqlError("raise_error", "raise: spec must be a concrete map", "raise")
	}
	codeVal, hasCode := m.Get("code")
	msgVal, hasMsg := m.Get("message")
	if !hasCode || !hasMsg {
		return nil, r.AqlErrorHint("raise_error",
			"raise: spec map needs both 'code' and 'message' keys", "raise",
			`hint: raise {code: bad_input, message: "expected a list"} — extra keys ride along for the handler`)
	}
	code, cerr := codeVal.AsConcreteAtom()
	if cerr != nil {
		if s, serr := codeVal.AsConcreteString(); serr == nil {
			code = s
		} else {
			return nil, r.AqlError("raise_error",
				fmt.Sprintf("raise: code must be an atom or string, got %s", codeVal.String()), "raise")
		}
	}
	msg, merr := msgVal.AsConcreteString()
	if merr != nil {
		return nil, r.AqlError("raise_error",
			fmt.Sprintf("raise: message must be a string, got %s", msgVal.String()), "raise")
	}
	var data *OrderedMap
	for _, k := range m.Keys() {
		if k == "code" || k == "message" {
			continue
		}
		if data == nil {
			data = NewOrderedMap()
		}
		v, _ := m.Get(k)
		data.Set(k, v)
	}
	ae := MakeAqlError(code, msg, "raise", r.Source, "")
	ae.Data = data
	return nil, ae
}

// getErrorFieldHandler reads a field off an Error value: "code" (an
// atom; none when the error carries no stable code), "message" (the
// short description), or any key the raising spec map carried. An
// unknown key yields none, mirroring map reads.
func getErrorFieldHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := ValToString(args[0])
	ei, err := AsError(args[1])
	if err != nil {
		return nil, r.AqlError("get_error", "get: not an error value", "get")
	}
	switch key {
	case "code":
		if ei.Code == "" {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{NewAtom(ei.Code)}, nil
	case "message":
		return []Value{NewString(ei.Message)}, nil
	}
	if ei.Data != nil {
		if v, ok := ei.Data.Get(key); ok {
			return []Value{v}, nil
		}
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}
