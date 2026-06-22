package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
	model "github.com/voxgig/model/go"
)

// The aql:model module — the Model namespace, a thin AQL surface over the
// upstream Go implementation of @voxgig/model (github.com/voxgig/model, the
// repo's go/ module). voxgig/model unifies .jsonic source into a single
// system model (via aontu — see native/aontu.go) and runs generator
// "actions" over it, once or in a rebuild-on-change watch loop.
//
// This module does not re-implement any of that: it constructs a real
// *model.Model from a spec Map, drives its Run / Start / Stop pipeline, and
// marshals the BuildResult back to AQL. Actions are AQL Functions: each is
// wrapped in a Go callback that hands the unified model (as a Map) to the
// function via the interpreter (r.CallAQL) and reads its result.
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
//	src     String   inline .jsonic source (materialised to a temp dir)
//	path    String   .jsonic file path (one of src / path is required)
//	base    String   base directory for @"..." imports and JSON output
//	args    Map       build args passed through to the model
//	dryrun  Boolean   keep writes in memory (no disk output)
//	order   List      action names, in run order
//	config  Boolean   resolve a .model-config build (default false)
//	watch   Map       {mod,add,rem} filesystem events that trigger a rebuild
//	actions Map        name -> Function, or name -> {run:Function, step:'pre'|'post'|'all'}
//
// Filesystem note: voxgig/model is a build tool — model.New always uses the
// real OS filesystem (or an in-memory write buffer when dryrun is set);
// ModelSpec exposes no filesystem seam. So `path` reads real files and the
// model writer writes <base>/<name>.json to disk. Inline `src` is written to
// an os temp directory. This is intentional and distinct from AQL's
// sandboxed aql:io words.

// TModel is Ideal/Model — the inert carrier wrapping a *modelHandle (the
// live *model.Model plus the registry used to run its AQL-function actions)
// in an ExtensionPayload the kernel never inspects. FixedID 5006 — next free
// in the 5000–9999 kernel/language band (Module 5000, ModuleExport 5001,
// KeyVal 5002, MiniLangCompiled 5003, Patrun 5004, ParseGrammar 5005). See
// eng/go/CLAUDE.md "FixedID Allocation" and test/fixedid_stability_test.go.
var TModel = registerModelType()

var modelTypeInitErr error

func registerModelType() *native.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin("Ideal/Model", 5006, nil)
	if err != nil {
		modelTypeInitErr = fmt.Errorf("model: register Ideal/Model: %w", err)
	}
	return t
}

// modelHandle is the Go state behind a Model value: the constructed
// *model.Model, the registry actions call back into, the temp dir holding
// inline source (if any), action errors collected during a build (CallAQL
// errors cannot flow through the model.Action signature, so they are stashed
// here and merged into the result), and a mutex serialising action callbacks
// (watch rebuilds run on a background goroutine).
type modelHandle struct {
	m       *model.Model
	reg     *native.Registry
	tmpDir  string
	mu      sync.Mutex
	actErrs []error
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
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TMap},
			Returns:    []*native.Type{TModel},
			BarrierPos: -1,
			Handler:    modelNewHandler,
		}},
	})
	exports.Set("new", wrapMiniFnDef("model-new", [][]native.FnParam{{{Type: native.TMap}}},
		[]*native.Type{TModel}, nil, subReg))

	// Model.run <Model> -> Map
	registerModelWord(subReg, exports, "run", modelRunHandler)
	// Model.start <Model> -> Map
	registerModelWord(subReg, exports, "start", modelStartHandler)
	// Model.model <Model> -> Map
	registerModelWord(subReg, exports, "model", modelModelHandler)

	// Model.stop <Model> -> (nothing)
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "model-stop",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{TModel},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Handler:    modelStopHandler,
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
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{TModel},
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
			Handler:    h,
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

// modelNewHandler builds a *model.Model from the spec Map and returns it as a
// Model value.
func modelNewHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if !native.IsConcrete(args[0]) {
		return nil, r.AqlError("model_bad_spec", "new: spec must be a concrete Map", "new")
	}
	specMap, merr := native.AsMap(args[0])
	if merr != nil {
		return nil, r.AqlError("model_bad_spec", "new: spec must be a Map", "new")
	}

	h := &modelHandle{reg: r}
	spec, err := buildModelSpec(specMap, h, r)
	if err != nil {
		return nil, err
	}
	h.m = model.New(spec)
	return []native.Value{eng.NewExtension(TModel, h)}, nil
}

