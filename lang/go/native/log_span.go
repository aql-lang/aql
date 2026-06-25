package native

import (
	"fmt"
)

// log_span.go — phase 4 of aql:log: the OpenTelemetry *traces* signal.
//
// `Log.span NAME [ATTRS]` starts a span and returns a Span instance (an
// OrderedMap carrying the trace/span ids plus method closures). The span
// is pushed onto the registry's active-span stack, so any record emitted
// while it is open is stamped with its trace-id / span-id (the neutral
// trace-context propagation — no separate machinery). `Log.with-span
// NAME BODY` brackets a body: it starts a span, runs the body, records a
// raised error as the span status, ends the span, and re-raises. Span
// methods — `set-attr`, `add-event`, `record-error`, `end` — mutate the
// shared span state. Ended spans fan out to every attached sink's
// span-end hook (the memory sink captures them for `Log.traces`); a host
// OTel sink's OnSpanEnd translates them to OTel spans.

// startSpan mints a span, pushes it active, and notifies span-start
// hooks. A child (started while another span is open) inherits the
// parent's trace-id and records the parent's span-id; a root mints a new
// trace-id. Ids are derived from a per-registry counter so they are
// deterministic under a frozen clock.
func (lsr *LogSinkRegistry) startSpan(r *Registry, name string, attrs *OrderedMap) *spanState {
	if attrs == nil {
		attrs = NewOrderedMap()
	}
	lsr.mu.Lock()
	lsr.spanSeq++
	seq := lsr.spanSeq
	st := &spanState{
		spanID: fmt.Sprintf("%016x", seq),
		name:   name,
		attrs:  attrs,
		start:  EffectiveClock(r).Now(),
	}
	if n := len(lsr.spanStack); n > 0 {
		parent := lsr.spanStack[n-1]
		st.traceID = parent.traceID
		st.parentID = parent.spanID
	} else {
		st.traceID = fmt.Sprintf("%032x", seq)
	}
	lsr.spanStack = append(lsr.spanStack, st)
	sinks := lsr.attachedSinksLocked()
	lsr.mu.Unlock()
	for _, s := range sinks {
		if s.spanStart != nil {
			s.spanStart(r, st)
		}
	}
	return st
}

// endSpan finalises a span (idempotent), pops it if it is the active
// top, and notifies span-end hooks.
func (lsr *LogSinkRegistry) endSpan(r *Registry, st *spanState) {
	lsr.mu.Lock()
	if st.ended {
		lsr.mu.Unlock()
		return
	}
	st.ended = true
	st.end = EffectiveClock(r).Now()
	if n := len(lsr.spanStack); n > 0 && lsr.spanStack[n-1] == st {
		lsr.spanStack = lsr.spanStack[:n-1]
	}
	sinks := lsr.attachedSinksLocked()
	lsr.mu.Unlock()
	for _, s := range sinks {
		if s.spanEnd != nil {
			s.spanEnd(r, st)
		}
	}
}

// attachedSinksLocked returns the attached sinks in order. Caller holds
// lsr.mu.
func (lsr *LogSinkRegistry) attachedSinksLocked() []*logSink {
	out := make([]*logSink, 0, len(lsr.attached))
	for _, n := range lsr.attached {
		if s := lsr.available[n]; s != nil {
			out = append(out, s)
		}
	}
	return out
}

// Traces returns the ended spans captured by the memory sink (a copy).
func (lsr *LogSinkRegistry) Traces() []*spanState {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	return append([]*spanState(nil), lsr.spans...)
}

// spanToMap renders a span as a Record-shaped Map for Log.traces.
func spanToMap(st *spanState) Value {
	m := NewOrderedMap()
	m.Set("trace-id", NewString(st.traceID))
	m.Set("span-id", NewString(st.spanID))
	m.Set("parent-id", NewString(st.parentID))
	m.Set("name", NewString(st.name))
	m.Set("attributes", NewMap(cloneOrderedMap(st.attrs)))
	status := st.status
	if status == "" {
		status = "ok"
	}
	m.Set("status", NewString(status))
	m.Set("status-message", NewString(st.statusMsg))
	events := make([]Value, len(st.events))
	for i, e := range st.events {
		em := NewOrderedMap()
		em.Set("name", NewString(e.Name))
		em.Set("timestamp", NewDateTime(e.Timestamp))
		em.Set("attributes", e.Attributes)
		events[i] = NewMap(em)
	}
	m.Set("events", NewList(events))
	m.Set("start", NewDateTime(st.start))
	m.Set("end", NewDateTime(st.end))
	return NewMap(m)
}

