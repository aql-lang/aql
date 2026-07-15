package eng

import (
	"errors"
	"strings"
	"testing"
)

// The VM-side word-policy gate (plan Phase 10): every NAMED compiled
// dispatch — CALL_NATIVE, CALL_USER/TAIL_CALL_USER, CALL_NATIVE_POLY,
// CALL_USER_POLY, CALL_DYN_METHOD — consults the SAME WordChecker the
// interpreter's policyGateWord runs, so a denied word raises the identical
// error on either engine. Programs are hand-built (the seam7 pattern):
// production compiles still refuse policy-gated registries, so the gate is
// reached by installing the checker AFTER construction.

var errZzDenied = errors.New("zz-policy: word denied")

// denyChecker denies exactly one word.
type denyChecker struct{ word string }

func (d denyChecker) CheckWord(name string) error {
	if name == d.word {
		return errZzDenied
	}
	return nil
}

func gateReg(t *testing.T, deny string) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	if err := r.Capabilities.Set(CapPolicy, denyChecker{word: deny}); err != nil {
		t.Fatalf("install checker: %v", err)
	}
	return r
}

func TestVMWordPolicyGateArms(t *testing.T) {
	sig := Signature{Args: []*Type{TInteger}, Returns: []*Type{TInteger}, BarrierPos: -1,
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0]}, nil
		})}
	one := NewInteger(1)

	cases := []struct {
		name string
		p    *Program
	}{
		{"call-native", &Program{
			Code:   []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpCallNative, Arg: 0}},
			Consts: []Value{one}, Sigs: []SigRef{{Word: "zz-gated", Sig: &sig}},
		}},
		{"call-user", &Program{
			Code: []Instr{{Op: OpCallUser, Arg: 0}},
			Fns:  []CompiledFn{{Name: "zz-gated", NParams: 0}},
		}},
		{"call-native-poly", &Program{
			Code:     []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpCallNativePoly, Arg: 0}},
			Consts:   []Value{one},
			PolyRefs: []PolyRef{{Word: "zz-gated", Arity: 1}},
		}},
		{"call-user-poly", &Program{
			Code:      []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpCallUserPoly, Arg: 0}},
			Consts:    []Value{one},
			UserPolys: []UserPolyRef{{Word: "zz-gated", Arity: 1}},
		}},
		{"call-dyn-method", &Program{
			Code:       []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpCallDynMethod, Arg: 0}},
			Consts:     []Value{one},
			DynMethods: []DynMethodSpec{{Word: "zz-gated", NArgs: 1, NOut: 1}},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := RunProgram(c.p, gateReg(t, "zz-gated"))
			if err == nil || !strings.Contains(err.Error(), "zz-policy: word denied") {
				t.Fatalf("denied word must raise the checker's error, got %v", err)
			}
			// The negative twin: the same program under a checker denying a
			// DIFFERENT word passes the gate (it may then fail later for
			// structural reasons — the gate must be what stopped it above).
			_, err2 := RunProgram(c.p, gateReg(t, "zz-other"))
			if err2 != nil && strings.Contains(err2.Error(), "zz-policy") {
				t.Fatalf("non-denied word must pass the gate, got %v", err2)
			}
		})
	}

	// Internal markers are exempt, exactly as in policyGateWord.
	marker := &Program{
		Code:   []Instr{{Op: OpPushConst, Arg: 0}, {Op: OpCallNative, Arg: 0}},
		Consts: []Value{one}, Sigs: []SigRef{{Word: "__zz", Sig: &sig}},
	}
	if _, err := RunProgram(marker, gateReg(t, "__zz")); err != nil && strings.Contains(err.Error(), "zz-policy") {
		t.Fatalf("internal marker must bypass the gate, got %v", err)
	}
}
