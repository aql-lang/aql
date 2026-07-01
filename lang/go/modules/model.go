package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/native"
	model "github.com/voxgig/model/go"
)

// The aql:model module — the Model namespace, a thin AQL surface over the
// upstream Go implementation of @voxgig/model (github.com/voxgig/model, the
// repo's go/ module). voxgig/model unifies .jsonic source into a single
// system model (via aontu — see native/aontu.go) and runs generator
// "actions" over it, once or in a rebuild-on-change watch loop.
//
// This module does not re-implement that pipeline: it assembles a model
// Build/Watch from the library's own exported building blocks (NewBuild,
// NewWatch, ModelProducer, LocalProducer) — exactly what model.New does
// internally — and drives Run / Start / Stop. The one thing it changes is
// the filesystem seam: every file read and write goes through AQL's aql:io
// FileOps capability (native.EffectiveFileOps), so a test that installs an
// in-memory FileOps (capabilities.NewMem, via AQL.SetFileOps) runs the whole
// model — source reads, the <base>/<name>.json write, inline source — entirely
// in memory, touching no disk. model.New itself cannot do this: its ModelSpec
// exposes no FS seam (only BuildSpec does), which is why the module assembles
// the Build directly.
//
// Word surface (mirrors voxgig New / Run / Start / Stop):
//
//	Model.new   <spec:Map>   -> Model        construct a model from a spec
//	Model.run   <Model>      -> Map          build once; resolved model + result
//	Model.start <Model>      -> Map          build once, then watch & rebuild
//	Model.stop  <Model>                      end watching
//	Model.model <Model>      -> Map          the unified model (after a build)
//
// Spec Map keys (all optional unless noted):
//
//	src     String   inline .jsonic source (one of src / path is required)
//	path    String   .jsonic file path (read through FileOps)
//	base    String   base directory for @"..." imports and JSON output
//	args    Map       build args passed through to the model
//	dryrun  Boolean   keep writes in an overlay (no FileOps writes)
//	order   List      action names, in run order
//	watch   Map       {mod,add,rem} filesystem events that trigger a rebuild
//	actions Map        name -> Function, or name -> {run:Function, step:'pre'|'post'|'all'}

// TModel is Ideal/Model — the inert carrier wrapping a *modelHandle (the
// assembled model Build/Watch plus the registry used to run its AQL-function
// actions) in an ExtensionPayload the kernel never inspects. FixedID 5006 —
// next free in the 5000–9999 kernel/language band (Module 5000, ModuleExport
// 5001, KeyVal 5002, MiniLangCompiled 5003, Patrun 5004, ParseGrammar 5005).
// See eng/go/CLAUDE.md "FixedID Allocation" and test/fixedid_stability_test.go.
var TModel = registerModelType()

var modelTypeInitErr error

func registerModelType() *native.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin("Ideal/Model", 5006, nil)
	if err != nil {
		modelTypeInitErr = fmt.Errorf("model: register Ideal/Model: %w", err)
	}
	return t
}

// inlineSeq names inline-source scratch directories uniquely (within the
// active FileOps — disk for the OS backend, memory for a test backend).
var inlineSeq atomic.Int64

// modelHandle is the Go state behind a Model value: the assembled Build and
// Watch, the registry actions call back into (actionReg), action errors
// collected during a build (CallAQL errors cannot flow through the
// model.Action signature, so they are stashed here and merged into the
// result), and a mutex serialising action callbacks.
//
// actionReg is the registry an action's CallAQL runs on. For Model.run it is
// the live foreground registry (the build runs synchronously on the calling
// goroutine, so that is race-free). For Model.start it is a ForkConcurrent of
// the registry, taken on the foreground goroutine before the watch begins, so
// the watch goroutine's rebuilds run their AQL actions on an isolated registry
// that can never race the main interpreter — the same pattern timeout /
// interval / await use (see eng/go/fork.go).
type modelHandle struct {
	build     *model.Build
	watch     *model.Watch
	reg       *native.Registry
	actionReg *native.Registry
	mu        sync.Mutex
	actErrs   []error
}

