package modules

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/policy"
)

// fixedLogClock freezes time so emitted record timestamps are
// deterministic.
var fixedLogClock = capabilities.FixedClock{T: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)}

// newLogReg builds a production registry with the module resolver and a
// frozen clock, and returns it plus the buffer wired to ErrOutput (where
// the console sink writes).
func newLogReg(t *testing.T) (*native.Registry, *bytes.Buffer) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	native.SetHostClock(reg, fixedLogClock)
	var buf bytes.Buffer
	reg.ErrOutput = &buf
	return reg, &buf
}

func runLog(t *testing.T, reg *native.Registry, src string) []eng.Value {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := native.NewTop(reg).Run(vals)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out
}

// TestLogConsoleDefault verifies the zero-config path: importing the
// module attaches a console sink that writes one line per record to
// ErrOutput via Go's standard log package, with the level text, the
// frozen timestamp, the message, and the structured fields.
func TestLogConsoleDefault(t *testing.T) {
	reg, buf := newLogReg(t)
	runLog(t, reg, `import "aql:log" ; Log.warn "disk low" {pct:91 mount:"/"}`)
	got := buf.String()
	for _, want := range []string{"2021-01-01T00:00:00Z", "WARN", "disk low", "pct=91", "mount=/"} {
		if !strings.Contains(got, want) {
			t.Errorf("console line %q missing %q", got, want)
		}
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("expected exactly one log line, got %q", got)
	}
}

// TestLogLevelThreshold confirms a record below the global threshold is
// not written to the console at all.
func TestLogLevelThreshold(t *testing.T) {
	reg, buf := newLogReg(t)
	runLog(t, reg, `import "aql:log" ; Log.set-level error/q ; Log.warn "ignored" ; Log.error "kept"`)
	got := buf.String()
	if strings.Contains(got, "ignored") {
		t.Errorf("sub-threshold WARN should be dropped, got %q", got)
	}
	if !strings.Contains(got, "kept") {
		t.Errorf("ERROR at/above threshold should be emitted, got %q", got)
	}
}

// TestLogJSONFormat checks the json console format renders a one-line
// object carrying the OTel severity number and the attributes.
func TestLogJSONFormat(t *testing.T) {
	reg, buf := newLogReg(t)
	runLog(t, reg, `import "aql:log" ; Log.set-format json/q ; Log.info "hi" {n:7}`)
	got := strings.TrimSpace(buf.String())
	for _, want := range []string{`"level":"INFO"`, `"severity":9`, `"msg":"hi"`, `"n":7`} {
		if !strings.Contains(got, want) {
			t.Errorf("json line %q missing %q", got, want)
		}
	}
}

// TestLogMemoryRecordFields inspects a captured record's fields through
// Log.dump — proving the neutral LogRecord shape (timestamp, severity
// number + text, body, attributes).
func TestLogMemoryRecordFields(t *testing.T) {
	reg, _ := newLogReg(t)
	out := runLog(t, reg, `import "aql:log" ; Log.add-sink memory/q ; Log.remove-sink console/q ; Log.error "boom" {code:42} ; Log.dump`)
	if len(out) != 1 {
		t.Fatalf("want one result (the dump list), got %d", len(out))
	}
	list, err := native.RequireConcreteList(out[0], "dump")
	if err != nil || list.Len() != 1 {
		t.Fatalf("want a one-element list, got %v (err %v)", out[0], err)
	}
	rec, err := native.RequireConcreteMap(list.Get(0), "record")
	if err != nil {
		t.Fatalf("record not a map: %v", err)
	}
	wantField := func(key, want string) {
		v, ok := rec.Get(key)
		if !ok {
			t.Errorf("record missing %q", key)
			return
		}
		if got := native.FormatForPrint(v); got != want {
			t.Errorf("record[%q] = %q, want %q", key, got, want)
		}
	}
	wantField("severity", "17")
	wantField("severity-text", "ERROR")
	wantField("body", "boom")
	wantField("timestamp", "2021-01-01T00:00:00")
	attrs, ok := rec.Get("attributes")
	if !ok {
		t.Fatal("record missing attributes")
	}
	am, err := native.RequireConcreteMap(attrs, "attrs")
	if err != nil {
		t.Fatalf("attributes not a map: %v", err)
	}
	if v, ok := am.Get("code"); !ok || native.FormatForPrint(v) != "42" {
		t.Errorf("attributes.code = %v (ok=%v), want 42", v, ok)
	}
}

