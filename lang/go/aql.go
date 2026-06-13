package lang

import (
	"github.com/aql-lang/aql/eng/go"
	"io"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/policy"

	udk "voxgiguniversalsdk"
)

// Policy is the public alias for the permissions policy interface.
// Callers construct one via policy.Load / LoadFile / LoadInline /
// LoadAuto / FromMap and pass it through Options.
type Policy = policy.Policy

// FileOps is the interface for file system operations used by read/write words.
type FileOps = capabilities.FileOps

// Format handles encoding and decoding file content for a specific format.
type Format = native.Format

// Type represents an AQL type such as "string", "number/integer", or "any".
// Use NewType to create types from slash-separated paths.
type Type = native.Type

// Value is a typed entry on the AQL stack.
type Value = native.Value

// Signature describes one way a function can be called.
// Args lists the types the word needs, ordered deepest-first (Args[0] = deepest
// on the stack, Args[last] = top of the stack for prefix matching).
//
// Handler receives the matched args, the current context map, the resolved
// stack (only for FullStack signatures), and the registry. Most handlers
// only use the first parameter and ignore the rest with _.
type Signature = native.Signature

// Well-known AQL types for use in Signature definitions.
var (
	TAny            = native.TAny
	TScalar         = native.TScalar
	TString         = native.TString
	TNumber         = native.TNumber
	TInteger        = native.TInteger
	TFloat          = native.TFloat
	TBoolean        = native.TBoolean
	TNode           = native.TNode
	TAtom           = native.TAtom
	TList           = native.TList
	TMap            = native.TMap
	TTable          = native.TTable
	TRecord         = native.TRecord
	TResource       = native.TResource
	TResourceEntity = native.TResourceEntity
)

// NewType creates a *Type from a slash-separated path (e.g. "string/proper",
// "number/integer"). Use this for custom or hierarchical types.
var NewType = native.NewType

// NewString creates a string Value.
var NewString = native.NewString

// NewInteger creates a number/integer Value.
var NewInteger = native.NewInteger

// NewBoolean creates a boolean Value.
var NewBoolean = native.NewBoolean

// NewList creates a list Value from a slice of Values.
var NewList = native.NewList

// NewMap creates a map Value from an OrderedMap.
var NewMap = native.NewMap

// NewAtom creates an atom Value from a bare name.
var NewAtom = native.NewAtom

// NewMemFileOps creates an in-memory file system for testing.
func NewMemFileOps() *capabilities.MemFileOps {
	return capabilities.NewMem()
}

// Options configures an AQL instance.
type Options struct {
	// Registry is a string identifier for the registry to use.
	Registry string
	// Seed sets the random seed for ID generation.
	// If zero, the current time is used.
	Seed int64
	// Policy is the optional permissions profile applied to this
	// instance. Nil means "no permissions configured" — the engine
	// and every capability wrapper treat that as allow-everything,
	// preserving the historical default. When set, the policy is
	// installed before host capabilities so SetHostX hooks can
	// auto-wrap or skip-install per the profile.
	Policy policy.Policy
	// Tape bounds the execution tape's growth (initial size, max grows,
	// growth factor). The zero value uses the engine defaults. Set via
	// the CLI's --options flag (e.g. `--options tape:initial:65536`) or
	// directly by a host.
	Tape TapeOptions
}

// TapeOptions configures the execution tape's bounded growth — see
// eng/go/tape.go. InitialSize 0 derives from the program; MaxGrows 0 and
// GrowthFactor 0 use the defaults (7 and 2.7).
type TapeOptions = native.TapeConfig

// AQL is an independent AQL execution instance.
// Each instance has its own state (set/get storage is isolated).
// Create multiple instances with New() for independent execution contexts.
//
// AQL is not safe for concurrent use. (*AQL).Run and (*AQL).Check both
// mutate the underlying Registry (source pointer, def/type stacks, check
// state) and must not be called from multiple goroutines simultaneously
// on the same instance. Use one instance per goroutine for parallel work.
type AQL struct {
	registry *native.Registry
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
		native.SetIDSeed(o.Seed)
	}

	reg, err := native.DefaultRegistryWithPolicy(o.Policy)
	if err != nil {
		return nil, err
	}
	reg.TapeConfig = o.Tape
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)

	um := udk.NewUniversalManager(map[string]any{
		"registry": o.Registry,
	})

	reg.Manager = um

	// Enable dynamic help generation for functions registered after this point.
	native.EnableDynamicHelp(reg)
	reg.MarkReady()

	return &AQL{registry: reg, options: o, manager: um}, nil
}