// BuildModelModule creates the "aql:model" native module.
func BuildModelModule(parent *native.Registry) (native.ModuleDesc, error) {
	if modelTypeInitErr != nil {
		return native.ModuleDesc{}, modelTypeInitErr
	}
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()

	// Model.new <spec:Map> -> Model
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "model-new",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TMap},
			Returns:    []*native.Type{TModel},
			BarrierPos: -1,
			Impl:       native.Go(modelNewHandler),
		}},
	})
	exports.Set("new", wrapMiniFnDef("model-new", [][]native.FnParam{{{Type: native.TMap}}},
		[]*native.Type{TModel}, nil, subReg))

	registerModelWord(subReg, exports, "run", modelRunHandler)
	registerModelWord(subReg, exports, "start", modelStartHandler)
	registerModelWord(subReg, exports, "model", modelModelHandler)

	// Model.stop <Model> -> (nothing)
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "model-stop",
		Signatures: []native.Signature{{
			Args:       []*native.Type{TModel},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Impl:       native.Go(modelStopHandler),
		}},
	})
	exports.Set("stop", wrapMiniFnDef("model-stop", [][]native.FnParam{{{Type: TModel}}},
		[]*native.Type{}, nil, subReg))

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"Model": exports},
	}, nil
}

// registerModelWord registers a [Model] -> [Map] word under both the inner
// sub-registry and the export map.
func registerModelWord(subReg *native.Registry, exports *native.OrderedMap, name string, h native.Handler) {
	inner := "model-" + name
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: inner,
		Signatures: []native.Signature{{
			Args:       []*native.Type{TModel},
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
			Impl:       native.Go(h),
		}},
	})
	exports.Set(name, wrapMiniFnDef(inner, [][]native.FnParam{{{Type: TModel}}},
		[]*native.Type{native.TMap}, nil, subReg))
}

// asModelHandle unwraps a Model value's *modelHandle.
func asModelHandle(v native.Value) (*modelHandle, bool) {
	ep, ok := v.Data.(eng.ExtensionPayload)
	if !ok {
		return nil, false
	}
	h, ok := ep.Body.(*modelHandle)
	return h, ok
}

// parsedSpec is the spec Map decoded into the fields the Build assembly needs.
type parsedSpec struct {
	path      string
	base      string
	inlineSrc string
	hasInline bool
	args      map[string]any
	dryrun    bool
	order     []string
	watch     model.WatchModes
	actions   map[string]model.ActionDef
}

// modelNewHandler decodes the spec Map, wires a FileOps-backed model.FS, and
// assembles the model Build/Watch.
func modelNewHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if !native.IsConcrete(args[0]) {
		return nil, r.AqlError("model_bad_spec", "new: spec must be a concrete Map", "new")
	}
	specMap, merr := native.AsMap(args[0])
	if merr != nil {
		return nil, r.AqlError("model_bad_spec", "new: spec must be a Map", "new")
	}

	h := &modelHandle{reg: r, actionReg: r}
	ps, err := parseSpec(specMap, h, r)
	if err != nil {
		return nil, err
	}

	// Every file operation goes through the active aql:io FileOps so a test
	// can run the model on an in-memory FS (capabilities.NewMem).
	fs := &fileOpsFS{ops: native.EffectiveFileOps(r), dry: ps.dryrun, mem: map[string][]byte{}}

	path, base := ps.path, ps.base
	if ps.hasInline {
		base = filepath.Join(os.TempDir(), fmt.Sprintf("aql-model-%d-%d", os.Getpid(), inlineSeq.Add(1)))
		path = filepath.Join(base, "model.jsonic")
		if mkErr := fs.MkdirAll(base, 0o755); mkErr != nil {
			return nil, r.AqlError("model_io", "new: inline base dir: "+mkErr.Error(), "new")
		}
		if wErr := fs.WriteFile(path, []byte(ps.inlineSrc), 0o600); wErr != nil {
			return nil, r.AqlError("model_io", "new: write inline source: "+wErr.Error(), "new")
		}
	} else if base == "" {
		base = filepath.Dir(path)
	}

	build := model.NewBuild(model.BuildSpec{
		Name:    "model",
		Path:    path,
		Base:    base,
		Args:    ps.args,
		Dryrun:  ps.dryrun,
		Actions: ps.actions,
		Order:   ps.order,
		Idle:    model.DefaultIdle,
		Watch:   ps.watch,
		FS:      fs,
		Res: []model.ProducerDef{
			{Path: "/", Build: model.ModelProducer},
			{Path: "/", Build: model.LocalProducer},
		},
	})
	h.build = build
	h.watch = model.NewWatch(build, "model", model.DefaultIdle)

	return []native.Value{eng.NewExtension(TModel, h)}, nil
}