// TestLogFanOut proves a record reaches BOTH the console and the memory
// sink when both are attached.
func TestLogFanOut(t *testing.T) {
	reg, buf := newLogReg(t)
	out := runLog(t, reg, `import "aql:log" ; Log.add-sink memory/q ; Log.info "twice" ; Log.dump size`)
	if !strings.Contains(buf.String(), "twice") {
		t.Errorf("console sink did not receive the record: %q", buf.String())
	}
	if len(out) != 1 || native.FormatForPrint(out[0]) != "1" {
		t.Errorf("memory sink should hold one record, got %v", out)
	}
}

// TestLogPolicyBlocksSinkInstall verifies the "log" policy scope:
// install=false makes sink management error, and emission a silent
// no-op — logging must never abort the program.
func TestLogPolicyBlocksSinkInstall(t *testing.T) {
	reg, buf := newLogReg(t)
	pol, err := policy.LoadInline(`{ scopes: { log: { install: false } } }`)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	native.SetHostPolicy(reg, pol)

	// add-sink is denied as an error.
	vals, _ := parser.Parse(`import "aql:log" ; Log.add-sink memory/q`)
	if _, err := native.NewTop(reg).Run(vals); err == nil {
		t.Error("add-sink under log.install=false should error")
	}

	// emit is dropped silently (no error, nothing written).
	vals2, _ := parser.Parse(`import "aql:log" ; Log.info "silent"`)
	if _, err := native.NewTop(reg).Run(vals2); err != nil {
		t.Errorf("emit under a restrictive policy must not error, got %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("emit should be dropped under log.install=false, wrote %q", buf.String())
	}
}

// TestLogPolicyAllowsByDefault confirms that with no policy installed
// (or a permissive one) logging works normally.
func TestLogPolicyAllowsByDefault(t *testing.T) {
	reg, buf := newLogReg(t)
	pol, err := policy.LoadInline(`{ scopes: { log: { words: { default: "allow" } } } }`)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	native.SetHostPolicy(reg, pol)
	runLog(t, reg, `import "aql:log" ; Log.info "ok"`)
	if !strings.Contains(buf.String(), "ok") {
		t.Errorf("permissive policy should allow emission, got %q", buf.String())
	}
}

// TestLogHostPreattachedSink verifies the host seam: a LogSinkRegistry
// installed before import (e.g. carrying a host OTel sink) is the one
// the module uses, rather than a fresh default.
func TestLogHostPreattachedSink(t *testing.T) {
	reg, _ := newLogReg(t)
	lsr := native.NewLogSinkRegistry()
	lsr.SetLevel(native.LevelError) // host-chosen threshold
	native.SetHostLogSinks(reg, lsr)
	out := runLog(t, reg, `import "aql:log" ; Log.get-level`)
	if len(out) != 1 || native.FormatForPrint(out[0]) != "error" {
		t.Errorf("module should use the host-installed registry (level error), got %v", out)
	}
}

// TestLogNoPanicTypeLiterals passes bare type literals to every word
// that reads a scalar arg, asserting an error (or clean result) rather
// than a panic — the CLAUDE.md Panic-Prevention discipline.
func TestLogNoPanicTypeLiterals(t *testing.T) {
	srcs := []string{
		`import "aql:log" ; Log.set-level Atom`,
		`import "aql:log" ; Log.enabled Atom`,
		`import "aql:log" ; Log.set-format Atom`,
		`import "aql:log" ; Log.add-sink Atom`,
		`import "aql:log" ; Log.remove-sink Atom`,
		`import "aql:log" ; Log.info Any`,
		`import "aql:log" ; Log.log Atom String`,
	}
	for _, src := range srcs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on %q: %v", src, r)
				}
			}()
			reg, _ := newLogReg(t)
			vals, err := parser.Parse(src)
			if err != nil {
				return // a parse rejection is fine; we only guard against panics
			}
			_, _ = native.NewTop(reg).Run(vals) // error is acceptable; panic is not
		}()
	}
}

// TestLogInstallExports checks the test-setup convenience helper binds
// the Log namespace as a def.
func TestLogInstallExports(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	if err := InstallLogExports(reg); err != nil {
		t.Fatalf("InstallLogExports: %v", err)
	}
	if _, ok := reg.Defs.Top("Log"); !ok {
		t.Error("InstallLogExports did not bind the Log namespace")
	}
}