// buildModelSpec translates the AQL spec Map into a model.ModelSpec, wiring
// AQL-function actions through makeAction and materialising inline source to
// a temp directory.
func buildModelSpec(specMap native.ReadMap, h *modelHandle, r *native.Registry) (model.ModelSpec, error) {
	var spec model.ModelSpec

	path, hasPath := mapStr(specMap, "path")
	src, hasSrc := mapStr(specMap, "src")
	switch {
	case hasSrc:
		dir, err := os.MkdirTemp("", "aql-model-")
		if err != nil {
			return spec, r.AqlError("model_io", "new: temp dir: "+err.Error(), "new")
		}
		h.tmpDir = dir
		file := filepath.Join(dir, "model.jsonic")
		if werr := os.WriteFile(file, []byte(src), 0o600); werr != nil {
			return spec, r.AqlError("model_io", "new: write inline source: "+werr.Error(), "new")
		}
		spec.Path = file
		spec.Base = dir
	case hasPath:
		spec.Path = path
		if base, ok := mapStr(specMap, "base"); ok {
			spec.Base = base
		}
	default:
		return spec, r.AqlErrorHint("model_bad_spec",
			"new: spec needs a 'src' (inline) or 'path' (file) String", "new",
			"e.g. Model.new {src:'service: name: \"orders\"'}")
	}

	if v, ok := specMap.Get("args"); ok && native.IsConcrete(v) {
		if m, mok := native.ValueToAny(v).(map[string]any); mok {
			spec.Args = m
		}
	}
	if b, ok := mapBool(specMap, "dryrun"); ok {
		spec.Dryrun = b
	}
	if order, ok := mapStrList(specMap, "order"); ok {
		spec.Order = order
	}
	if w, ok := specMap.Get("watch"); ok && native.IsConcrete(w) {
		if wm, werr := native.AsMap(w); werr == nil {
			mod, _ := mapBool(wm, "mod")
			add, _ := mapBool(wm, "add")
			rem, _ := mapBool(wm, "rem")
			spec.Watch = model.WatchModes{Mod: mod, Add: add, Rem: rem}
		}
	}
	// Config defaults OFF: the .model-config build reads/creates files in the
	// base dir, a surprise for a one-shot model. Opt in with config:true.
	cfg := false
	if b, ok := mapBool(specMap, "config"); ok {
		cfg = b
	}
	spec.Config = &cfg

	actions, err := buildActions(specMap, h, r)
	if err != nil {
		return spec, err
	}
	spec.Actions = actions
	return spec, nil
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
		out[name] = makeAction(h, r, fnVal, step)
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
					"e.g. actions:{gen:{run:([m] => [...]), step:'post'}}")
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
		"e.g. actions:{summary:([m] => [m size])}")
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
// unified model (as an AQL Map) to the function via r.CallAQL and reads its
// result: a Boolean is the OK flag; a Map supplies {ok, reload}; anything
// else (including no result) is treated as OK. A CallAQL error is stashed on
// the handle (the model.Action signature has no error return) and surfaces in
// the build result.
func makeAction(h *modelHandle, r *native.Registry, fnVal native.Value, step model.Step) model.ActionDef {
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
			res, err := r.CallAQL(sig, []native.Value{modelVal}, caps)
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
	h.actErrs = nil
	br := h.m.Run()
	return []native.Value{buildResultValue(br, h)}, nil
}

func modelStartHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	h, ok := asModelHandle(args[0])
	if !ok {
		return nil, r.AqlError("model_bad_handle", "start: not a Model", "start")
	}
	h.actErrs = nil
	br := h.m.Start()
	return []native.Value{buildResultValue(br, h)}, nil
}

func modelStopHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	h, ok := asModelHandle(args[0])
	if !ok {
		return nil, r.AqlError("model_bad_handle", "stop: not a Model", "stop")
	}
	h.m.Stop()
	return nil, nil
}

func modelModelHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	h, ok := asModelHandle(args[0])
	if !ok {
		return nil, r.AqlError("model_bad_handle", "model: not a Model", "model")
	}
	b := h.m.Build()
	if b == nil || b.Model == nil {
		return nil, r.AqlErrorHint("model_not_built",
			"model: the model has not been built yet", "model",
			"call Model.run (or Model.start) first")
	}
	return []native.Value{native.AnyToValue(b.Model)}, nil
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