// parseSpec decodes the spec Map, wiring AQL-function actions through
// makeAction.
func parseSpec(specMap native.ReadMap, h *modelHandle, r *native.Registry) (parsedSpec, error) {
	var ps parsedSpec

	path, hasPath := mapStr(specMap, "path")
	src, hasSrc := mapStr(specMap, "src")
	switch {
	case hasSrc:
		ps.inlineSrc = src
		ps.hasInline = true
	case hasPath:
		ps.path = path
		if base, ok := mapStr(specMap, "base"); ok {
			ps.base = base
		}
	default:
		return ps, r.AqlErrorHint("model_bad_spec",
			"new: spec needs a 'src' (inline) or 'path' (file) String", "new",
			"e.g. Model.new {src:'service: name: \"orders\"'}")
	}

	if v, ok := specMap.Get("args"); ok && native.IsConcrete(v) {
		if m, mok := native.ValueToAny(v).(map[string]any); mok {
			ps.args = m
		}
	}
	if b, ok := mapBool(specMap, "dryrun"); ok {
		ps.dryrun = b
	}
	if order, ok := mapStrList(specMap, "order"); ok {
		ps.order = order
	}
	if w, ok := specMap.Get("watch"); ok && native.IsConcrete(w) {
		if wm, werr := native.AsMap(w); werr == nil {
			mod, _ := mapBool(wm, "mod")
			add, _ := mapBool(wm, "add")
			rem, _ := mapBool(wm, "rem")
			ps.watch = model.WatchModes{Mod: mod, Add: add, Rem: rem}
		}
	}

	actions, err := buildActions(specMap, h, r)
	if err != nil {
		return ps, err
	}
	ps.actions = actions
	return ps, nil
}

// buildActions reads the `actions` spec field into a map of model.ActionDef,
// each wrapping an AQL Function.
func buildActions(specMap native.ReadMap, h *modelHandle, r *native.Registry) (map[string]model.ActionDef, error) {
	av, ok := specMap.Get("actions")
	if !ok {
		return nil, nil
	}
	if !native.IsConcrete(av) {
		return nil, r.AqlError("model_bad_action", "new: actions must be a Map", "new")
	}
	am, aerr := native.AsMap(av)
	if aerr != nil {
		return nil, r.AqlError("model_bad_action", "new: actions must be a Map", "new")
	}
	out := make(map[string]model.ActionDef, len(am.Keys()))
	for _, name := range am.Keys() {
		entry, _ := am.Get(name)
		fnVal, step, err := actionFnAndStep(name, entry, r)
		if err != nil {
			return nil, err
		}
		out[name] = makeAction(h, fnVal, step)
	}
	return out, nil
}

// actionFnAndStep resolves one action entry to its Function value and step.
// An entry is either a Function directly (step defaults to post) or a Map
// {run:Function, step:'pre'|'post'|'all'}.
func actionFnAndStep(name string, entry native.Value, r *native.Registry) (native.Value, model.Step, error) {
	if _, ok := entry.Data.(native.FnDefInfo); ok {
		return entry, model.StepPost, nil
	}
	if native.IsConcrete(entry) {
		if m, err := native.AsMap(entry); err == nil {
			run, hasRun := m.Get("run")
			if !hasRun {
				return native.Value{}, "", r.AqlErrorHint("model_bad_action",
					fmt.Sprintf("new: action %q map needs a 'run' Function", name), "new",
					"e.g. actions:{gen:{run:([m:Any] => [...]), step:'post'}}")
			}
			if _, ok := run.Data.(native.FnDefInfo); !ok {
				return native.Value{}, "", r.AqlError("model_bad_action",
					fmt.Sprintf("new: action %q 'run' must be a Function", name), "new")
			}
			step := model.StepPost
			if s, ok := mapStr(m, "step"); ok {
				step = parseStep(s)
			}
			return run, step, nil
		}
	}
	return native.Value{}, "", r.AqlErrorHint("model_bad_action",
		fmt.Sprintf("new: action %q must be a Function or {run:Function}", name), "new",
		"e.g. actions:{summary:([m:Any] => [m size])}")
}

func parseStep(s string) model.Step {
	switch s {
	case "pre":
		return model.StepPre
	case "all":
		return model.StepAll
	default:
		return model.StepPost
	}
}

