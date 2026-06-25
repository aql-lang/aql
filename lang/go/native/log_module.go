package native

import (
	"encoding/json"
	golog "log"
	"sync"
	"time"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/policy"
)

// log_module.go implements the Go-side machinery for the `aql:log`
// native module (namespace `Log`). The module builder and FnDef
// wrappers live in modules/log.go; this file owns the value types
// (LogLevel, LogRecord), the fan-out sink registry, the built-in
// sinks (console / memory / null), and the word handlers.
//
// Phase 1 (this file) covers the traditional-logging layer: six
// severity levels, structured fields, a global level threshold, a
// console sink built on Go's standard `log` package writing to
// Registry.ErrOutput, plus `memory` (capture-for-tests) and `null`
// sinks. Provider hooks (RegisterHostLogSink / Log.register) and the
// trace/span surface arrive in later phases — see
// design/LOG-MODULE.10.md.
//
// Per-import state (the level, format, attached sinks, capture buffer)
// lives in ONE *LogSinkRegistry captured by the word closures at
// module-build time — the same pattern aql:rand uses for its PRNG
// (modules/rand.go). The registry passed to a handler at dispatch is
// used only for the live ErrOutput / Clock / Policy seams, exactly as
// `print` reaches them.

// LogLevel is a severity, valued at the OpenTelemetry severity number
// for its band so the mapping to the OTel logs data model is lossless.
type LogLevel int

// The six severity bands, on OTel severity numbers.
const (
	LevelTrace LogLevel = 1
	LevelDebug LogLevel = 5
	LevelInfo  LogLevel = 9
	LevelWarn  LogLevel = 13
	LevelError LogLevel = 17
	LevelFatal LogLevel = 21
)

// Name returns the lowercase level atom name (the surface spelling:
// `trace`, `debug`, …) used by Log.get-level and accepted by
// Log.set-level / Log.enabled.
func (l LogLevel) Name() string {
	switch l {
	case LevelTrace:
		return "trace"
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return "info"
	}
}

// Text returns the uppercase OTel severity text ("INFO", …).
func (l LogLevel) Text() string {
	switch l {
	case LevelTrace:
		return "TRACE"
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "INFO"
	}
}

// parseLevel maps a level atom name to its LogLevel. The second result
// is false for an unrecognised name (the caller raises log_error).
func parseLevel(name string) (LogLevel, bool) {
	switch name {
	case "trace":
		return LevelTrace, true
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "warn":
		return LevelWarn, true
	case "error":
		return LevelError, true
	case "fatal":
		return LevelFatal, true
	default:
		return 0, false
	}
}

// LogRecord is the neutral OTel-logs shape: the in-memory record every
// sink consumes. A host OTel bridge (phase 3) translates it to an OTel
// LogRecord; the built-in console/memory sinks consume it directly.
// TraceID/SpanID are populated once the span surface lands (phase 4);
// in phase 1 they are always empty so the shape stays stable.
type LogRecord struct {
	Timestamp  time.Time
	Severity   LogLevel
	Body       Value // the message (a String) or any logged Value
	Attributes Value // a concrete Map of structured fields (never a type literal)
	Logger     string
	TraceID    string
	SpanID     string
}

// toMap renders a LogRecord as a Record-shaped Map, the form Log.dump
// returns so AQL code (and spec rows) can inspect captured records.
// Field order matches design/LOG-MODULE.10.md §3.2.
func (rec LogRecord) toMap() Value {
	m := NewOrderedMap()
	m.Set("timestamp", NewDateTime(rec.Timestamp))
	m.Set("severity", NewInteger(int64(rec.Severity)))
	m.Set("severity-text", NewString(rec.Severity.Text()))
	m.Set("body", rec.Body)
	m.Set("attributes", rec.attributesOrEmpty())
	m.Set("logger", NewString(rec.Logger))
	m.Set("trace-id", NewString(rec.TraceID))
	m.Set("span-id", NewString(rec.SpanID))
	return NewMap(m)
}

