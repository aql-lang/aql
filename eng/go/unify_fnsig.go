package eng

// unifyFnUndefShape handles unification when at least one side is a
// FnUndef (structural function-type constraint). The other side must
// be a function value (FnDef or Function) whose signatures cover the
// FnUndef pattern.
//
// The structural-subtyping rule itself (contravariant params,
// covariant returns, Pattern compatibility) lives in fnsig.go;
// this file only owns the kernel-side dispatch.
func unifyFnUndefShape(a Value, sa ValueShape, b Value, sb ValueShape) (Value, bool) {
	// Canonicalize: undef on the left, fn on the right.
	var undef, fn Value
	var fnShape ValueShape
	if sa == ShapeFnUndef {
		undef, fn = a, b
		fnShape = sb
	} else {
		undef, fn = b, a
		fnShape = sa
	}
	if fnShape != ShapeFnDef && fnShape != ShapeFunction {
		return Value{}, false
	}
	if FnUndefMatchesFnDef(undef, fn) {
		return fn, true
	}
	return Value{}, false
}
