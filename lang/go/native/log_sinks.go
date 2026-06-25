package native

import (
	"fmt"
	"strings"

	"github.com/aql-lang/aql/eng/go"
)

// Span is a host-facing, read-only view of a span, passed to a host
// sink's OnSpanStart / OnSpanEnd (phase 4). It exposes the W3C trace
// context, the span name and attributes, the status, and the start/end
// times — everything an OTel/Datadog bridge needs to translate the span
// without reaching into the engine's internal state.
type Span struct {
	TraceID       string
	SpanID        string
	ParentID      string
	Name          string
	Attributes    map[string]any
	Status        string // "" (ok) | "error"
	StatusMessage string
	StartUnixNano int64
	EndUnixNano   int64
}

// newSpanView projects internal span state into the host-facing view.
func newSpanView(sp *spanState) Span {
	var attrs map[string]any
	if sp.attrs != nil {
		if m, ok := eng.ToNative(NewMap(sp.attrs)).(map[string]any); ok {
			attrs = m
		}
	}
	v := Span{
		TraceID:       sp.traceID,
		SpanID:        sp.spanID,
		ParentID:      sp.parentID,
		Name:          sp.name,
		Attributes:    attrs,
		Status:        sp.status,
		StatusMessage: sp.statusMsg,
		StartUnixNano: sp.start.UnixNano(),
	}
	if !sp.end.IsZero() {
		v.EndUnixNano = sp.end.UnixNano()
	}
	return v
}

// log_sinks.go — phase 3 of aql:log: provider hooks. A sink is a named
// consumer of records (and, in later phases, spans and measurements).
// Beyond the three built-ins, sinks are pluggable two ways, modelled on
// the parse/emit host-registration pattern:
//
//   - from Go, a host installs a sink with RegisterHostLogSink, passing
//     a LogSinkSpec — e.g. an OpenTelemetry / Datadog bridge that
//     translates the neutral LogRecord to its SDK. This is the seam
//     that keeps the OTel SDK a HOST dependency, never a runtime one.
//   - from AQL, `Log.register FN NAME MIN` installs an AQL function as a
//     sink: FN is invoked with the record Map for every admitted record.
//
// Registering a sink also attaches it (the common intent — you
// registered it because you want it live); `Log.remove-sink` detaches
// without unregistering, and `Log.add-sink` re-attaches.

// LogSinkSpec is the host-registration envelope (the parse/emit Spec
// shape applied to logging). OnRecord is required; OnSpanStart/OnSpanEnd
// (phase 4) and OnMeasure (phase 5) are optional, so a logs-only sink
// leaves them nil. Name is a lowercase, unprefixed kind atom; MinLevel
// drops records below it before the handler runs.
type LogSinkSpec struct {
	Name        string
	MinLevel    LogLevel
	OnRecord    func(LogRecord) error
	OnSpanStart func(Span)
	OnSpanEnd   func(Span)
	OnMeasure   func(Measurement)
}

// RegisterHostLogSink installs a host sink on r, creating and installing
// a default LogSinkRegistry first if the module has not been imported
// yet (so a host can pre-attach an OTel sink before `import "aql:log"`).
// The sink is registered AND attached. Validation errors (bad name,
// nil OnRecord, name collision) are returned.
func RegisterHostLogSink(r *Registry, spec LogSinkSpec) error {
	lsr := HostLogSinks(r)
	if lsr == nil {
		lsr = NewLogSinkRegistry()
		SetHostLogSinks(r, lsr)
		// SetHostLogSinks is a no-op under a policy that uninstalls the
		// log scope; in that case there is nothing to register into.
		if HostLogSinks(r) == nil {
			return fmt.Errorf("log: sink registry not installed (log scope disabled by policy)")
		}
	}
	return lsr.RegisterSink(spec)
}

// validSinkName rejects empty / non-lowercase / whitespace-bearing
// names, keeping sink atoms a clean kind namespace.
func validSinkName(name string) error {
	if name == "" {
		return fmt.Errorf("log: sink name must not be empty")
	}
	if name != strings.ToLower(name) || strings.ContainsAny(name, " \t\n") {
		return fmt.Errorf("log: sink name %q must be lowercase with no spaces", name)
	}
	return nil
}

// installSink validates a sink's name against the existing set, then
// registers and attaches it. Shared by every registration path.
func (lsr *LogSinkRegistry) installSink(s *logSink) error {
	if err := validSinkName(s.name); err != nil {
		return err
	}
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	if _, exists := lsr.available[s.name]; exists {
		return fmt.Errorf("log: a sink named %q is already registered", s.name)
	}
	lsr.available[s.name] = s
	lsr.attached = append(lsr.attached, s.name)
	return nil
}

// RegisterSink installs a host LogSinkSpec. OnRecord must be non-nil.
func (lsr *LogSinkRegistry) RegisterSink(spec LogSinkSpec) error {
	if spec.OnRecord == nil {
		return fmt.Errorf("log: sink %q has a nil OnRecord handler", spec.Name)
	}
	s := &logSink{
		name: spec.Name,
		min:  spec.MinLevel,
		emit: func(_ *Registry, rec LogRecord) error { return spec.OnRecord(rec) },
	}
	if spec.OnSpanStart != nil {
		s.spanStart = func(_ *Registry, sp *spanState) { spec.OnSpanStart(newSpanView(sp)) }
	}
	if spec.OnSpanEnd != nil {
		s.spanEnd = func(_ *Registry, sp *spanState) { spec.OnSpanEnd(newSpanView(sp)) }
	}
	if spec.OnMeasure != nil {
		s.measure = func(_ *Registry, m Measurement) { spec.OnMeasure(m) }
	}
	return lsr.installSink(s)
}

// registerFnSink installs an AQL function as a sink. The function is
// invoked (in a sub-engine over the live registry, so it resolves
// module / global defs) with the record Map for every admitted record.
func (lsr *LogSinkRegistry) registerFnSink(name string, min LogLevel, fn Value) error {
	s := &logSink{
		name: name,
		min:  min,
		emit: func(r *Registry, rec LogRecord) error {
			sub := New(r)
			if _, err := sub.Run([]Value{rec.toMap(), fn}); err != nil {
				return fmt.Errorf("log sink %q: %w", name, err)
			}
			return nil
		},
	}
	return lsr.installSink(s)
}

// logRegisterNative is `Log.register FN NAME MIN` — install an AQL
// function as a sink at the given minimum level.
func logRegisterNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-register",
		Signatures: []NativeSig{{
			Args:       []*Type{TFunction, TAtom, TAtom},
			Returns:    []*Type{},
			BarrierPos: -1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[1].AsConcreteAtom()
				if err != nil {
					return nil, r.AqlError("log_error", "sink name must be an atom", "Log.register")
				}
				min, err := levelArg(r, args[2], "Log.register")
				if err != nil {
					return nil, err
				}
				if err := checkLogInstall(r, "Log.register", name); err != nil {
					return nil, err
				}
				if err := lsr.registerFnSink(name, min, args[0]); err != nil {
					return nil, r.AqlError("sink-exists", err.Error(), "Log.register")
				}
				return nil, nil
			},
		}},
	}
}
