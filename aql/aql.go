package aql

import (
	"io"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"github.com/metsitaba/voxgig-exp/aql/internal/nativemod"
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
// Handler receives the matched args, the current context map, the resolved
// stack (only for FullStack signatures), and the registry. Most handlers
// only use the first parameter and ignore the rest with _.
type Signature = engine.Signature

// Well-known AQL types for use in Signature definitions.
var (
	TAny            = engine.TAny
	TScalar         = engine.TScalar
	TString         = engine.TString
	TNumber         = engine.TNumber
	TInteger        = engine.TInteger
	TDecimal        = engine.TDecimal
	TBoolean        = engine.TBoolean
	TNode           = engine.TNode
	TAtom           = engine.TAtom
	TList           = engine.TList
	TMap            = engine.TMap
	TTable          = engine.TTable
	TRecord         = engine.TRecord
	TResource       = engine.TResource
	TResourceEntity = engine.TResourceEntity
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
	// Seed sets the random seed for ID generation.
	// If zero, the current time is used.
	Seed int64
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

	if o.Seed != 0 {
		engine.SetIDSeed(o.Seed)
	}

	reg, err := engine.DefaultRegistry()
	if err != nil {
		return nil, err
	}
	reg.SetParseFunc(parser.Parse)
	reg.NativeModResolver = nativemod.Resolve
	native.Register(reg)

	um := udk.NewUniversalManager(map[string]any{
		"registry": o.Registry,
	})

	reg.Manager = um

	// Enable dynamic help generation for functions registered after this point.
	engine.EnableDynamicHelp(reg)
	reg.MarkReady()

	return &AQL{registry: reg, options: o, manager: um}, nil
}

// Options returns the Options the instance was created with.
func (a *AQL) Options() Options {
	return a.options
}

// Check parses the source and runs it through the engine in static
// type-check mode. Literals are stripped to carrier values (type-only)
// and signature handlers are replaced by carrier return propagation
// driven by Signature.Returns. The actual runtime dispatch, matching,
// and forward-collection machinery is reused verbatim so checker and
// runtime stay in absolute parity.
//
// The returned CheckResult holds the residual carrier stack (as type
// path strings) and any diagnostics the checker collected.
func (a *AQL) Check(src string) (CheckResult, error) {
	values, err := parser.Parse(src)
	if err != nil {
		return CheckResult{}, err
	}

	a.registry.Source = src
	a.registry.CheckMode = true
	a.registry.CheckDiagnostics = nil
	a.registry.CheckStepCount = 0
	a.registry.CheckBudgetTripped = false
	a.registry.CheckDefsInstalled = nil
	a.registry.CheckDefsUsed = nil
	a.registry.CheckContextTypes = nil
	defer func() { a.registry.CheckMode = false }()

	eng := engine.NewTop(a.registry)
	eng.SetSource(src)
	result, err := eng.Run(values)
	// Emit unused-def warnings after all execution has completed
	// so the Used map has been fully populated.
	a.registry.EmitUnusedDefDiagnostics()
	if err != nil {
		return CheckResult{Diagnostics: a.registry.CheckDiagnostics}, err
	}

	stack := make([]string, len(result))
	for i, v := range result {
		stack[i] = v.VType.String()
	}

	// Fill in missing Row/Col on diagnostics by locating the Word
	// in the source text. Best-effort — duplicates fall back to
	// the last occurrence, which is usually the call site rather
	// than the definition.
	diags := a.registry.CheckDiagnostics
	var summary CheckSummary
	for i := range diags {
		if diags[i].Row == 0 && diags[i].Word != "" {
			diags[i].Row, diags[i].Col = engine.FindWordInSource(src, diags[i].Word)
		}
		switch diags[i].Severity {
		case SeverityError:
			summary.Errors++
		case SeverityWarning:
			summary.Warnings++
		default:
			summary.Infos++
		}
	}
	return CheckResult{Stack: stack, Diagnostics: diags, Summary: summary}, nil
}

// SetFileOps replaces the file operations implementation used by read/write.
func (a *AQL) SetFileOps(ops FileOps) {
	a.registry.SetFileOps(ops)
}

// SetOutput replaces the writer used by print, help, and other output words.
func (a *AQL) SetOutput(w io.Writer) {
	a.registry.Output = w
}

// RegisterFormat adds or replaces a format in the format registry.
// Formats are used by read/write words via the {fmt:"name"} option.
func (a *AQL) RegisterFormat(name string, f Format) {
	a.registry.Formats[name] = f
}

// Register adds a named word with one or more signatures.
// Registered words use forward precedence: the engine tries to collect
// arguments from after the word before falling back to prefix matching.
//
// Example — register a word "double" that doubles an integer
// (extra handler params are context, stack, and registry — use _ to ignore):
//
//	a.Register("double", aql.Signature{
//	    Args: []aql.Type{aql.TInteger},
//	    Handler: func(args []aql.Value, _ map[string]aql.Value, _ []aql.Value, _ *engine.Registry) ([]aql.Value, error) {
//	        n := args[0].AsInteger()
//	        return []aql.Value{aql.NewInteger(n * 2)}, nil
//	    },
//	})
func (a *AQL) Register(name string, sigs ...Signature) {
	a.registry.Register(name, sigs...)
}

// RegisterStackOnly adds a named word with one or more signatures that
// only match prefix arguments (values already on the stack before the word).
// No forward argument collection is attempted.
//
// Example — register a stack-only word "neg" that negates an integer:
//
//	a.RegisterStackOnly("neg", aql.Signature{
//	    Args: []aql.Type{aql.TInteger},
//	    Handler: func(args []aql.Value, _ map[string]aql.Value, _ []aql.Value, _ *engine.Registry) ([]aql.Value, error) {
//	        n := args[0].AsInteger()
//	        return []aql.Value{aql.NewInteger(-n)}, nil
//	    },
//	})
func (a *AQL) RegisterStackOnly(name string, sigs ...Signature) {
	a.registry.RegisterStackOnly(name, sigs...)
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

	a.registry.Source = src
	eng := engine.NewTop(a.registry)
	eng.SetSource(src)
	result, err := eng.Run(values)
	if err != nil {
		return nil, err
	}

	out := make([]any, len(result))
	for i, v := range result {
		switch {
		case v.VType.Matches(engine.TInteger):
			n, _ := v.AsInteger()
			out[i] = n
		case v.VType.Matches(engine.TString):
			s, _ := v.AsString()
			out[i] = s
		default:
			out[i] = v.String()
		}
	}
	return out, nil
}

// CheckResult is the outcome of a static type-check run.
//
// Stack holds the carrier values left on the stack after symbolic
// execution (one per residual result), represented as their type
// path strings. Diagnostics holds any findings the checker recorded
// (e.g. missing return-type annotations). Summary captures a count
// per severity so callers can quickly decide pass/fail without
// walking the diagnostics slice.
type CheckResult struct {
	Stack       []string          `json:"stack"`
	Diagnostics []CheckDiagnostic `json:"diagnostics"`
	Summary     CheckSummary      `json:"summary"`
}

// CheckSummary reports the per-severity count of diagnostics from
// a check run. Errors > 0 means the program has at least one type
// violation the runtime will trip on.
type CheckSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
}

// CheckSeverity classifies a diagnostic.
type CheckSeverity = engine.CheckSeverity

// Re-exported severity constants.
const (
	SeverityError   = engine.SeverityError
	SeverityWarning = engine.SeverityWarning
	SeverityInfo    = engine.SeverityInfo
)

// CheckDiagnostic is a single finding from the static checker.
type CheckDiagnostic = engine.CheckDiagnostic
