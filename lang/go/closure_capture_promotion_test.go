package lang

import "testing"

// closure_capture_promotion_test.go pins NUR126: a RETURNED lambda's
// captured computed value must reach the closure push through a frame
// local. The promotion planner counts a residual closure's captures
// (appendResidualSeqs) — without that the capture stayed an opEvent, and
// pushOperand's const arm baked the event's SEQ as a const INDEX: the
// factory DROPped the computed value and captured an unrelated constant,
// so the lambda answered the CALLER'S OWN ARGUMENT (7 for the
// interpreter's 42) and the disassembler panicked on the out-of-range
// index.
func TestReturnedClosureCapturePromotion(t *testing.T) {
	rows := []struct{ src, note string }{
		// the miscompile's own shape, with every value kind the capture can hold
		{`def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any][j]] )]]  def q (h {f: 5})  (q 1)`, "5 — the capture, not the argument"},
		{`def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any][j]] )]]  def q (h {f: "s"})  (q 1)`, "s"},
		{`def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any][j]] )]]  def q (h {f: [1 2]})  (q 1)`, "[1 2]"},
		{`def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any][j x add]] )]]  def q (h {f: 5})  (q 1)`, "6 — capture and param together"},
		// a computed capture from a param, and two of them
		{`def h fn [[n:Integer][Function][def j (n add 1)  ( fn [[x:Integer][Any][j]] )]]  def q (h 5)  (q 1)`, "6"},
		{`def h fn [[n:Integer][Function][def j (n add 1)  ( fn [[x:Integer][Integer][x add j]] )]]  def q (h 5)  (q 10)`, "16"},
		{`def h fn [[n:Integer][Function][def j (n add 1)  def k (n mul 2)  ( fn [[x:Integer][Integer][x add j add k]] )]]  def q (h 5)  (q 10)`, "26 — two computed captures"},
		// the param itself as the capture (no promotion needed), and two
		// closures from one factory (each keeps its own captured value)
		{`def h fn [[m:Map][Function][( fn [[x:Integer][Any][m]] )]]  def q (h {f: 1})  (q 1)`, "{f:1}"},
		{`def h fn [[n:Integer][Function][def j (n add 1)  ( fn [[x:Integer][Integer][x add j]] )]]  def q (h 5)  def r (h 1)  (r 10)`, "12 — a second instance keeps its own captured value"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.note, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}
