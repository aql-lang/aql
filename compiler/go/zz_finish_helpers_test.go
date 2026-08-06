package compiler

import core "github.com/boru-lang/boru/core/go"

func strictDisjunct(alts ...core.Value) core.Value {
	d := core.NewDisjunct(alts)
	d.Carrier = true
	return d
}
