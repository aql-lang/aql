package aql

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"

	udk "voxgiguniversalsdk"
)

// FileOps is the interface for file system operations used by read/write words.
type FileOps = fileops.FileOps

// Format handles encoding and decoding file content for a specific format.
type Format = engine.Format

// Type represents an AQL type such as "string", "number/integer", or "any".
// Use NewType to create types from slash-separated paths.
type Type = engine.Type

// Value is a typed entry on the AQL stack.
type Value = engine.Value

// Signature describes one way a function can be called.
// Args lists the types the word needs, ordered deepest-first (Args[0] = deepest
// on the stack, Args[last] = top of the stack for prefix matching).
//
// Precedence controls binding strength for suffix-precedence words;
// higher values bind tighter (0 = default).
//
// Handler receives the matched args and returns replacement values for the stack.
type Signature = engine.Signature

// Well-known AQL types for use in Signature definitions.
var (
	TAny     = engine.TAny
	TScalar  = engine.TScalar
	TString  = engine.TString
	TNumber  = engine.TNumber
	TInteger = engine.TInteger
	TDecimal = engine.TDecimal
	TBoolean = engine.TBoolean
	TNode    = engine.TNode
	TAtom    = engine.TAtom
	TList    = engine.TList
	TMap     = engine.TMap
	TTable   = engine.TTable
	TRecord  = engine.TRecord
)

// NewType creates a Type from a slash-separated path (e.g. "string/proper",
// "number/integer"). Use this for custom or hierarchical types.
var NewType = engine.NewType

// NewString creates a string Value.
var NewString = engine.NewString

// NewInteger creates a number/integer Value.
var NewInteger = engine.NewInteger

// NewBoolean creates a boolean Value.
var NewBoolean = engine.NewBoolean

// NewList creates a list Value from a slice of Values.
var NewList = engine.NewList

// NewMap creates a map Value from an OrderedMap.
var NewMap = engine.NewMap

// NewAtom creates an atom Value from a bare name.
var NewAtom = engine.NewAtom

// NewMemFileOps creates an in-memory file system for testing.
func NewMemFileOps() *fileops.MemFileOps {
	return fileops.NewMem()
}

// Options configures an AQL instance.
type Options struct {
	// Registry is a string identifier for the registry to use.
	Registry string
}

// AQL is an independent AQL execution instance.
// Each instance has its own state (set/get storage is isolated).
// Create multiple instances with New() for independent execution contexts.
type AQL struct {
	registry *engine.Registry
	options  Options
	manager  *udk.UniversalManager
}

// New creates a new AQL instance with built-in functions.
// An optional Options value may be provided to configure the instance.
func New(opts ...Options) (*AQL, error) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}

	reg, err := engine.DefaultRegistry()
	if err != nil {
		return nil, err
	}
	reg.SetParseFunc(parser.Parse)
	native.Register(reg)

	um := udk.NewUniversalManager(map[string]any{
		"registry": o.Registry,
	})

	reg.Manager = um

	return &AQL{registry: reg, options: o, manager: um}, nil
}

// Options returns the Options the instance was created with.
func (a *AQL) Options() Options {
	return a.options
}

// SetFileOps replaces the file operations implementation used by read/write.
func (a *AQL) SetFileOps(ops FileOps) {
	a.registry.SetFileOps(ops)
}

// RegisterFormat adds or replaces a format in the format registry.
// Formats are used by read/write words via the {fmt:"name"} option.
func (a *AQL) RegisterFormat(name string, f Format) {
	a.registry.Formats[name] = f
}

// Register adds a named word with one or more signatures.
// Registered words use suffix precedence: the engine tries to collect
// arguments from after the word before falling back to prefix matching.
//
// Example — register a word "double" that doubles an integer:
//
//	a.Register("double", aql.Signature{
//	    Args: []aql.Type{aql.TInteger},
//	    Handler: func(args []aql.Value) ([]aql.Value, error) {
//	        n := args[0].AsInteger()
//	        return []aql.Value{aql.NewInteger(n * 2)}, nil
//	    },
//	})
//
// Use the Precedence field on Signature to control binding strength
// for suffix argument collection (higher binds tighter, 0 = default).
func (a *AQL) Register(name string, sigs ...Signature) {
	a.registry.Register(name, sigs...)
}

// RegisterPrefixOnly adds a named word with one or more signatures that
// only match prefix arguments (values already on the stack before the word).
// No suffix argument collection is attempted.
//
// Example — register a prefix-only word "neg" that negates an integer:
//
//	a.RegisterPrefixOnly("neg", aql.Signature{
//	    Args: []aql.Type{aql.TInteger},
//	    Handler: func(args []aql.Value) ([]aql.Value, error) {
//	        n := args[0].AsInteger()
//	        return []aql.Value{aql.NewInteger(-n)}, nil
//	    },
//	})
func (a *AQL) RegisterPrefixOnly(name string, sigs ...Signature) {
	a.registry.RegisterPrefixOnly(name, sigs...)
}

// SetSDK injects an SDK instance for the given spec name.
// Used in tests to provide a pre-configured SDK (e.g. test mode with mock data).
func (a *AQL) SetSDK(spec string, sdk any) {
	a.registry.SDKCache[spec] = sdk
}

// Run parses and executes an AQL source string.
// The source may span multiple lines; newlines and tabs are treated as
// whitespace (equivalent to spaces).
//
// Returns the result stack as Go values:
//   - int64 for integers
//   - string for strings
//
// State from set/get persists across multiple Run calls on the same instance.
func (a *AQL) Run(src string) ([]any, error) {
	values, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}

	eng := engine.NewTop(a.registry)
	result, err := eng.Run(values)
	if err != nil {
		return nil, err
	}

	out := make([]any, len(result))
	for i, v := range result {
		switch {
		case v.VType.Matches(engine.TInteger):
			out[i] = v.AsInteger()
		case v.VType.Matches(engine.TString):
			out[i] = v.AsString()
		default:
			out[i] = v.String()
		}
	}
	return out, nil
}
