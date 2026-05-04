package engine

import "fmt"

// guard is the predicate-body workhorse: take a Boolean condition and
// a value, return the value if the condition is true, otherwise None.
// Designed for the predicate-type idiom — every predicate body is
// "if cond [val] [None]", which guard reduces to "cond guard val".
//
// Source form: `<cond expression> guard <value>`. The condition is
// left on the stack by the preceding expression (typically an `and`
// chain of type/range checks) and the value is the forward arg.
//
// Sig is [Any, Boolean] in mirror-pattern order — sig[0]=val
// (forward), sig[1]=cond (stack). BarrierPos=1 keeps `guard` from
// greedily consuming a chained second forward arg, matching the
// non-greedy precedence of `or`/`and`/`is`.
//
// Compare:
//
//	type Bbd fn [x:Any Any [
//	    if ((x is String) and (x gte "b") and (x lte "d")) [x] [None]
//	]]
//
//	type Bbd fn [x:Any Any [
//	    (x is String) and (x gte "b") and (x lte "d") guard x
//	]]
//
// Same semantics, the second form drops the if/else nest.
func RegisterGuard(r *Registry) {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		val := args[0]
		cond, err := args[1].AsBoolean()
		if err != nil {
			return nil, fmt.Errorf("guard: condition must be Boolean, got %s", args[1].VType.String())
		}
		if cond {
			return []Value{val}, nil
		}
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              "guard",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:       []Type{TAny, TBoolean},
			BarrierPos: 1,
			Handler:    handler,
			Returns:    []Type{TAny},
		}},
	})
}
