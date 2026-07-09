package lang

import (
	"os"
	"testing"

	"github.com/aql-lang/aql/lang/go/capabilities"
)

// design/CHECK-FALSE-POSITIVES.0.md — corpus of programs that RUN CORRECTLY yet
// the static pre-flight check emitted ERROR-severity diagnostics, which blocks
// `aql run` by default. Each case asserts the plain `Check` pass — the one the
// run pre-flight gates on — produces no error-level diagnostic (info/warning
// are fine).
//
// Root cause (isolated): a module-fn wrapper whose connection parameter was
// typed `Any` (mini-redis's `redis-cmd [ep:Any …]`). An `Any` forward slot lets
// matchSignature collect the FOLLOWING function word (`drop`) past the
// function-word barrier on the insertForward re-entry, which flips the
// wrapper's dispatch off the clean declared-return path onto the inline
// body-splice path. That path leaks the body's residual (`reply.line` + the
// unconsumed `call` result); inside a statically-counted `for`, the
// spread-residual model repeats it per iteration and the end-of-run re-eval
// re-dispatches the leaked Function as the next call's `ep`, mistyping the
// Service as a Function → the spurious `no_signature` (and, on the compile
// pass, an `undefined_word: expires` cascade from the same leak).
//
// Fix: type the connection parameter concretely — `redis-cmd [ep:Service …]`
// (an Endpoint from Net.connect IS a Service, and the body's `call` requires
// one). The concrete slot no longer collects the trailing `drop`, so dispatch
// stays on the declared-return path and nets its single return. This mirrors
// the echo benchmark's `sock:Any` → `sock:Socket` resolution.

// loadApps returns a lang.AQL whose import resolver is backed by an in-memory FS
// holding the named example apps at /apps/<name>.
func loadApps(t *testing.T, names ...string) *AQL {
	t.Helper()
	mem := capabilities.NewMem()
	for _, n := range names {
		src, err := os.ReadFile("../../design/examples/apps/" + n)
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		mem.Files["/apps/"+n] = src
	}
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	a.SetFileOps(mem)
	return a
}

// errorDiags returns the error-severity diagnostics for src — the ones that
// abort `aql run`. It runs the SAME pass the run pre-flight gates on: plain
// `Check` (cmd/go/internal/check.Preflight → a.Check), NOT the stricter compile
// pass. A false positive here is what actually refuses an otherwise-correct
// program by default.
func errorDiags(t *testing.T, a *AQL, src string) []CheckDiagnostic {
	t.Helper()
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	var errs []CheckDiagnostic
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			errs = append(errs, d)
		}
	}
	return errs
}

// Two service calls over the SAME endpoint in one loop body must not spuriously
// fail the second call's dispatch. Before the `ep:Service` fix the first
// `MiniRedis.cmd … drop` leaked its residual, which the spread-residual model
// repeated and the end-of-run re-eval re-dispatched as a Function for `ep`, so
// `call` reported `got (Map, Function, Map); nearest [Map Service Map]`. The
// program runs correctly (`-no-check`); it now also passes the default gate.
func TestCheckNoFalsePositiveMiniRedisLoop(t *testing.T) {
	a := loadApps(t, "mini-redis.aql")
	src := `import "/apps/mini-redis.aql"
import "aql:net"
def ln (MiniRedis.serve {port: 0})
def ep (MiniRedis.connect (join "" ["127.0.0.1:" (convert String (Net.addr ln).port)]))
for 3 [ MiniRedis.cmd ep "SET k v" drop  MiniRedis.cmd ep "GET k" drop ]`
	if errs := errorDiags(t, a, src); len(errs) > 0 {
		for _, d := range errs {
			t.Errorf("false-positive check error: [%s] %s (%d:%d)", d.Code, d.Detail, d.Row, d.Col)
		}
	}
}

// The full app driver shape: before the fix the leaked residual cascaded into
// the following `def dur (…)` (a widened carrier in its name slot) and produced
// `undefined_word: dur` plus a dependent `no_signature`. None are real; with a
// concrete `ep` the whole driver checks clean.
func TestCheckNoFalsePositiveMiniRedisDriver(t *testing.T) {
	a := loadApps(t, "mini-redis.aql")
	src := `import "/apps/mini-redis.aql"
import "aql:net"
import "aql:time-util"
def ln (MiniRedis.serve {port: 0})
def ep (MiniRedis.connect (join "" ["127.0.0.1:" (convert String (Net.addr ln).port)]))
MiniRedis.cmd ep "SET k hello" drop
def start (TimeUtil.now)
for 5000 [ MiniRedis.cmd ep "SET k hello" drop  MiniRedis.cmd ep "GET k" drop ]
def dur (TimeUtil.total-ms (TimeUtil.elapsed start))
print (join "" ["ops " (convert String dur) " " (convert String (div dur 10000000.0))])`
	if errs := errorDiags(t, a, src); len(errs) > 0 {
		for _, d := range errs {
			t.Errorf("false-positive check error: [%s] %s (%d:%d)", d.Code, d.Detail, d.Row, d.Col)
		}
	}
}

// NEGATIVE guard: clearing the false positive must not silence GENUINE loop-body
// errors. An undefined word inside the same two-dispatch loop body must still be
// reported — proving the resolution restored precision (a concrete parameter
// type) rather than blanket-suppressing loop-body diagnostics.
func TestCheckLoopBodyGenuineErrorStillReports(t *testing.T) {
	a := loadApps(t, "mini-redis.aql")
	src := `import "/apps/mini-redis.aql"
import "aql:net"
def ln (MiniRedis.serve {port: 0})
def ep (MiniRedis.connect (join "" ["127.0.0.1:" (convert String (Net.addr ln).port)]))
for 3 [ MiniRedis.cmd ep "SET k v" drop  no-such-word-xyz ep drop ]`
	errs := errorDiags(t, a, src)
	found := false
	for _, d := range errs {
		if d.Code == "undefined_word" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an undefined_word error for no-such-word-xyz inside the loop; got %d error(s): %v", len(errs), errs)
	}
}
