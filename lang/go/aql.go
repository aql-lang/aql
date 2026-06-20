package lang

import (
	"errors"
	"io"

	"github.com/aql-lang/aql/eng/go"

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
	if es := a.registry.Check.Emit; es != nil && len(es.SiteCounts) > 0 {
		res.SiteCounts = make(map[string]int, len(es.SiteCounts))
		for k, v := range es.SiteCounts {
			res.SiteCounts[k] = v
		}
	}
	if runErr != nil {
		return nil, "check error", res, runErr
	}
	for _, d := range res.Diagnostics {
		if d.Severity == SeverityError {
			return nil, "check diagnostics", res, nil
		}
	}
	// Some words are deliberately lenient in check mode but raise a
	// strict error at runtime (an orphan `gen [...]`, an `unpack` of a
	// missing key). The compiled stream IS the check pass, so it would
	// silently succeed where the interpreter errors. Such a word flags
	// the suppression; refuse to compile and let the interpreter raise
	// the real error on the fallback path.
	if a.registry.Check.SuppressedRuntimeError {
		return nil, "check-mode suppressed a runtime error (uncompilable)", res, nil
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

// FnParam describes one parameter of a function signature — re-exported so
// hosts can declare a mini-language kind's stack Inputs (see MiniLangSpec).
type FnParam = native.FnParam

// Handler is the signature of a native word / mini-language handler:
// func(args, ctx, stack, registry) (results, error). args are delivered in
// signature order (args[0] = sig position 0).
type Handler = native.Handler

// MiniLangSpec describes a Go-implemented mini-language kind for
// RegisterMiniLang. The standard [src:String opts:Map] prefix is supplied
// automatically; declare only the extra stack Inputs, the Returns, and the
// Handler.
type MiniLangSpec = modules.MiniLangSpec

// RegisterMiniLang installs a Go-implemented mini-language kind on the
// instance — the embedder twin of the AQL `MiniLang.register` word. After
// the program imports "aql:minilang", the kind is callable as
// `mini <Name> <src> <opts>` exactly like a built-in (`re`, `bf`).
// Registration may precede or follow the import.
//
// The kind name must be a lowercase atom with no `lang_` prefix, the
// handler must be non-nil, and the name must not collide with an existing
// kind (built-in or host). The handler receives args[0]=src, args[1]=opts,
// and args[2..]=the declared Inputs.
//
// Example — an integer binary op whose operands name keys in opts:
//
//	a.RegisterMiniLang(lang.MiniLangSpec{
//	    Name:    "iop",
//	    Returns: []*lang.Type{lang.TInteger},
//	    Handler: iopHandler, // parses "x + y", applies to opts.x, opts.y
//	})
//	a.Run(`"aql:minilang" import end  mini iop 'x + y' {x:10, y:2}`) // → 12
func (a *AQL) RegisterMiniLang(spec MiniLangSpec) error {
	return modules.RegisterHostMiniLang(a.registry, spec)
}

// ParseLangSpec describes a Go-implemented parser for RegisterParser. The
// standard [source:String opts:Map] prefix is supplied automatically and the
// source is resolved to a String before the handler runs; declare only the
// Returns and the Handler.
type ParseLangSpec = modules.ParseLangSpec

// RegisterParser installs a Go-implemented parser on the instance — the
// embedder twin of the AQL `ParseLang.register` word, and the sibling of
// RegisterMiniLang. After the program imports "aql:parselang", the parser is
// callable as `parse <Name> <opts?> <source>`; it returns whatever the
// language yields (an AST, a transduction, …), typed Any. Registration may
// precede or follow the import.
//
// The kind name must be a lowercase atom with no `parse_` prefix, the handler
// must be non-nil, and the name must not collide with an existing parser. The
// handler receives args[0]=source (a resolved String) and args[1]=opts.
//
// Example — a calc parser that returns an AST instead of evaluating:
//
//	a.RegisterParser(lang.ParseLangSpec{
//	    Name:    "calc",
//	    Returns: []*lang.Type{lang.TMap},
//	    Handler: calcParseHandler, // 'x + y' → {op:'+', left:'x', right:'y'}
//	})
//	a.Run(`"aql:parselang" import end  parse calc 'x + y'`) // → {op:'+' …}
func (a *AQL) RegisterParser(spec ParseLangSpec) error {
	return modules.RegisterHostParser(a.registry, spec)
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
//
// LIMITATION — the step budget is the one place this is NOT byte-for-byte
// transparent. The interpreter meters its DefaultStepLimit per tape token
// stepped; the VM meters the SAME cap per bytecode instruction. The compiled
// stream is leaner than the expanded token walk, so for any given program the
// VM reaches at least as far as the interpreter before the cap — the divergence
// is one-directional: a long-but-terminating computation that the interpreter
// would abort with evaluation_limit may COMPLETE under compilation; the reverse
// never happens (the VM does not spuriously raise evaluation_limit on a program
// the interpreter finishes). A genuine runaway trips evaluation_limit fast in
// both. So at the ceiling, Run and RunCompiled are observably different
// programs; everywhere below it they agree. TestStepBudgetNoSpuriousLimit pins
// the agreement on a long terminating loop; TestPropertyDifferential keeps the
// generated corpus well under the cap so the divergence never makes it flaky.
//
// Two boundaries qualify "identical results":
//
//   - Internal errors. A compiled-mode VM/lowering soundness assertion or a
//     recovered handler panic (taxonomy internal_error), and any non-AQL Go
//     error, are NOT surfaced: the run rolls back and re-executes on the
//     interpreter, so a latent compiler bug degrades to the correct result
//     rather than a raw failure (the differential gate's row-count floor still
//     catches the regression).
//   - The step budget. The interpreter counts it per tape token stepped, the
//     VM per bytecode instruction, both capped at eng.DefaultStepLimit. Only
//     iteration/recursion can approach that ceiling, and for those the compiled
//     stream is leaner than the expanded token stream — so the VM reaches at
//     least as far as the interpreter and never spuriously raises
//     evaluation_limit on a program the interpreter completes. The residual
//     deviation is benign and one-directional: a long computation the
//     interpreter reports as evaluation_limit may COMPLETE under compilation.
//     A genuine runaway trips evaluation_limit fast in both (the VM does not
//     fall back on it — that would only re-burn the same budget).
func (a *AQL) RunCompiled(src string) ([]any, bool, error) {
	// CompileCheck executes the program in check mode, so its
	// RunInCheckMode words (def/import/type/macro, the Test harness)
	// leave real side effects on the registry. The COMPILED path needs
	// those to persist (OpPushType resolves minted IDs; islands re-run
	// through a sub-engine over the same registry). But the interpreter
	// FALLBACK re-runs the whole source, so it must NOT see them or it
	// double-applies a re-mint / re-import / re-run Test spec. Snapshot
	// the mutable scopes before the check pass and roll them back on the
	// fallback path; keep them on the compiled path.
	snap := a.registry.SnapshotForCompile()
	prog, _, _, err := a.CompileCheck(src)
	if err != nil || prog == nil {
		a.registry.RestoreForCompile(snap)
		out, rerr := a.Run(src)
		return out, false, rerr
	}
	result, err := eng.RunProgram(prog, a.registry)
	if err != nil {
		// An INTERNAL compiled-mode error — a VM/lowering soundness assertion
		// or a recovered handler panic (both carry code internal_error), or any
		// non-AQL Go error — must never reach the caller as a raw compiler bug.
		// Roll the registry back to the pre-check state (exactly as the
		// uncompilable path does) and let the interpreter render the canonical
		// result. Genuine AQL runtime errors (type_error, div-by-zero, and the
		// resource ceilings evaluation_limit / tape_exhausted) match the
		// interpreter by the differential gate and are returned as-is — the
		// resource limits in particular fail FAST in both engines by design
		// (see the step-budget note above), so re-running the interpreter would
		// only burn the same budget again.
		if runtimeShouldFallback(err) {
			a.registry.RestoreForCompile(snap)
			out, rerr := a.Run(src)
			return out, false, rerr
		}
		return nil, true, err
	}
	return convertResults(result), true, nil
}

// RunCompiledStrict is RunCompiled in FORCE mode: it REQUIRES the bytecode
// path. Where RunCompiled silently falls back to the interpreter for a program
// the emitter cannot lower (or a VM/lowering soundness assertion), RunCompiledStrict
// surfaces that as an error instead — the returned message carries the emitter's
// refusal reason, or the VM's internal_error. Use it to GUARANTEE a program ran
// through the compiler (verifying the compilable subset, benchmarking the VM in
// isolation, or catching a compiler regression that would otherwise hide behind
// the fallback). Genuine AQL runtime errors (type_error, div-by-zero, the
// resource ceilings) are returned as-is, exactly as RunCompiled returns them.
//
// Side-effect parity matches RunCompiled: on the compiled path the check pass's
// RunInCheckMode words (def/import/type/macro) persist; on every error path the
// registry is rolled back to its pre-check state.
func (a *AQL) RunCompiledStrict(src string) ([]any, error) {
	snap := a.registry.SnapshotForCompile()
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		a.registry.RestoreForCompile(snap)
		return nil, err
	}
	if prog == nil {
		a.registry.RestoreForCompile(snap)
		if reason == "" {
			reason = "program is not compilable"
		}
		return nil, errors.New("force-compile: " + reason)
	}
	result, err := eng.RunProgram(prog, a.registry)
	if err != nil {
		// Force mode does NOT fall back: surface the error (including an
		// internal_error, which under RunCompiled would silently re-run the
		// interpreter) so a compiler bug is visible rather than masked.
		return nil, err
	}
	return convertResults(result), nil
}

// runtimeShouldFallback reports whether a compiled-mode RUN error should be
// resolved by re-running on the interpreter rather than surfaced. True for an
// internal_error (a VM/lowering soundness assertion or a recovered handler
// panic — never surface a raw compiler bug; the interpreter is the correctness
// backstop) and any non-AQL (foreign) error. False for every genuine AQL
// runtime error — type_error, div-by-zero, and the resource ceilings
// (evaluation_limit / tape_exhausted) — which the differential gate proves
// match the interpreter, and which the VM deliberately surfaces fast rather
// than hanging or double-running.
func runtimeShouldFallback(err error) bool {
	var ae *eng.AqlError
	if !errors.As(err, &ae) {
		return true
	}
	return ae.Code == "internal_error"
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
	// SiteCounts tallies dispatch sites by compilation class during the
	// bytecode recording pass — "mono" (a single resolved signature,
	// compiles to CALL_NATIVE), "poly" (polymorphic, not lowered),
	// "dynamic" (a dynamic carrier / interpreter-island site), and
	// "meta" (compile-time / fn-invoking / higher-order words). Populated
	// only by CompileCheck (the recording pass); nil for a plain Check.
	// It answers "why didn't my hot loop compile to a single path?".
	SiteCounts map[string]int `json:"site_counts,omitempty"`
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