// makeAction wraps an AQL Function as a model.Action. The callback hands the
// unified model (as an AQL Map) to the function via CallAQL on h.actionReg —
// the foreground registry for Model.run, an isolated fork for Model.start — and
// reads its result: a Boolean is the OK flag; a Map supplies {ok, reload};
// anything else (including no result) is treated as OK. A CallAQL error is
// stashed on the handle (the model.Action signature has no error return) and
// surfaces in the build result.
func makeAction(h *modelHandle, fnVal native.Value, step model.Step) model.ActionDef {
	var caps []native.CapturedBinding
	if fd, ok := fnVal.Data.(native.FnDefInfo); ok {
		caps = fd.Captured
	}
	return model.ActionDef{
		Step: step,
		Run: func(mdl map[string]any, _ *model.Build, _ *model.BuildContext) model.ActionResult {
			h.mu.Lock()
			defer h.mu.Unlock()

			modelVal := native.AnyToValue(mdl)
			sig := native.MatchFnSig(fnVal, []native.Value{modelVal})
			if sig == nil {
				h.actErrs = append(h.actErrs, fmt.Errorf("action: no matching signature (declare one param for the model Map)"))
				return model.ActionResult{OK: false}
			}
			res, err := h.actionReg.CallAQL(sig, []native.Value{modelVal}, caps)
			if err != nil {
				h.actErrs = append(h.actErrs, fmt.Errorf("action: %w", err))
				return model.ActionResult{OK: false}
			}
			return interpretActionResult(res)
		},
	}
}

// interpretActionResult maps an AQL action's result stack to an ActionResult.
func interpretActionResult(res []native.Value) model.ActionResult {
	if len(res) == 0 {
		return model.ActionResult{OK: true}
	}
	last := res[len(res)-1]
	if b, err := last.AsConcreteBoolean(); err == nil {
		return model.ActionResult{OK: b}
	}
	if native.IsConcrete(last) {
		if m, err := native.AsMap(last); err == nil {
			ok := true
			if v, has := m.Get("ok"); has {
				if bb, berr := v.AsConcreteBoolean(); berr == nil {
					ok = bb
				}
			}
			reload := false
			if v, has := m.Get("reload"); has {
				if bb, berr := v.AsConcreteBoolean(); berr == nil {
					reload = bb
				}
			}
			return model.ActionResult{OK: ok, Reload: reload}
		}
	}
	return model.ActionResult{OK: true}
}

func modelRunHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	h, ok := asModelHandle(args[0])
	if !ok {
		return nil, r.AqlError("model_bad_handle", "run: not a Model", "run")
	}
	// Model.run is synchronous on this (foreground) goroutine, so actions can
	// call back into the live registry directly — re-entrant, single-threaded,
	// race-free (the same way each / fold / filter invoke their callbacks).
	h.mu.Lock()
	h.actErrs = nil
	h.actionReg = r
	h.mu.Unlock()
	br := h.watch.Run(false)
	return []native.Value{buildResultValue(br, h)}, nil
}

func modelStartHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	h, ok := asModelHandle(args[0])
	if !ok {
		return nil, r.AqlError("model_bad_handle", "start: not a Model", "start")
	}
	// Watch rebuilds run on a background goroutine, so their AQL actions must
	// NOT touch the foreground registry. Fork an isolated registry HERE, on the
	// foreground goroutine (ForkConcurrent's contract: fork while the parent is
	// not concurrently mutating), and route action callbacks to it. Outputs are
	// SyncWriter-wrapped so concurrent prints don't interleave with the main
	// program's. Mirrors timeout / interval (eng/go/fork.go).
	fork := r.ForkConcurrent()
	fork.Output = native.NewSyncWriter(r.Output)
	fork.ErrOutput = native.NewSyncWriter(r.ErrOutput)
	h.mu.Lock()
	h.actErrs = nil
	h.actionReg = fork
	h.mu.Unlock()
	br := h.watch.Start()
	return []native.Value{buildResultValue(br, h)}, nil
}

func modelStopHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	h, ok := asModelHandle(args[0])
	if !ok {
		return nil, r.AqlError("model_bad_handle", "stop: not a Model", "stop")
	}
	h.watch.Stop()
	return nil, nil
}

func modelModelHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	h, ok := asModelHandle(args[0])
	if !ok {
		return nil, r.AqlError("model_bad_handle", "model: not a Model", "model")
	}
	if h.build == nil || h.build.Model == nil {
		return nil, r.AqlErrorHint("model_not_built",
			"model: the model has not been built yet", "model",
			"call Model.run (or Model.start) first")
	}
	return []native.Value{native.AnyToValue(h.build.Model)}, nil
}

// buildResultValue marshals a *model.BuildResult to an AQL Map, merging any
// action errors stashed on the handle during the build.
func buildResultValue(br *model.BuildResult, h *modelHandle) native.Value {
	om := native.NewOrderedMap()
	ok := br != nil && br.OK && len(h.actErrs) == 0
	om.Set("ok", native.NewBoolean(ok))

	errs := []native.Value{}
	if br != nil {
		for _, e := range br.Errs {
			errs = append(errs, native.NewString(e.Error()))
		}
	}
	for _, e := range h.actErrs {
		errs = append(errs, native.NewString(e.Error()))
	}
	om.Set("errs", native.NewList(errs))

	runlog := []native.Value{}
	if br != nil {
		for _, s := range br.Runlog {
			runlog = append(runlog, native.NewString(s))
		}
	}
	om.Set("runlog", native.NewList(runlog))

	if b := br.Build(); b != nil && b.Model != nil {
		om.Set("model", native.AnyToValue(b.Model))
	}
	return native.NewMap(om)
}