// attributesOrEmpty returns the record's attribute map, or a fresh
// empty Map when no structured fields were supplied (so the record
// shape is uniform and never carries a bare type literal).
func (rec LogRecord) attributesOrEmpty() Value {
	if IsConcrete(rec.Attributes) && rec.Attributes.Parent != nil && rec.Attributes.Parent.Equal(TMap) {
		return rec.Attributes
	}
	return NewMap(NewOrderedMap())
}

// logSink is one named destination. emit consumes a record; built-in
// sinks never error, but the signature carries an error for the
// host-registered sinks of phase 3.
type logSink struct {
	name string
	min  LogLevel
	emit func(r *Registry, rec LogRecord) error
}

// LogSinkRegistry holds the per-import logging state: the global level
// threshold, the console render format, the set of available
// (attachable) sinks, the attachment order, and the memory sink's
// capture buffer. All access is mutex-guarded — a logger may be shared
// across goroutines.
type LogSinkRegistry struct {
	mu        sync.Mutex
	level     LogLevel
	format    string // "text" | "json"
	available map[string]*logSink
	attached  []string
	captured  []LogRecord
}

// NewLogSinkRegistry returns a default sink registry (level INFO, text
// format, console attached, memory/null available). Exposed so a host
// can construct one, pre-attach an OTel/Datadog sink, and install it
// with SetHostLogSinks before the AQL program runs `import "aql:log"`.
func NewLogSinkRegistry() *LogSinkRegistry { return newLogSinkRegistry() }

// newLogSinkRegistry builds the default registry: level INFO, text
// format, the three built-in sinks registered, and `console` attached
// — so `Log.info` works the moment the module is imported, with no
// host wiring, exactly like `print`.
func newLogSinkRegistry() *LogSinkRegistry {
	lsr := &LogSinkRegistry{
		level:     LevelInfo,
		format:    "text",
		available: map[string]*logSink{},
		attached:  []string{"console"},
	}
	// console — traditional logging via Go's standard `log` package,
	// writing one line per record to the registry's ErrOutput.
	lsr.available["console"] = &logSink{
		name: "console",
		min:  LevelTrace,
		emit: func(r *Registry, rec LogRecord) error {
			if r == nil || r.ErrOutput == nil {
				return nil
			}
			golog.New(r.ErrOutput, "", 0).Println(lsr.render(rec))
			return nil
		},
	}
	// memory — capture records for inspection / tests (Log.dump).
	lsr.available["memory"] = &logSink{
		name: "memory",
		min:  LevelTrace,
		emit: func(_ *Registry, rec LogRecord) error {
			lsr.mu.Lock()
			lsr.captured = append(lsr.captured, rec)
			lsr.mu.Unlock()
			return nil
		},
	}
	// null — discard (benchmarking / silencing).
	lsr.available["null"] = &logSink{
		name: "null",
		min:  LevelTrace,
		emit: func(_ *Registry, _ LogRecord) error { return nil },
	}
	return lsr
}

// Level / SetLevel / Format / SetFormat are the config accessors used
// by the Log.get-level / set-level / get-format / set-format words.
func (lsr *LogSinkRegistry) Level() LogLevel {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	return lsr.level
}

func (lsr *LogSinkRegistry) SetLevel(l LogLevel) {
	lsr.mu.Lock()
	lsr.level = l
	lsr.mu.Unlock()
}

func (lsr *LogSinkRegistry) Format() string {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	return lsr.format
}

func (lsr *LogSinkRegistry) SetFormat(f string) {
	lsr.mu.Lock()
	lsr.format = f
	lsr.mu.Unlock()
}

// Available reports whether a sink name is registered (attachable).
func (lsr *LogSinkRegistry) Available(name string) bool {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	_, ok := lsr.available[name]
	return ok
}

// Attach adds a registered sink to the active pipeline, idempotently
// (re-attaching an already-attached sink is a no-op, preserving its
// position).
func (lsr *LogSinkRegistry) Attach(name string) {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	for _, n := range lsr.attached {
		if n == name {
			return
		}
	}
	lsr.attached = append(lsr.attached, name)
}