// cloneOrderedMap returns a shallow copy, or a fresh empty map for nil.
func cloneOrderedMap(m *OrderedMap) *OrderedMap {
	out := NewOrderedMap()
	if m != nil {
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			out.Set(k, v)
		}
	}
	return out
}

// buildSpanInstance constructs the Span value: an OrderedMap carrying
// the trace/span ids and name as fields, plus the method closures
// (set-attr / add-event / record-error / end), all sharing st.
func buildSpanInstance(st *spanState, lsr *LogSinkRegistry) (*OrderedMap, error) {
	subReg, err := DefaultRegistry()
	if err != nil {
		return nil, err
	}
	natives := spanNatives(st, lsr)
	for _, n := range natives {
		subReg.RegisterNativeFunc(n)
	}
	inst := NewOrderedMap()
	inst.Set("trace-id", NewString(st.traceID))
	inst.Set("span-id", NewString(st.spanID))
	inst.Set("name", NewString(st.name))
	for _, n := range natives {
		inst.Set(spanExportName(n.Name), wrapLoggerFnDef(n, subReg))
	}
	return inst, nil
}

func spanExportName(inner string) string { return inner[len("span-"):] }

// spanNatives builds the span instance methods closing over st (+lsr for
// end).
func spanNatives(st *spanState, lsr *LogSinkRegistry) []NativeFunc {
	return []NativeFunc{
		{
			Name: "span-set-attr",
			Signatures: []NativeSig{{
				Args:       []*Type{TAtom, TAny},
				Returns:    []*Type{},
				BarrierPos: -1,
				Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					key, err := args[0].AsConcreteAtom()
					if err != nil {
						return nil, r.AqlError("log_error", "attribute key must be an atom", "Span.set-attr")
					}
					lsr.mu.Lock()
					st.attrs.Set(key, args[1])
					lsr.mu.Unlock()
					return nil, nil
				},
			}},
		},
		{
			Name: "span-add-event",
			Signatures: []NativeSig{
				{
					Args:       []*Type{TString, TMap},
					Returns:    []*Type{},
					BarrierPos: -1,
					Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
						return spanAddEvent(lsr, r, st, args[0], args[1])
					},
				},
				{
					Args:       []*Type{TString},
					Returns:    []*Type{},
					BarrierPos: -1,
					Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
						return spanAddEvent(lsr, r, st, args[0], NewMap(NewOrderedMap()))
					},
				},
			},
		},
		{
			Name: "span-record-error",
			Signatures: []NativeSig{{
				Args:       []*Type{TAny},
				Returns:    []*Type{},
				BarrierPos: -1,
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					lsr.mu.Lock()
					st.status = "error"
					st.statusMsg = FormatForPrint(args[0])
					lsr.mu.Unlock()
					return nil, nil
				},
			}},
		},
		{
			// Exported as `finish`, not `end`: `end` is the statement
			// separator token, so a `.end` dot-access would terminate the
			// statement instead of reading the method.
			Name: "span-finish",
			Signatures: []NativeSig{{
				Args:       []*Type{},
				Returns:    []*Type{},
				BarrierPos: -1,
				Handler: func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					lsr.endSpan(r, st)
					return nil, nil
				},
			}},
		},
	}
}

func spanAddEvent(lsr *LogSinkRegistry, r *Registry, st *spanState, name, attrs Value) ([]Value, error) {
	n, err := name.AsConcreteString()
	if err != nil {
		return nil, r.AqlError("log_error", "event name must be a string", "Span.add-event")
	}
	lsr.mu.Lock()
	st.events = append(st.events, spanEvent{Name: n, Timestamp: EffectiveClock(r).Now(), Attributes: attrs})
	lsr.mu.Unlock()
	return nil, nil
}

// --- top-level span words ---------------------------------------------------

func logSpanNative(lsr *LogSinkRegistry) NativeFunc {
	start := func(r *Registry, name string, attrs *OrderedMap) ([]Value, error) {
		st := lsr.startSpan(r, name, attrs)
		inst, err := buildSpanInstance(st, lsr)
		if err != nil {
			return nil, r.AqlError("log_error", err.Error(), "Log.span")
		}
		return []Value{NewMap(inst)}, nil
	}
	return NativeFunc{
		Name: "log-span",
		Signatures: []NativeSig{
			{
				Args:       []*Type{TString, TMap},
				Returns:    []*Type{TMap},
				BarrierPos: -1,
				ReturnsFn:  spanShapeReturns(lsr),
				Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					name, err := args[0].AsConcreteString()
					if err != nil {
						return nil, r.AqlError("log_error", "span name must be a string", "Log.span")
					}
					return start(r, name, asConcreteOrderedMap(args[1]))
				},
			},
			{
				Args:       []*Type{TString},
				Returns:    []*Type{TMap},
				BarrierPos: -1,
				ReturnsFn:  spanShapeReturns(lsr),
				Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					name, err := args[0].AsConcreteString()
					if err != nil {
						return nil, r.AqlError("log_error", "span name must be a string", "Log.span")
					}
					return start(r, name, nil)
				},
			},
		},
	}
}

