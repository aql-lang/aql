package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The const-chokepoint stamp (EmitState.stampFnConst) is an OPTIMISATION: it
// compiles a fn value baked as a const so an apply of it runs on the VM
// instead of islanding. Because it analyses the body against the LIVE emit
// state, it can perturb the ENCLOSING program — and a stamp that makes the
// enclosing program refuse is strictly worse than no stamp at all, since the
// island it replaces is the behaviour the differential already validates.
//
// dynScopeNames is the channel that showed this. A stamped body whose free
// word cannot resolve locally takes dynScopeRescue, which registers the name;
// Finalize then installs an OpBindDynScope twin in every BINDING unit, so the
// enclosing program's own `def` of that name must lower a dynamic bind it may
// have no promoted value for. Measured on this exact source, which went from
// compiling to refusing "dynamic-scope def `files` of unpromoted computed
// value" the moment mount handlers started stamping.
//
// The rule: snapshot the map, and DECLINE a stamp that added to it (a live
// stamped unit genuinely reads through OpLookupDynScope, so the name cannot
// be dropped out from under it — only the whole stamp can go).
const stampDynScopeSrc = "if true [import \"boru:io\"  def files (flex {})  " +
	"IO.mount {read: (p:Pathon => [files get `${p}`]) " +
	"write: ([p:Pathon data:Any] => [files set `${p}` data drop])}  " +
	"IO.write (make Pathon \"n/a.txt\") \"hello mounted\" drop  " +
	"IO.read (make Pathon \"n/a.txt\")] [0]"

func TestStampConstDynScopeDeclineKeepsEnclosingCompile(t *testing.T) {
	a := mustNew(t)
	disarm := a.ArmRuntimeStamping()
	defer disarm()

	prog, reason, _, err := a.CompileCheck(stampDynScopeSrc)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	if prog == nil {
		t.Fatalf("the enclosing program must still compile; refused with %q", reason)
	}

	// The decline is the mechanism under test, not an incidental outcome: at
	// least one handler must have been offered to the stamp and turned down.
	// (Were it silently stamping, the assertion above would pass for the wrong
	// reason and the refusal would return the next time the body changed.)
	declined := false
	for _, ev := range a.StampReport() {
		if !ev.Stamped {
			declined = true
		}
	}
	if !declined {
		t.Errorf("expected a declined stamp attempt; report = %+v", a.StampReport())
	}

	// And the decline costs nothing observable: both engines agree.
	gotC, compiled, errC, gotI, errI := runBothEngines(t, stampDynScopeSrc)
	if !compiled {
		t.Errorf("expected the compiled engine to run the program")
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("engine divergence: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
	}
	if !strings.Contains(fmt.Sprint(gotI), "hello mounted") {
		t.Errorf("mount round-trip lost: %v", gotI)
	}
}