// Detach removes a sink from the active pipeline; absent is a no-op.
func (lsr *LogSinkRegistry) Detach(name string) {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	out := lsr.attached[:0:0]
	for _, n := range lsr.attached {
		if n != name {
			out = append(out, n)
		}
	}
	lsr.attached = out
}

// Attached returns the attachment order (a copy).
func (lsr *LogSinkRegistry) Attached() []string {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	return append([]string(nil), lsr.attached...)
}

// Captured returns the memory sink's buffer (a copy).
func (lsr *LogSinkRegistry) Captured() []LogRecord {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	return append([]LogRecord(nil), lsr.captured...)
}

// ClearCaptured empties the memory sink's buffer.
func (lsr *LogSinkRegistry) ClearCaptured() {
	lsr.mu.Lock()
	lsr.captured = nil
	lsr.mu.Unlock()
}

// Emit fans a record out to every attached sink whose threshold admits
// it. Emission is gated on the "log" policy scope: when a policy
// uninstalls (or denies emit on) the scope the record is dropped
// silently — logging must never abort the program. A record below the
// global level is dropped before fan-out.
func (lsr *LogSinkRegistry) Emit(r *Registry, rec LogRecord) {
	if pol := HostPolicy(r); pol != nil {
		if pol.Check("log", "emit", policy.Args{"level": rec.Severity.Name()}) != nil {
			return
		}
	}
	lsr.mu.Lock()
	if rec.Severity < lsr.level {
		lsr.mu.Unlock()
		return
	}
	order := append([]string(nil), lsr.attached...)
	sinks := make([]*logSink, 0, len(order))
	for _, n := range order {
		sinks = append(sinks, lsr.available[n])
	}
	lsr.mu.Unlock()
	for _, s := range sinks {
		if s == nil || rec.Severity < s.min {
			continue
		}
		_ = s.emit(r, rec) // a sink error must never crash the caller
	}
}

// render formats a record for the console sink per the active format.
func (lsr *LogSinkRegistry) render(rec LogRecord) string {
	if lsr.Format() == "json" {
		return renderJSON(rec)
	}
	return renderText(rec)
}

// renderText produces a human-readable line:
//
//	2021-01-01T00:00:00Z INFO  message key=val key2=val2
func renderText(rec LogRecord) string {
	out := rec.Timestamp.UTC().Format(time.RFC3339) + " " +
		PadRight(rec.Severity.Text(), 5) + " " + FormatForPrint(rec.Body)
	if m, err := AsMap(rec.attributesOrEmpty()); err == nil && m != nil {
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			out += " " + k + "=" + FormatForPrint(v)
		}
	}
	return out
}

// renderJSON produces a one-line JSON object for log shippers. Falls
// back to the text form if marshalling ever fails.
func renderJSON(rec LogRecord) string {
	obj := map[string]any{
		"time":     rec.Timestamp.UTC().Format(time.RFC3339),
		"level":    rec.Severity.Text(),
		"severity": int(rec.Severity),
		"msg":      eng.ToNative(rec.Body),
	}
	if attrs, ok := eng.ToNative(rec.attributesOrEmpty()).(map[string]any); ok && len(attrs) > 0 {
		obj["attributes"] = attrs
	}
	bs, err := json.Marshal(obj)
	if err != nil {
		return renderText(rec)
	}
	return string(bs)
}