// logWithSpanNative is `Log.with-span NAME BODY` — run BODY inside a
// span, record a raised error, end the span, and re-raise on error.
func logWithSpanNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-with-span",
		Signatures: []NativeSig{{
			Args:       []*Type{TString, TList},
			Returns:    []*Type{TAny},
			NoEvalArgs: map[int]bool{1: true},
			BarrierPos: -1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[0].AsConcreteString()
				if err != nil {
					return nil, r.AqlError("log_error", "span name must be a string", "Log.with-span")
				}
				body, err := RequireConcreteList(args[1], "Log.with-span body")
				if err != nil {
					return nil, err
				}
				st := lsr.startSpan(r, name, nil)
				sub := New(r)
				out, runErr := sub.Run(append([]Value(nil), body.Slice()...))
				if runErr != nil {
					lsr.mu.Lock()
					st.status = "error"
					st.statusMsg = runErr.Error()
					lsr.mu.Unlock()
					lsr.endSpan(r, st)
					return nil, runErr
				}
				lsr.endSpan(r, st)
				if len(out) > 0 {
					return []Value{out[len(out)-1]}, nil
				}
				return nil, nil
			},
		}},
	}
}

// logEndNative is `Log.end SPAN` — end a span given its instance.
func logEndNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-end",
		Signatures: []NativeSig{{
			Args:       []*Type{TMap},
			Returns:    []*Type{},
			BarrierPos: -1,
			Handler: func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				m, err := RequireConcreteMap(args[0], "Log.end")
				if err != nil {
					return nil, err
				}
				sidVal, ok := m.Get("span-id")
				if !ok {
					return nil, r.AqlError("log_error", "Log.end expects a span (no span-id field)", "Log.end")
				}
				sid, _ := sidVal.AsConcreteString()
				lsr.mu.Lock()
				var target *spanState
				if n := len(lsr.spanStack); n > 0 && lsr.spanStack[n-1].spanID == sid {
					target = lsr.spanStack[n-1]
				}
				lsr.mu.Unlock()
				if target == nil {
					return nil, r.AqlError("span-mismatch", "Log.end: span "+sid+" is not the active span", "Log.end")
				}
				lsr.endSpan(r, target)
				return nil, nil
			},
		}},
	}
}

// logCurrentSpanNative is `Log.current-span` — the active span instance,
// or None.
func logCurrentSpanNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-current-span",
		Signatures: []NativeSig{{
			Args:       []*Type{},
			Returns:    []*Type{TAny},
			BarrierPos: -1,
			Handler: func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				lsr.mu.Lock()
				var st *spanState
				if n := len(lsr.spanStack); n > 0 {
					st = lsr.spanStack[n-1]
				}
				lsr.mu.Unlock()
				if st == nil {
					return []Value{NewTypeLiteral(TNone)}, nil
				}
				inst, err := buildSpanInstance(st, lsr)
				if err != nil {
					return nil, r.AqlError("log_error", err.Error(), "Log.current-span")
				}
				return []Value{NewMap(inst)}, nil
			},
		}},
	}
}

// logTracesNative is `Log.traces` — the ended spans captured by the
// memory sink.
func logTracesNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-traces",
		Signatures: []NativeSig{{
			Args:       []*Type{},
			Returns:    []*Type{TList},
			BarrierPos: -1,
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				spans := lsr.Traces()
				out := make([]Value, len(spans))
				for i, st := range spans {
					out[i] = spanToMap(st)
				}
				return []Value{NewList(out)}, nil
			},
		}},
	}
}

// spanShapeReturns is the check-mode shape for Log.span: a concrete Span
// instance Map (shape-only) so a downstream `s.set-attr` / `s.end`
// resolves the method wrapper statically. Mirrors loggerShapeReturns.
func spanShapeReturns(lsr *LogSinkRegistry) func([]Value, *Registry) []Value {
	return func(_ []Value, _ *Registry) []Value {
		inst, err := buildSpanInstance(&spanState{attrs: NewOrderedMap()}, lsr)
		if err != nil {
			return []Value{NewCarrier(TMap)}
		}
		return []Value{NewMap(inst)}
	}
}
