package eng

import "testing"

func TestFnCompareProbeQ(t *testing.T) {
	f := NewFunction(FnDefInfo{Name: "f"})
	f.Quoted = true
	f2 := NewFunction(FnDefInfo{Name: "f"})
	f2.Quoted = true
	n, vf, err := compareValuesClassified(f, f2)
	t.Logf("quoted f vs f2: n=%d viaFamily=%v err=%v", n, vf, err)
	sigs := []Signature{{Args: []*Type{TInteger}}}
	a := NewFunction(FnDefInfo{Name: "f", Signatures: sigs})
	b := NewFunction(FnDefInfo{Name: "f", Signatures: sigs})
	n, vf, err = compareValuesClassified(a, b)
	t.Logf("sig f vs f: n=%d viaFamily=%v err=%v render=%q vs %q", n, vf, err, a.String(), b.String())
}
