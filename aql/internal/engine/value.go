package engine

import "fmt"

// WordInfo carries the name and optional modifiers for a function reference.
type WordInfo struct {
	Name        string
	ArgCount    int  // -1 = unspecified
	ForcePrefix bool // lower/p
	ForceSuffix bool // lower/s
}

// ForwardInfo tracks suffix argument collection for a deferred function call.
type ForwardInfo struct {
	FuncName      string
	ExpectedArgs  int
	CollectedArgs int
	// FuncIndex records where the deferred function word sits in the stack.
	FuncIndex  int
	Precedence int        // copied from matched Signature
	Sig        *Signature // the matched signature, for direct execution on completion
}

// Value is a typed entry on the AQL stack.
type Value struct {
	VType Type
	Data  interface{}
}

// NewString creates a string value. Empty strings get type string/empty.
func NewString(s string) Value {
	if s == "" {
		return Value{VType: TStringEmpty, Data: s}
	}
	return Value{VType: TStringProper, Data: s}
}

// NewInteger creates a number/integer value.
func NewInteger(n int64) Value {
	return Value{VType: TInteger, Data: n}
}

// NewWord creates a word value (function reference) with no modifiers.
func NewWord(name string) Value {
	return Value{
		VType: TWord,
		Data:  WordInfo{Name: name, ArgCount: -1},
	}
}

// NewWordModified creates a word value with explicit modifiers.
func NewWordModified(name string, argCount int, forcePrefix, forceSuffix bool) Value {
	return Value{
		VType: TWord,
		Data: WordInfo{
			Name:        name,
			ArgCount:    argCount,
			ForcePrefix: forcePrefix,
			ForceSuffix: forceSuffix,
		},
	}
}

// NewForward creates a forward primitive value for suffix argument tracking.
func NewForward(info ForwardInfo) Value {
	return Value{VType: TForward, Data: info}
}

// NewOpenParen creates an open-paren marker value for sub-expression scoping.
func NewOpenParen() Value {
	return Value{VType: TOpenParen, Data: nil}
}

// IsWord reports whether this value is a word (function reference).
func (v Value) IsWord() bool {
	return v.VType.Equal(TWord)
}

// IsForward reports whether this value is a forward primitive.
func (v Value) IsForward() bool {
	return v.VType.Equal(TForward)
}

// IsOpenParen reports whether this value is an open-paren marker.
func (v Value) IsOpenParen() bool {
	return v.VType.Equal(TOpenParen)
}

// AsWord returns the WordInfo, panics if not a word.
func (v Value) AsWord() WordInfo {
	return v.Data.(WordInfo)
}

// AsForward returns the ForwardInfo, panics if not a forward.
func (v Value) AsForward() ForwardInfo {
	return v.Data.(ForwardInfo)
}

// AsString returns the string payload, panics if not a string type.
func (v Value) AsString() string {
	return v.Data.(string)
}

// AsInteger returns the int64 payload, panics if not an integer type.
func (v Value) AsInteger() int64 {
	return v.Data.(int64)
}

// String returns a human-readable representation.
func (v Value) String() string {
	switch {
	case v.IsWord():
		w := v.AsWord()
		return fmt.Sprintf("word(%s)", w.Name)
	case v.IsForward():
		f := v.AsForward()
		return fmt.Sprintf("forward(%s,%d/%d)", f.FuncName, f.CollectedArgs, f.ExpectedArgs)
	case v.IsOpenParen():
		return "("
	case v.VType.Matches(TString):
		return fmt.Sprintf("'%s'", v.Data)
	case v.VType.Matches(TInteger):
		return fmt.Sprintf("%d", v.Data)
	default:
		return fmt.Sprintf("%v(%v)", v.VType, v.Data)
	}
}