// LogModuleNativeFuncs builds the Go-implemented words for the aql:log
// module, all closing over the single per-import sink registry `lsr`.
// They are registered ONLY into the module's sub-registry by
// modules.BuildLogModule — deliberately absent from the global
// registry, like the aql:io words.
func LogModuleNativeFuncs(lsr *LogSinkRegistry) []NativeFunc {
	funcs := []NativeFunc{
		logSetLevelNative(lsr),
		logGetLevelNative(lsr),
		logEnabledNative(lsr),
		logSetFormatNative(lsr),
		logGetFormatNative(lsr),
		logAddSinkNative(lsr),
		logRemoveSinkNative(lsr),
		logSinksNative(lsr),
		logDumpNative(lsr),
		logClearNative(lsr),
		logGenericNative(lsr),
	}
	for _, lvl := range []struct {
		word  string
		level LogLevel
	}{
		{"log-trace", LevelTrace},
		{"log-debug", LevelDebug},
		{"log-info", LevelInfo},
		{"log-warn", LevelWarn},
		{"log-error", LevelError},
		{"log-fatal", LevelFatal},
	} {
		funcs = append(funcs, levelNative(lsr, lvl.word, lvl.level))
	}
	return funcs
}

// emitRecord builds and fans out a record at the given level. attrs is
// a concrete Map or an empty placeholder Value.
func emitRecord(lsr *LogSinkRegistry, r *Registry, level LogLevel, body, attrs Value) {
	lsr.Emit(r, LogRecord{
		Timestamp:  EffectiveClock(r).Now(),
		Severity:   level,
		Body:       body,
		Attributes: attrs,
	})
}

// levelNative builds one severity word (Log.info, …). The 2-arg
// signature (message + fields) is listed FIRST so a call with a
// trailing Map binds it as structured fields rather than the 1-arg
// form greedily claiming only the message.
func levelNative(lsr *LogSinkRegistry, word string, level LogLevel) NativeFunc {
	return NativeFunc{
		Name: word,
		Signatures: []NativeSig{
			{
				Args:       []*Type{TAny, TMap},
				Returns:    []*Type{},
				BarrierPos: -1,
				Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					emitRecord(lsr, r, level, args[0], args[1])
					return nil, nil
				},
			},
			{
				Args:       []*Type{TAny},
				Returns:    []*Type{},
				BarrierPos: -1,
				Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					emitRecord(lsr, r, level, args[0], Value{})
					return nil, nil
				},
			},
		},
	}
}

// logGenericNative is `Log.log LEVEL MESSAGE [FIELDS]` — emit at a
// computed level.
func logGenericNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-log",
		Signatures: []NativeSig{
			{
				Args:       []*Type{TAtom, TAny, TMap},
				Returns:    []*Type{},
				BarrierPos: -1,
				Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					lvl, err := levelArg(r, args[0], "Log.log")
					if err != nil {
						return nil, err
					}
					emitRecord(lsr, r, lvl, args[1], args[2])
					return nil, nil
				},
			},
			{
				Args:       []*Type{TAtom, TAny},
				Returns:    []*Type{},
				BarrierPos: -1,
				Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					lvl, err := levelArg(r, args[0], "Log.log")
					if err != nil {
						return nil, err
					}
					emitRecord(lsr, r, lvl, args[1], Value{})
					return nil, nil
				},
			},
		},
	}
}

// levelArg reads a level atom and resolves it, raising log_error on an
// unknown name.
func levelArg(r *Registry, v Value, word string) (LogLevel, error) {
	name, err := v.AsConcreteAtom()
	if err != nil {
		return 0, r.AqlError("log_error", "level must be an atom (trace/debug/info/warn/error/fatal)", word)
	}
	lvl, ok := parseLevel(name)
	if !ok {
		return 0, r.AqlError("log_error", "unknown level: "+name, word)
	}
	return lvl, nil
}

func logSetLevelNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-set-level",
		Signatures: []NativeSig{{
			Args:       []*Type{TAtom},
			Returns:    []*Type{},
			BarrierPos: -1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				lvl, err := levelArg(r, args[0], "Log.set-level")
				if err != nil {
					return nil, err
				}
				lsr.SetLevel(lvl)
				return nil, nil
			},
		}},
	}
}

func logGetLevelNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-get-level",
		Signatures: []NativeSig{{
			Args:       []*Type{},
			Returns:    []*Type{TAtom},
			BarrierPos: -1,
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewAtom(lsr.Level().Name())}, nil
			},
		}},
	}
}

func logEnabledNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-enabled",
		Signatures: []NativeSig{{
			Args:       []*Type{TAtom},
			Returns:    []*Type{TBoolean},
			BarrierPos: -1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				lvl, err := levelArg(r, args[0], "Log.enabled")
				if err != nil {
					return nil, err
				}
				return []Value{NewBoolean(lvl >= lsr.Level())}, nil
			},
		}},
	}
}

func logSetFormatNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-set-format",
		Signatures: []NativeSig{{
			Args:       []*Type{TAtom},
			Returns:    []*Type{},
			BarrierPos: -1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[0].AsConcreteAtom()
				if err != nil {
					return nil, r.AqlError("log_error", "format must be an atom (text/json)", "Log.set-format")
				}
				if name != "text" && name != "json" {
					return nil, r.AqlError("log_error", "unknown format: "+name+" (want text or json)", "Log.set-format")
				}
				lsr.SetFormat(name)
				return nil, nil
			},
		}},
	}
}

func logGetFormatNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-get-format",
		Signatures: []NativeSig{{
			Args:       []*Type{},
			Returns:    []*Type{TAtom},
			BarrierPos: -1,
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewAtom(lsr.Format())}, nil
			},
		}},
	}
}

func logAddSinkNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-add-sink",
		Signatures: []NativeSig{{
			Args:       []*Type{TAtom},
			Returns:    []*Type{},
			BarrierPos: -1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[0].AsConcreteAtom()
				if err != nil {
					return nil, r.AqlError("log_error", "sink name must be an atom", "Log.add-sink")
				}
				if err := checkLogInstall(r, "Log.add-sink", name); err != nil {
					return nil, err
				}
				if !lsr.Available(name) {
					return nil, r.AqlError("unknown-sink", "no registered sink named "+name, "Log.add-sink")
				}
				lsr.Attach(name)
				return nil, nil
			},
		}},
	}
}

func logRemoveSinkNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-remove-sink",
		Signatures: []NativeSig{{
			Args:       []*Type{TAtom},
			Returns:    []*Type{},
			BarrierPos: -1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[0].AsConcreteAtom()
				if err != nil {
					return nil, r.AqlError("log_error", "sink name must be an atom", "Log.remove-sink")
				}
				if err := checkLogInstall(r, "Log.remove-sink", name); err != nil {
					return nil, err
				}
				lsr.Detach(name)
				return nil, nil
			},
		}},
	}
}

// checkLogInstall gates a sink-management op on the "log" policy scope.
// Unlike emission (which drops silently), management ops surface the
// denial as an error so the caller knows the change did not take.
func checkLogInstall(r *Registry, word, sink string) error {
	pol := HostPolicy(r)
	if pol == nil {
		return nil
	}
	if err := pol.Check("log", "install", policy.Args{"sink": sink}); err != nil {
		return r.AqlError("log_error", "sink management denied by policy: "+err.Error(), word)
	}
	return nil
}

func logSinksNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-sinks",
		Signatures: []NativeSig{{
			Args:       []*Type{},
			Returns:    []*Type{TList},
			BarrierPos: -1,
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				names := lsr.Attached()
				out := make([]Value, len(names))
				for i, n := range names {
					out[i] = NewAtom(n)
				}
				return []Value{NewList(out)}, nil
			},
		}},
	}
}

func logDumpNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-dump",
		Signatures: []NativeSig{{
			Args:       []*Type{},
			Returns:    []*Type{TList},
			BarrierPos: -1,
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				recs := lsr.Captured()
				out := make([]Value, len(recs))
				for i, rec := range recs {
					out[i] = rec.toMap()
				}
				return []Value{NewList(out)}, nil
			},
		}},
	}
}

func logClearNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-clear",
		Signatures: []NativeSig{{
			Args:       []*Type{},
			Returns:    []*Type{},
			BarrierPos: -1,
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				lsr.ClearCaptured()
				return nil, nil
			},
		}},
	}
}
