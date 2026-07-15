package lang

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/aql-lang/aql/eng/go"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/policy"

	udk "github.com/voxgig/udk/go"
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

// NewTypeLiteral creates the type-literal Value for a *Type — e.g. as an
// alternative for DefineEnum, or a value standing for a type.
var NewTypeLiteral = native.NewTypeLiteral

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

	reg, err := newDefaultRegistryWithPolicy(o.Policy)
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

// NewFromRegistry wraps an ALREADY-WIRED registry in an *AQL instance so a
// host with a long-lived registry of its own — the REPL, a service — can use
// the compiled-by-default entry points (RunAutoValues / RunCompiledReason)
// over it. The caller owns the wiring (parse func, module resolver, Manager,
// Output, policy): nothing is installed or re-installed here, and the zero
// Options apply. Plan Phase 2 prerequisite (entry-point routing).
func NewFromRegistry(reg *native.Registry) (*AQL, error) {
	if reg == nil {
		return nil, fmt.Errorf("NewFromRegistry: nil registry")
	}
	return &AQL{registry: reg}, nil
}

// Options returns the Options the instance was created with.
func (a *AQL) Options() Options {
	return a.options
}

// NativeRegistry returns the live *native.Registry backing this instance.
// Intended for tooling that needs to introspect or serve the running
// runtime's state — notably the debug-attach server (lang/go/debugserve),
// which wraps a registry behind authenticated HTTP introspection. Most
// callers should use the higher-level Run/Check API instead; this is the
// escape hatch for host-level inspection tools.
func (a *AQL) NativeRegistry() *native.Registry {
	return a.registry
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
// SetStrictCheck toggles STRICT check mode for subsequent Check calls:
// every committed dispatch over a dynamic operand emits a non-gating
// dynamic_dispatch info diagnostic, making the gradual frontier loud —
// the Typed-Racket-style migration surface
// (design/checker-accuracy-review.10.md). Persistent on the instance
// until toggled off.
func (a *AQL) SetStrictCheck(on bool) {
	if a.registry.Check.Strict != on {
		// Fn-body summaries memoised under the OTHER strictness suppress
		// re-analysis on a reused instance (a bare Begin keeps them), so a
		// body's per-dispatch dynamic_dispatch advisories would never
		// surface after the toggle — drop the memo so the next Check
		// re-analyses under the new mode.
		a.registry.Check.FnSummaries = nil
		a.registry.Check.FnInflight = nil
	}
	a.registry.Check.Strict = on
}

func (a *AQL) Check(src string) (CheckResult, error) {
	values, err := parser.Parse(src)
	if err != nil {
		return CheckResult{}, err
	}

	a.registry.Source = src
	defer a.registry.Check.Begin()()
	// Deferred parse kinds are per-check-run state: clear any left over from a
	// prior Check on this reused instance so a now-unregistered `parse <kind>`
	// reports parse_unknown_lang instead of silently degrading to dynamic.
	native.ResetParseDeferredKinds(a.registry)
	native.ResetModuleExportGrowth(a.registry)

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

// StampEvent is one detached-stamp attempt (re-exported for hosts and the
// CLI's -compile-report surface).
type StampEvent = eng.StampEvent

// StampReport returns the detached-stamp attribution recorded on this
// instance's registry (design/RUNTIME-STAMPING.0.md Phase 5): one event per
// stamp ATTEMPT — runtime-constructed codec fns, service handlers, and
// module fns — with the refusal reason when the compile declined. Nil when
// runtime stamping was never armed (a plain Run / -no-compile execution).
func (a *AQL) StampReport() []eng.StampEvent {
	return a.registry.StampEvents()
}

// InterpEntry / BailEvent are the observability-seam event types (eng
// interp_entry.go), re-exported for the frontier test suite.
type InterpEntry = eng.InterpEntry

// BailEvent is one designed VM defer-to-interpreter (see InterpEntry).
type BailEvent = eng.BailEvent

// ArmInterpEntryHook forwards to the registry's interpreter-entry
// observability seam (eng interp_entry.go — a TEST seam, not API): fn fires
// on every entry into tree-walking machinery until the returned disarm func
// runs.
func (a *AQL) ArmInterpEntryHook(fn func(InterpEntry)) func() {
	return a.registry.ArmInterpEntryHook(fn)
}

// ArmRuntimeBailHook forwards to the registry's runtime-bail observability
// seam (eng interp_entry.go — a TEST seam, not API): fn fires on every
// designed VM defer-to-interpreter until the returned disarm func runs.
func (a *AQL) ArmRuntimeBailHook(fn func(BailEvent)) func() {
	return a.registry.ArmRuntimeBailHook(fn)
}

// CompileCheck runs the source through the checker with the bytecode
// recording pass enabled (Stage 1: straight-line, monomorphic native
// calls only) and linearises the trace into a Program. When the
// source contains a construct Stage 1 cannot lower — control flow,
// user fns, polymorphic or dynamic dispatch, compile-time words —
// the Program is nil and reason names the first offender; the
// CheckResult is valid either way.
func (a *AQL) CompileCheck(src string) (*Program, string, CheckResult, error) {
	// SECURITY GATE: compiled dispatch does not consult the engine word
	// policy — the interpreter's policyGateWord runs per stepWord dispatch
	// (and deliberately not in check mode), so a compiled program would
	// BYPASS every word deny rule (found 2026-07-13: a "deny add" policy
	// interpreted `1 add 2` to permission-denied but ran it compiled to 3).
	// A policy-gated registry therefore refuses compilation outright and the
	// interpreter, where the gate lives, owns every dispatch. Lifting this
	// requires a VM-side policy gate at CALL_NATIVE/CALL_USER/poly dispatch
	// (recorded as a Phase 10 item in the completion plan).
	if eng.LookupWordChecker(a.registry) != nil {
		return nil, "policy-gated registry (compiled dispatch does not consult word rules)", CheckResult{}, nil
	}
	values, err := parser.Parse(src)
	if err != nil {
		return nil, "parse error", CheckResult{}, err
	}

	a.registry.Source = src
	// BeginCompilePass arms the shared compile-pass ritual (fresh
	// EmitState, Compiling flag, fn-memo drop) in one place.
	defer a.registry.Check.BeginCompilePass()()
	// Per-check-run reset (see Check): a reused instance must not inherit a
	// prior pass's deferred parse kinds.
	native.ResetParseDeferredKinds(a.registry)
	native.ResetModuleExportGrowth(a.registry)

	engine := native.NewTop(a.registry)
	engine.SetSource(src)
	residual, runErr := engine.Run(values)
	a.registry.RescueForwardRefDiagnostics()
	a.registry.Check.EmitUnusedDefDiagnostics()

	res := CheckResult{Diagnostics: a.registry.Check.Diagnostics}
	if sites := a.registry.Check.Recorder().Sites(); len(sites) > 0 {
		res.SiteCounts = make(map[string]int, len(sites))
		for k, v := range sites {
			res.SiteCounts[k] = v
		}
	}
	if runErr != nil {
		return nil, "check error", res, runErr
	}
	for _, d := range res.Diagnostics {
		// Refuse only on MODEL-UNDERMINING findings (undefined_word,
		// no_signature, … — dispatch did not resolve, so the recording is
		// a guess). A RuntimeMirror error is a validation finding whose
		// model is exact — the program compiles and raises the identical
		// error at runtime (trap / VM RET / the same handler) — so it
		// must not cost compile coverage. A CAUGHT model-undermining
		// finding still undermines the model: the `do` body's contents
		// were recorded from the same guess (the runtime catches the
		// error, but the compiled region computes a different value), so
		// the caught downgrade must not slip it past this gate.
		if !d.RuntimeMirror && (d.Severity == SeverityError || d.CaughtAtRuntime) {
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
	// A dispatch whose forward/stack split depended on a genuinely mixed
	// gradual carrier (e.g. `and 0 false not 0`, where `and`'s
	// Disjunct(Integer,Boolean) result makes `not` forward-collect in
	// check mode but stack-grab the concrete Boolean at runtime) cannot be
	// faithfully compiled — the static split diverges from the runtime
	// one. Refuse and fall back to the interpreter.
	if a.registry.Check.AmbiguousGradualSplit {
		return nil, "forward/stack split depends on a gradual operand (uncompilable)", res, nil
	}
	prog, reason, ok := a.registry.Check.Recorder().Finalize(residual)
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

// RegisterFormat adds or replaces a format in the format registry and maps
// any given file extensions (leading dot optional) to it. Formats are used
// by the read/write words via the {fmt:"name"} option and, for any mapped
// extensions, by `read` on a matching path.
func (a *AQL) RegisterFormat(name string, f Format, exts ...string) {
	if native.HostFormats(a.registry) == nil {
		native.SetHostFormats(a.registry, make(map[string]native.Format))
	}
	_ = native.RegisterFormat(a.registry, name, f, exts...)
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

// DefineType installs a user type from a body Value by the SAME path the
// `def Name body` word uses (eng.InstallType), and returns the minted
// type handle for use in Register'd signatures. This is the embedding-API
// counterpart of running `def`, but with the *Type handed back — closing
// the gap where an embedder could define a type in source yet never
// obtain its handle.
//
// `body` is an ordinary type body: a bare type literal (alias), a refine
// prefab (newtype), a disjunct (DefineEnum / NewDisjunct — union/enum), a
// negation, a record/object/schema body, etc. For a body expressed in AQL
// syntax, use DefineTypeFromSource; for a membership rule expressed as a
// Go func, use DefineMemberType.
func (a *AQL) DefineType(name string, body Value) (*Type, error) {
	return a.registry.DefineType(name, body)
}

// DefineEnum installs `name` as the union/enumeration of the given
// alternatives — the embedding equivalent of `def Name (v0 tor v1 …)`.
// Alternatives may be concrete values (a closed enum) or type literals
// (a union). Returns the minted type handle.
func (a *AQL) DefineEnum(name string, alternatives ...Value) (*Type, error) {
	return a.registry.DefineEnum(name, alternatives...)
}

// DefineMemberType installs `name` as the type whose inhabitants are the
// concrete values satisfying member — a membership rule expressed as a Go
// func (the one case DefineType's body path cannot express). The name is
// bound (resolves in source and exports like a `def`-installed type) and
// the minted handle returned.
func (a *AQL) DefineMemberType(name string, parent *Type, member func(v Value) bool) (*Type, error) {
	return a.registry.DefineMemberType(name, parent, member)
}

// DefineTypeFromSource installs `name` with a body written in AQL syntax
// — it runs `def Name <bodySource>` on the instance and returns the
// minted type handle. The most direct embedding form: the host writes the
// body exactly as it would in a script (e.g.
// DefineTypeFromSource("Point", "refine Record [x:Integer y:Integer]"))
// and gets the *Type back to thread into Register'd signatures.
func (a *AQL) DefineTypeFromSource(name, bodySource string) (*Type, error) {
	// `name` is composed into a `def` program, so it must be a plain type
	// identifier — never source. Reject anything else BEFORE running, so a
	// name like `X end <code>` cannot terminate the def and execute
	// trailing source against the registry. (`bodySource` is intentionally
	// source; `name` is not.) The clash / part checks still run inside
	// InstallType when the def executes.
	if !validTypeIdent(name) {
		return nil, errors.New("define type: invalid type name " + strconv.Quote(name) +
			" (must be a capitalised identifier of letters, digits, '_', '-', '/')")
	}
	if _, err := a.Run("def " + name + " " + bodySource); err != nil {
		return nil, err
	}
	if t := a.registry.LookupTypeName(name); t != nil {
		return t, nil
	}
	return nil, errors.New("define type " + name + ": installed but did not resolve")
}

// validTypeIdent reports whether name is a safe type identifier to splice
// into a `def` program: capitalised, and otherwise only letters, digits,
// and the path/word separators '_', '-', '/'. This excludes whitespace
// and every AQL-significant character (quotes, parens, brackets, ';',
// 'end', …), so a name can never break out of the def-name position.
func validTypeIdent(name string) bool {
	if name == "" || name[0] < 'A' || name[0] > 'Z' {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '_', c == '-', c == '/':
		default:
			return false
		}
	}
	return true
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
//	a.Run(`import "aql:minilang"  mini iop 'x + y' {x:10, y:2}`) // → 12
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
//	a.Run(`import "aql:parselang"  parse calc 'x + y'`) // → {op:'+' …}
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
//
// Run currently executes on the tree-walking interpreter (RunInterp);
// Stage J of the runtime-independence plan flips it to the compiled path,
// with RunInterp retained as the differential oracle. Callers that need
// the interpreter SPECIFICALLY — parity oracles, canonical-error
// rendering — must call RunInterp so the flip cannot silently change what
// they measure.
func (a *AQL) Run(src string) ([]any, error) {
	return a.RunInterp(src)
}

// RunInterp parses and executes src on the TREE-WALKING INTERPRETER,
// unconditionally — never the bytecode VM. It is the differential oracle
// the compiled path is measured against (byte-identical values, errors,
// and output), and it survives Stage J's Run flip as the explicitly-named
// interpreter entry point.
func (a *AQL) RunInterp(src string) ([]any, error) {
	result, err := a.runValues(src)
	if err != nil {
		return nil, err
	}
	return convertResults(result), nil
}

// runValues is Run without the host-value projection: the raw engine stack.
// The Value-returning entry points (RunAutoValues' fallback arms) need the
// unprojected Values so a host renderer (the REPL's v.String()) stays
// byte-identical with what the engine produced.
func (a *AQL) runValues(src string) ([]native.Value, error) {
	values, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}

	a.registry.Source = src
	eng := native.NewTop(a.registry)
	eng.SetSource(src)
	return eng.Run(values)
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
//     catches the regression). EXCEPT when observable output already escaped:
//     rolling back cannot un-print, so a re-run would duplicate every effect
//     (the L-DUP class, design/VOXGIG-COMPILE-LEAVES.2.md). The effect fence
//     (eng effects.go, design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md C1)
//     then PROPAGATES the internal_error, annotated with a run-with
//     --no-compile hint, instead of silently re-running.
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
	out, ran, _, err := a.RunCompiledReason(src)
	return out, ran, err
}

// RunCompiledReason is RunCompiled with the whole-program compilation-refusal
// reason surfaced as a third return, for tooling (chiefly the CLI's performance
// warning) that needs to know WHY a run fell back to the interpreter. It runs
// identically to RunCompiled — same results, same ranCompiled, same error — and
// additionally reports:
//
//   - "" when the program ran on the VM (ranCompiled true), or when the fallback
//     was NOT a performance refusal: a statically-invalid program (a parse/check
//     error, or the "check diagnostics" sentinel — it would fail in both engines)
//     and a runtime soundness bailout (an internal_error re-run, a latent
//     compiler bug the differential gate catches, not a compilable-subset gap).
//   - the first offending construct otherwise: a GENUINE whole-program refusal
//     (CompileCheck returned a nil Program with no check error) that silently
//     fell back to the slower interpreter (design/COMPILABLE-SUBSET.md §1 — the
//     refusal is "slow, not wrong"). A refusal is surprising performance debt,
//     so the CLI surfaces this reason as a warning.
func (a *AQL) RunCompiledReason(src string) ([]any, bool, string, error) {
	vals, ran, reason, err := a.RunAutoValues(src)
	if err != nil {
		return nil, ran, reason, err
	}
	return convertResults(vals), ran, reason, nil
}

// RunAutoValues is the compiled-by-default entry point returning the RAW
// engine Values — RunCompiledReason without the host-value projection
// (convertResults collapses Integer→int64/String→string, which cannot back
// a renderer that needs the engine's own Value.String() — the REPL's
// per-line echo and /stack). Identical semantics: same compiled/fallback
// arms, same fence, same reason contract (plan Phase 2's prerequisite for
// entry-point routing).
func (a *AQL) RunAutoValues(src string) ([]native.Value, bool, string, error) {
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
	// C1 effect fence (eng effects.go): a silent interpreter re-run — on
	// either fallback arm below — is sound only while NO observable effect
	// has escaped, because RestoreForCompile rolls back registry scopes but
	// cannot un-print emitted output; a re-run after an effect duplicates it
	// (the L-DUP class the pure-value differential is blind to). Armed BEFORE
	// the check pass, which executes module imports: an import-time effect
	// must count against the refusal arm too.
	//
	// The ledger is PER-REQUEST: a fresh one is installed for this run and
	// the prior one restored on return. Detached work from an EARLIER request
	// (a ForkConcurrent body still printing) captured the ledger pointer live
	// at ITS fork/arm time, so its late effects land on the old ledger and
	// cannot spuriously block THIS request's fallback — while a fork this
	// request spawns (an import-time module body) copies the fresh pointer
	// and counts, exactly the ownership the fence needs. Installed before
	// arming so the writer wrappers capture the fresh ledger.
	savedEffects := a.registry.Effects
	a.registry.Effects = &eng.EffectLedger{}
	defer func() { a.registry.Effects = savedEffects }()
	disarmFence := a.registry.ArmEffectFence()
	defer disarmFence()
	effectsAt := a.registry.Effects.Count()
	// Compiled execution requested: arm detached fn-unit stamping so
	// runtime-constructed callbacks (service handlers, custom codec fns)
	// compile to units at their store sites (eng.StampDetachedFn). The flag
	// stays armed through the interpreter FALLBACK below — the top level then
	// interprets but stored callbacks still earn the VM path, which is the
	// compiled mode's contract. It is RESTORED to its prior state on return
	// (defer) so a compiled-mode request never leaks the armed flag into a
	// later plain Run (-no-compile) on a reused instance; disarming is safe
	// because a callback already stamped keeps its VM path regardless of the
	// flag (InvokeCallback gates on the stored ref). Only this call's own
	// arming is undone: a caller that armed the registry itself keeps it armed.
	wasArmed := a.registry.RuntimeStampingEnabled()
	a.registry.EnableRuntimeStamping()
	if !wasArmed {
		defer a.registry.DisableRuntimeStamping()
	}
	prog, reason, res, err := a.CompileCheck(src)
	if err != nil || prog == nil {
		a.registry.RestoreForCompile(snap)
		// The check pass's in-place module-load stamps were rolled back with the
		// scopes; drop them so -compile-report shows only the fallback re-run's
		// authoritative stamps, not each rolled-back stamp twice.
		a.registry.ResetStampLog()
		// C1 fence on the REFUSAL arm: the check pass executed module imports
		// (and any other RunInCheckMode word) for real — if one of them emitted
		// an observable effect, re-running the whole source would emit it
		// twice. A statically-invalid program still surfaces its own verdict —
		// the check error, or the first ERROR-severity model-undermining
		// diagnostic behind the "check diagnostics" sentinel: that program
		// fails identically in both engines, so the diagnostic IS the truthful
		// result. A CaughtAtRuntime diagnostic is deliberately NOT surfaced
		// here even though it also raises the sentinel: it was downgraded
		// because a surrounding `do [...]` catches the failure — the
		// interpreter would CONTINUE with the handler's result — so reporting
		// it as the program's own error would be wrong; it falls through to
		// the honest blocked-fallback internal_error below, like any other
		// refusal the fence cannot resolve.
		if a.registry.Effects.Count() != effectsAt {
			if err != nil {
				return nil, false, "", err
			}
			for _, d := range res.Diagnostics {
				if !d.RuntimeMirror && d.Severity == SeverityError {
					return nil, false, "", a.registry.AqlError(d.Code, d.Detail, d.Word)
				}
			}
			return nil, false, "", fenceBlockedFallback(a.registry,
				a.registry.AqlError("internal_error",
					"compiled-mode refusal after the check pass emitted observable output ("+forceCompileReason(reason)+")", ""))
		}
		// C4 attribution: the refusal re-run is a SANCTIONED interpreter
		// entry — every entry it produces reports under this named seam
		// (plan Phase 10; the pre-Stage-J bounded fallback).
		restoreAtt := a.registry.SetInterpAttribution("fallback:refusal")
		out, rerr := a.runValues(src)
		restoreAtt()
		// Report the reason ONLY for a genuine performance refusal. A
		// statically-invalid program (err != nil, or the "check diagnostics"
		// sentinel) fails in both engines — the interpreter fallback raises the
		// real error — so it is not a fallback worth warning about.
		if err != nil || reason == "check diagnostics" {
			reason = ""
		}
		if rerr != nil {
			return nil, false, reason, rerr
		}
		return out, false, reason, nil
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
		// only burn the same budget again. The program DID compile, so this is
		// not a compilable-subset refusal — report no reason.
		if runtimeShouldFallback(err) {
			// C1 fence on the RUNTIME-BAIL arm: the compiled run may already
			// have printed/written before bailing (the L-DUP shape: every
			// section prints, then a dynamic-scope read misses); a silent
			// whole-source re-run would double every effect. Propagate the
			// internal_error, annotated, instead.
			if a.registry.Effects.Count() != effectsAt {
				return nil, true, "", fenceBlockedFallback(a.registry, err)
			}
			a.registry.RestoreForCompile(snap)
			a.registry.ResetStampLog()
			// C4 attribution: the runtime-bail re-run is the second
			// sanctioned interpreter entry (a designed VM defer resolved by
			// re-running) — named so the census distinguishes it.
			restoreAtt := a.registry.SetInterpAttribution("fallback:runtime-bail")
			out, rerr := a.runValues(src)
			restoreAtt()
			if rerr != nil {
				return nil, false, "", rerr
			}
			return out, false, "", nil
		}
		return nil, true, "", err
	}
	return result, true, "", nil
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
	// Same arming as RunCompiled: force mode is compiled execution, so
	// runtime-constructed callbacks stamp at their store sites too. Restored to
	// its prior state on return so the armed flag never leaks into a later plain
	// Run on a reused instance (see RunCompiled).
	wasArmed := a.registry.RuntimeStampingEnabled()
	a.registry.EnableRuntimeStamping()
	if !wasArmed {
		defer a.registry.DisableRuntimeStamping()
	}
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		a.registry.RestoreForCompile(snap)
		return nil, err
	}
	if prog == nil {
		a.registry.RestoreForCompile(snap)
		return nil, errors.New("force-compile: " + forceCompileReason(reason))
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

// forceCompileReason renders an uncompilable-program refusal reason for
// RunCompiledStrict's error, defaulting the (defensive, never observed
// through CompileCheck's current return paths) empty reason to a generic
// message rather than emitting a bare "force-compile: ".
func forceCompileReason(reason string) string {
	if reason == "" {
		return "program is not compilable"
	}
	return reason
}

// fenceBlockedFallback annotates a compiled-mode error whose silent
// interpreter re-run the effect fence blocked (eng effects.go,
// design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md C1): observable output
// already escaped, so re-running the source would duplicate it. The original
// error survives — an AqlError gains an explanatory note; a foreign Go error
// is wrapped in an internal_error carrying its text — so the caller sees both
// what failed and what to do about it.
func fenceBlockedFallback(r *native.Registry, err error) error {
	const note = "the interpreter fallback was blocked: output was already emitted, so re-running would duplicate it; run with --no-compile and report this as a compiler bug"
	var ae *eng.AqlError
	if errors.As(err, &ae) {
		// A designed defer that PREPARED for this arm (a no-match whose site
		// proved the interpreter would also fail the dispatch) carries the
		// rich user-facing raise — surface it instead of an internal error
		// telling the user to report a compiler bug (plan 3c).
		if ae.DeferAlt != nil {
			return ae.DeferAlt
		}
		cp := *ae
		cp.Notes = append(append([]string(nil), ae.Notes...), note)
		return &cp
	}
	return r.AqlErrorHint("internal_error", err.Error(), "", note)
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

// AqlError is the structured diagnostic every engine failure surfaces
// as (design/DIAGNOSTICS.0.md): code, detail, primary position,
// secondary labeled spans, notes, and suggestions. Hosts reach it with
// errors.As and re-render with color via Render(RenderOpts{Color:true});
// Error() is always the plain (ANSI-free) rendering.
type AqlError = native.AqlError

// RenderOpts controls diagnostic rendering (color on/off).
type RenderOpts = native.RenderOpts

// DiagSpan / DiagSuggestion are the structured payload of an AqlError
// or CheckDiagnostic: secondary labeled source locations and
// actionable fixes.
type (
	DiagSpan       = native.DiagSpan
	DiagSuggestion = native.DiagSuggestion
)

// ResolveColor decides whether to color output written to w for a
// --color mode of "always", "never", or "auto" (the default: color
// only a real terminal, honoring NO_COLOR).
var ResolveColor = native.ResolveColor

// RenderCheckDiagnostic renders the rich block (source excerpt, notes,
// suggestions) that sits under a check diagnostic's stable one-liner.
var RenderCheckDiagnostic = native.RenderCheckDiagnostic