// ---- FileOps-backed model.FS ---------------------------------------------

// fileOpsFS adapts AQL's aql:io FileOps to the model.FS seam, so the whole
// model pipeline (source reads, the JSON-model write, inline source) runs on
// whatever FileOps the registry has — disk for the OS backend, memory for a
// test backend. In dryrun mode writes are kept in an in-memory overlay (and
// read back from it) instead of reaching the underlying FileOps, mirroring
// voxgig/model's own dryFS.
type fileOpsFS struct {
	ops capabilities.FileOps
	dry bool
	mu  sync.Mutex
	mem map[string][]byte
}

func (f *fileOpsFS) key(name string) string {
	if resolved, err := f.ops.ResolvePath(name); err == nil {
		return resolved
	}
	return name
}

func (f *fileOpsFS) ReadFile(name string) ([]byte, error) {
	if f.dry {
		f.mu.Lock()
		data, ok := f.mem[f.key(name)]
		f.mu.Unlock()
		if ok {
			out := make([]byte, len(data))
			copy(out, data)
			return out, nil
		}
	}
	return f.ops.ReadFile(name)
}

func (f *fileOpsFS) WriteFile(name string, data []byte, _ os.FileMode) error {
	if f.dry {
		f.mu.Lock()
		cp := make([]byte, len(data))
		copy(cp, data)
		f.mem[f.key(name)] = cp
		f.mu.Unlock()
		return nil
	}
	return f.ops.WriteFile(name, data, 0o644)
}

func (f *fileOpsFS) MkdirAll(path string, _ os.FileMode) error {
	if f.dry {
		return nil
	}
	return f.ops.MkdirAll(path, 0o755)
}

// Stat is best-effort: FileOps has no Stat, so for the OS backend it resolves
// to a real path and os.Stat (giving the real mtime the watch loop needs),
// and otherwise (in-memory FileOps, dryrun overlay) returns a synthetic
// stable FileInfo when the file is readable. A non-existent file errors.
func (f *fileOpsFS) Stat(name string) (os.FileInfo, error) {
	if resolved, err := f.ops.ResolvePath(name); err == nil {
		if fi, serr := os.Stat(resolved); serr == nil {
			return fi, nil
		}
	}
	if _, err := f.ReadFile(name); err != nil {
		return nil, err
	}
	return synthFileInfo{name: filepath.Base(name)}, nil
}

// synthFileInfo is a minimal os.FileInfo for files that exist in a non-OS
// FileOps (no real mtime). Its stable zero ModTime means the watch loop will
// not detect content changes through an in-memory FS — acceptable, since the
// in-memory backend is used by tests that build once.
type synthFileInfo struct{ name string }

func (s synthFileInfo) Name() string     { return s.name }
func (synthFileInfo) Size() int64        { return 0 }
func (synthFileInfo) Mode() os.FileMode  { return 0o644 }
func (synthFileInfo) ModTime() time.Time { return time.Time{} }
func (synthFileInfo) IsDir() bool        { return false }
func (synthFileInfo) Sys() any           { return nil }

// ---- small Map readers ----------------------------------------------------

func mapStr(m native.ReadMap, key string) (string, bool) {
	v, ok := m.Get(key)
	if !ok || !native.IsConcrete(v) {
		return "", false
	}
	s, err := v.AsConcreteString()
	if err != nil {
		return "", false
	}
	return s, true
}

func mapBool(m native.ReadMap, key string) (bool, bool) {
	v, ok := m.Get(key)
	if !ok {
		return false, false
	}
	b, err := v.AsConcreteBoolean()
	if err != nil {
		return false, false
	}
	return b, true
}

func mapStrList(m native.ReadMap, key string) ([]string, bool) {
	v, ok := m.Get(key)
	if !ok || !native.IsConcrete(v) {
		return nil, false
	}
	lst, err := native.AsList(v)
	if err != nil {
		return nil, false
	}
	out := make([]string, 0, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		e := lst.Get(i)
		if s, err := e.AsConcreteString(); err == nil {
			out = append(out, s)
			continue
		}
		if a, err := e.AsConcreteAtom(); err == nil {
			out = append(out, a)
		}
	}
	return out, true
}
