package native

import "github.com/aql-lang/aql/eng/go"

// flexNatives installs the flex-node words:
//
//	flex <node>        — deep mutable copy: Map→FlexMap, List→FlexList,
//	                     nested Nodes converted recursively. Sugar for
//	                     `make FlexMap m` / `make FlexList l`.
//	node <node>        — the inverse: deep immutable conversion.
//	                     Identity on a node that contains no flex parts.
//	append v  fl       — append one element to a FlexList in place.
//	append [..] fl     — concatenate a list's elements in place (the
//	                     List sig outranks the Any sig, so lists concat
//	                     by default; wrap to append a list AS an
//	                     element: `append [[1 2]] fl`).
//
// Mutating words return the flex node itself so calls chain:
// `append 1 fl` leaves fl on the stack for the next word. Values are
// stored AS GIVEN (by reference, like map values everywhere else) —
// containment is the job of the conversion words, not storage: flex
// an element first if it must be mutable inside the container.
//
// The algorithms (FlexDeepCopy, NodeDeepCopy, MakeNodeHandler) live
// in eng/go/core_flex.go; this file owns the word names and dispatch
// wiring. The in-place `set` sigs live in native_storage.go and the
// flex push/pop/unshift/shift sigs in natives.go (handlers in
// listops.go).
var flexNatives = []NativeFunc{
	{
		Name: "flex",

		Signatures: []NativeSig{{
			Args:    []*Type{TNode},
			Handler: flexHandler,
			Returns: []*Type{TNode}, BarrierPos: -1,
		}},
	},
	{
		Name: "node",

		Signatures: []NativeSig{{
			Args:    []*Type{TNode},
			Handler: nodeHandler,
			Returns: []*Type{TNode}, BarrierPos: -1,
		}},
	},
	{
		Name: "append",

		Signatures: []NativeSig{
			// List source: concatenate its elements. More specific
			// than the Any sig, so it wins whenever the argument is a
			// list (including another FlexList, which conforms to List).
			{
				Args:    []*Type{TList, TFlexList},
				Handler: appendListHandler,
				Returns: []*Type{TFlexList}, BarrierPos: -1,
			},
			// Any other value: append as a single element.
			{
				Args:    []*Type{TAny, TFlexList},
				Handler: appendElemHandler,
				Returns: []*Type{TFlexList}, BarrierPos: -1,
			},
		},
	},
}

func flexHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("flex_error", "flex: expected a concrete map or list, got "+args[0].String(), "flex")
	}
	out, err := eng.FlexDeepCopy(args[0])
	if err != nil {
		return nil, r.AqlError("flex_error", err.Error(), "flex")
	}
	return []Value{out}, nil
}

func nodeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.AqlError("node_error", "node: expected a concrete map or list, got "+args[0].String(), "node")
	}
	out, err := eng.NodeDeepCopy(args[0])
	if err != nil {
		return nil, r.AqlError("node_error", err.Error(), "node")
	}
	return []Value{out}, nil
}

func appendElemHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexList(args[1])
	if err != nil {
		return nil, r.AqlError("append_error", "append: expected a FlexList, got "+args[1].Parent.String(), "append")
	}
	// A flex tree stays ENTIRELY mutable: a plain Node element is deep-
	// flexed on the way in (eng.AdoptIntoFlex; flex handles share).
	elem, aerr := eng.AdoptIntoFlex(args[0])
	if aerr != nil {
		return nil, r.AqlError("append_error", aerr.Error(), "append")
	}
	fd.Elems = append(fd.Elems, elem)
	return []Value{args[1]}, nil
}

func appendListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexList(args[1])
	if err != nil {
		return nil, r.AqlError("append_error", "append: expected a FlexList, got "+args[1].Parent.String(), "append")
	}
	src, err := RequireConcreteList(args[0], "append")
	if err != nil {
		return nil, r.AqlError("append_error", err.Error(), "append")
	}
	// Slice() snapshots before the grow, so `append f f` (self-concat)
	// is well-defined. Each appended element is adopted so the tree
	// stays entirely mutable.
	elems := src.Slice()
	adopted := make([]Value, len(elems))
	for i, el := range elems {
		a, aerr := eng.AdoptIntoFlex(el)
		if aerr != nil {
			return nil, r.AqlError("append_error", aerr.Error(), "append")
		}
		adopted[i] = a
	}
	fd.Elems = append(fd.Elems, adopted...)
	return []Value{args[1]}, nil
}
