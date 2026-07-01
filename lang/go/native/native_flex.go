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

		Signatures: []Signature{{
			Args:      []*Type{TNode},
			Impl:      Go(flexHandler),
			Returns:   []*Type{TNode},
			ReturnsFn: flexReturns, BarrierPos: -1,
		}},
	},
	{
		Name: "node",

		Signatures: []Signature{{
			Args:      []*Type{TNode},
			Impl:      Go(nodeHandler),
			Returns:   []*Type{TNode},
			ReturnsFn: nodeReturns, BarrierPos: -1,
		}},
	},
	{
		Name: "append",

		Signatures: []Signature{
			// List source: concatenate its elements. More specific
			// than the Any sig, so it wins whenever the argument is a
			// list (including another FlexList, which conforms to List).
			{
				Args:    []*Type{TList, TFlexList},
				Impl:    Go(appendListHandler),
				Returns: []*Type{TFlexList}, BarrierPos: -1,
			},
			// Any other value: append as a single element.
			{
				Args:    []*Type{TAny, TFlexList},
				Impl:    Go(appendElemHandler),
				Returns: []*Type{TFlexList}, BarrierPos: -1,
			},
			// FlexXml: append child nodes (elements or text) in place.
			// A List splices its elements; any other value is one child.
			{
				Args:    []*Type{TList, TFlexXml},
				Impl:    Go(appendXmlListHandler),
				Returns: []*Type{TFlexXml}, BarrierPos: -1,
			},
			{
				Args:    []*Type{TAny, TFlexXml},
				Impl:    Go(appendXmlChildHandler),
				Returns: []*Type{TFlexXml}, BarrierPos: -1,
			},
		},
	},
}

// flexReturns models the precise FLEX subtype `flex` produces from the input's
// node family (mirroring FlexDeepCopy: Map→FlexMap, List→FlexList, Xml→FlexXml).
// The static `Returns: [TNode]` was the supertype, so a FlexMap/FlexList
// consumer (`set`/`append`/`push`/`sort`/`each` over the result) never matched
// its `Flex*` sig and failed no_signature (flex.tsv). An unknown node shape
// (a dynamic Any / bare Node receiver) stays a DYNAMIC Node so a Flex* slot
// still matches optimistically rather than failing on a strict supertype.
func flexReturns(args []Value, _ *Registry) []Value {
	if len(args) != 1 || args[0].Parent == nil {
		return []Value{NewDynamicCarrier(TNode)}
	}
	switch p := args[0].Parent; {
	case p.ConformsTo(TMap):
		return []Value{NewCarrier(TFlexMap)}
	case p.ConformsTo(TList):
		return []Value{NewCarrier(TFlexList)}
	case p.ConformsTo(TXml):
		return []Value{NewCarrier(TFlexXml)}
	}
	return []Value{NewDynamicCarrier(TNode)}
}

// nodeReturns is the inverse: `node` deep-copies a flex tree back to a PLAIN
// Node, so the result's family is the input's (FlexMap→Map, FlexList→List,
// FlexXml→Xml). Same dynamic-Node fallback for an unknown shape.
func nodeReturns(args []Value, _ *Registry) []Value {
	if len(args) != 1 || args[0].Parent == nil {
		return []Value{NewDynamicCarrier(TNode)}
	}
	switch p := args[0].Parent; {
	case p.ConformsTo(TMap):
		return []Value{NewCarrier(TMap)}
	case p.ConformsTo(TList):
		return []Value{NewCarrier(TList)}
	case p.ConformsTo(TXml):
		return []Value{NewCarrier(TXml)}
	}
	return []Value{NewDynamicCarrier(TNode)}
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

func appendXmlChildHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexXml(args[1])
	if err != nil {
		return nil, r.AqlError("append_error", "append: expected a FlexXml, got "+args[1].Parent.String(), "append")
	}
	// A flex tree stays entirely mutable: a plain Node/Xml child is
	// deep-flexed on the way in; flex handles share.
	child, aerr := eng.AdoptIntoFlex(args[0])
	if aerr != nil {
		return nil, r.AqlError("append_error", aerr.Error(), "append")
	}
	fd.Cren = append(fd.Cren, child)
	return []Value{args[1]}, nil
}

func appendXmlListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexXml(args[1])
	if err != nil {
		return nil, r.AqlError("append_error", "append: expected a FlexXml, got "+args[1].Parent.String(), "append")
	}
	src, err := RequireConcreteList(args[0], "append")
	if err != nil {
		return nil, r.AqlError("append_error", err.Error(), "append")
	}
	elems := src.Slice()
	for _, el := range elems {
		child, aerr := eng.AdoptIntoFlex(el)
		if aerr != nil {
			return nil, r.AqlError("append_error", aerr.Error(), "append")
		}
		fd.Cren = append(fd.Cren, child)
	}
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