// Options returns the Options the instance was created with.
func (a *AQL) Options() Options {
	return a.options
}

// Policy returns the policy installed on this instance, or nil if
// none was configured. Equivalent to a.Options().Policy but reads
// the live capability slot, so it reflects any subsequent
// SetHostPolicy calls (rare; intended for tooling).
func (a *AQL) Policy() Policy {
	return native.HostPolicy(a.registry)
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
	defer a.registry.Check.Begin()()

	eng := native.NewTop(a.registry)
	eng.SetSource(src)
	result, err := eng.Run(values)
	// Drop fn-body forward-reference false positives (the name is
	// defined by now), then emit unused-def warnings — both need the
	// fully-populated end-of-pass state.
	a.registry.RescueForwardRefDiagnostics()
	a.registry.Check.EmitUnusedDefDiagnostics()
	if err != nil {
		return CheckResult{Diagnostics: a.registry.Check.Diagnostics}, err
	}

	stack := make([]string, len(result))
	for i, v := range result {
		if v.Dynamic {
			// Surface the gradual modality in the residual stack so a
			// dynamic carrier is distinguishable from a strict one.
			stack[i] = "dynamic(" + v.Parent.Leaf() + ")"
		} else {
			stack[i] = v.Parent.Leaf()
		}
	}

	// Diagnostics carry the source position stamped by the parser onto
	// the offending value (Row 0 means the engine could not attribute the
	// diagnostic to a source token). We do not guess a location by
	// text-searching the source — a guessed position is wrong whenever the
	// word appears more than once.
	diags := a.registry.Check.Diagnostics
	var summary CheckSummary
	for i := range diags {
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

// Program is the bytecode unit the compile pass produces — re-exported
// from the engine kernel for host callers (Stage 1 of
// design/aql-bytecode-plan.0.md).
type Program = eng.Program

// CompileCheck runs the source through the checker with the bytecode
// recording pass enabled (Stage 1: straight-line, monomorphic native
// calls only) and linearises the trace into a Program. When the
// source contains a construct Stage 1 cannot lower — control flow,
// user fns, polymorphic or dynamic dispatch, compile-time words —
// the Program is nil and reason names the first offender; the
// CheckResult is valid either way.
func (a *AQL) CompileCheck(src string) (*Program, string, CheckResult, error) {
	values, err := parser.Parse(src)
	if err != nil {
		return nil, "parse error", CheckResult{}, err
	}

	a.registry.Source = src
	defer a.registry.Check.Begin()()
	a.registry.Check.Emit = eng.NewEmitState()
	// Fn-body analyses must run (and record) under THIS emit pass —
	// a summary cached by an earlier plain Check on the same instance
	// would skip the body and leave its compiled unit empty.
	a.registry.Check.FnSummaries = nil
	a.registry.Check.FnInflight = nil

	engine := native.NewTop(a.registry)
	engine.SetSource(src)
	residual, runErr := engine.Run(values)
	a.registry.RescueForwardRefDiagnostics()
	a.registry.Check.EmitUnusedDefDiagnostics()

	res := CheckResult{Diagnostics: a.registry.Check.Diagnostics}
	if runErr != nil {
		return nil, "check error", res, runErr
	}
	for _, d := range res.Diagnostics {
		if d.Severity == SeverityError {
			return nil, "check diagnostics", res, nil
		}
	}
	prog, reason, ok := a.registry.Check.Emit.Finalize(residual)
	if !ok {
		return nil, reason, res, nil
	}
	return prog, "", res, nil
}

// SetFileOps replaces the file operations implementation used by read/write.
func (a *AQL) SetFileOps(ops FileOps) {
	native.SetHostFileOps(a.registry, ops)
}

// Clock is the time source used by temporal and random words —
// re-exported so hosts and tests can freeze it (capabilities.FixedClock)
// for reproducible runs.
type Clock = capabilities.Clock

// SetClock replaces the instance's clock.
func (a *AQL) SetClock(clk Clock) {
	native.SetHostClock(a.registry, clk)
}

// SetOutput replaces the writer used by print, help, and other output words.
func (a *AQL) SetOutput(w io.Writer) {
	a.registry.Output = w
}

// RegisterFormat adds or replaces a format in the format registry.
// Formats are used by read/write words via the {fmt:"name"} option.
func (a *AQL) RegisterFormat(name string, f Format) {
	formats := native.HostFormats(a.registry)
	if formats == nil {
		formats = make(map[string]native.Format)
		native.SetHostFormats(a.registry, formats)
	}
	formats[name] = f
}

// Register adds a named word with one or more signatures. Any sig
// whose BarrierPos is left at 0 is treated as forward-collecting:
// the engine collects arguments from tokens after the word before
// falling back to stack matching.
//
// Example — register a word "double" that doubles an integer
// (extra handler params are context, stack, and registry — use _ to ignore):
//
//	a.Register("double", lang.Signature{
//	    Args: []lang.Type{lang.TInteger},
//	    Handler: func(args []lang.Value, _ map[string]lang.Value, _ []lang.Value, _ *native.Registry) ([]lang.Value, error) {
//	        n := args[0].AsConcreteInteger()
//	        return []lang.Value{lang.NewInteger(n * 2)}, nil
//	    },
//	})
func (a *AQL) Register(name string, sigs ...Signature) {
	a.registry.Register(name, sigs...)
}

// RegisterNativeFunc installs a full NativeFunc (name + signatures) on the
// instance's registry. Convenience for callers that already hold a
// native.NativeFunc value (e.g. seeding the words that moved into loadable
// modules into a test instance without an explicit import).
func (a *AQL) RegisterNativeFunc(n native.NativeFunc) {
	a.registry.RegisterNativeFunc(n)
}

// (RegisterStackOnly was retired. To install a stack-only word, set
// `BarrierPos: 0` on each Signature and call `Register` — that's the
// canonical encoding of "this sig consumes its args from the prefix
// stack only.")

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
	eng := native.NewTop(a.registry)
	eng.SetSource(src)
	result, err := eng.Run(values)
	if err != nil {
		return nil, err
	}
	return convertResults(result), nil
}

// convertResults maps a residual engine stack to host-friendly Go
// values — the same projection Run has always applied.
func convertResults(result []eng.Value) []any {
	out := make([]any, len(result))
	for i, v := range result {
		switch {
		case v.IsDepScalar():
			// DepScalar's Parent IS the base scalar (TInteger,
			// TString, …) post the type-collapse, so it would
			// otherwise hit the AsInteger/AsString branches below and
			// pull a zero value from the DepScalarInfo payload. Route
			// through String() to render as "(Integer gte 5)".
			out[i] = v.String()
		case native.IsBareTypeNode(v):
			// Type literal (e.g. `typeof x` result, a bare `Integer`
			// or user-minted Foo literal): render its type name
			// rather than trying to extract an Integer/String payload
			// that isn't there.
			out[i] = v.String()
		case v.Parent.ConformsTo(native.TInteger):
			n, _ := native.AsInteger(v)
			out[i] = n
		case v.Parent.ConformsTo(native.TString):
			s, _ := native.AsString(v)
			out[i] = s
		default:
			out[i] = v.String()
		}
	}
	return out
}

// RunCompiled executes src in compiled (bytecode) mode when the
// emitter can lower it, and SILENTLY falls back to the interpreter
// otherwise — the plan's opt-in contract: identical results either
// way, the flag only changes the execution engine. The second return
// reports which path ran (for tooling; never branch program logic on
// it).
func (a *AQL) RunCompiled(src string) ([]any, bool, error) {
	prog, _, _, err := a.CompileCheck(src)
	if err != nil || prog == nil {
		out, rerr := a.Run(src)
		return out, false, rerr
	}
	result, err := eng.RunProgram(prog, a.registry)
	if err != nil {
		return nil, true, err
	}
	return convertResults(result), true, nil
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
type CheckSeverity = native.CheckSeverity

// Re-exported severity constants.
const (
	SeverityError   = native.SeverityError
	SeverityWarning = native.SeverityWarning
	SeverityInfo    = native.SeverityInfo
)

// CheckDiagnostic is a single finding from the static checker.
type CheckDiagnostic = native.CheckDiagnostic
