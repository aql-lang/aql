package lang

import (
	"os"
	"testing"

	"github.com/aql-lang/aql/lang/go/capabilities"
)

// design/CHECK-FALSE-POSITIVES.0.md — corpus of programs that RUN CORRECTLY yet
// the static pre-flight check emits ERROR-severity diagnostics, which blocks
// `aql run` by default. Each case asserts CompileCheck produces no error-level
// diagnostic (info/warning are fine). The root is check-mode carrier types
// degrading across repeated dispatch in a loop (a value bound outside the loop
// widens to __FN / dynamic(Any) on a later iteration → spurious no_signature).

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

// errorDiags returns the error-severity diagnostics for src (the ones that abort
// `aql run`), so a test can assert there are none.
func errorDiags(t *testing.T, a *AQL, src string) []CheckDiagnostic {
	t.Helper()
	_, _, res, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
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
// fail the second call's dispatch. On the first analysis the endpoint carries a
// concrete Service; without the fix the second widens it to __FN and `call`
// reports `no matching signature … got (Map, __FN, Map); nearest [Map Service
// Map]` inside mini-redis. The program runs correctly (`-no-check`).
func TestCheckNoFalsePositiveMiniRedisLoop(t *testing.T) {
	t.Skip("pending fix — design/CHECK-FALSE-POSITIVES.0.md Stage 2 (check-mode " +
		"carrier `ep` degrades to Function during the loop-fixpoint × return-type " +
		"probe interaction). Remove this Skip when the precision fix lands.")
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

// The full app driver shape: the loop's carrier degradation cascades into the
// following `def dur (…)` (its name slot mis-resolves to a widened carrier),
// then `undefined_word: dur` and a dependent `no_signature`. None are real.
func TestCheckNoFalsePositiveMiniRedisDriver(t *testing.T) {
	t.Skip("pending fix — design/CHECK-FALSE-POSITIVES.0.md Stage 2. The loop " +
		"carrier degradation cascades into `def dur` (undefined_word) here. Remove " +
		"this Skip when the precision fix lands.")
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
