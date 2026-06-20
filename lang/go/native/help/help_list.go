package help

func init() {
	register(&Entry{
		Word:        "iota",
		Summary:     "Generate [0, 1, …, n-1] — the first n non-negative integers.",
		Description: "Produces an integer list counting up from 0 to n-1. The canonical way to make a sequence to map, fold, or scan over.",
		Examples:    []string{`iota 4   ;# => [0 1 2 3]`},
	})

	register(&Entry{
		Word:    "range",
		Summary: "A list of integers from start to end (exclusive), with an optional step.",
		Description: "`range start end` counts start, start+1, …, end-1; `range start end step` strides by step. " +
			"The end is exclusive.",
		Examples: []string{
			`range 2 6      ;# => [2 3 4 5]`,
			`range 1 10 2   ;# => [1 3 5 7 9]`,
		},
	})

	register(&Entry{
		Word:        "reverse",
		Summary:     "Reverse the order of a list's elements.",
		Description: "Returns a new list with the elements in the opposite order.",
		Examples:    []string{`reverse [1 2 3]   ;# => [3 2 1]`},
	})

	register(&Entry{
		Word:    "sort",
		Summary: "Sort a list (or a map's entries) into ascending order.",
		Description: "Orders by AQL's total value order — numeric, not lexical, for numbers. On a map, sorts the " +
			"entries by key.",
		Examples: []string{`sort [3 1 2]   ;# => [1 2 3]`},
	})

	register(&Entry{
		Word:    "flatten",
		Summary: "Concatenate nested lists into one, to a given depth.",
		Description: "`flatten xs` splices one level of nesting; `flatten depth xs` flattens that many levels. " +
			"Deeper structure is left intact.",
		Examples: []string{
			`flatten [[1 2] [3]]        ;# => [1 2 3]`,
			`flatten 1 [[1 [2]] [3]]    ;# => [1 [2] 3]`,
		},
	})

	register(&Entry{
		Word:    "slice",
		Summary: "Extract a subsequence of a list or string by start/end indices.",
		Description: "`slice start end xs` returns the elements (or characters) from start up to end (exclusive). " +
			"With only a start it runs to the end; with neither it copies.",
		Examples: []string{`slice 1 4 "hello"   ;# => 'ell'`},
	})

	register(&Entry{
		Word:        "take",
		Summary:     "Keep the first n elements of a list.",
		Description: "Returns the leading n elements (fewer if the list is shorter). The complement of shed.",
		Examples:    []string{`take 2 [1 2 3 4]   ;# => [1 2]`},
	})

	register(&Entry{
		Word:        "shed",
		Summary:     "Drop the first n elements of a list (the complement of take).",
		Description: "Returns the list with its leading n elements removed.",
		Examples:    []string{`shed 2 [1 2 3 4]   ;# => [3 4]`},
	})

	register(&Entry{
		Word:    "append",
		Summary: "Append a value (or a list's elements) onto a flex list, in place.",
		Description: "Grows a mutable FlexList (`flex […]`) — the receiver is extended. For an immutable list use " +
			"push, which returns a new list instead of mutating.",
	})

	register(&Entry{
		Word:        "push",
		Summary:     "Add a value to the end of a list, returning the new list.",
		Description: "`push v xs` yields xs with v appended. Pairs with pop (which removes from the end).",
		Examples:    []string{`push 3 [1 2]   ;# => [1 2 3]`},
	})

	register(&Entry{
		Word:    "pop",
		Summary: "Remove the last element of a list, returning the list and the element.",
		Description: "Leaves two values on the stack: the shortened list (deeper) and the removed last element " +
			"(on top). The inverse of push.",
		Examples: []string{`pop [1 2 3]   ;# => [1 2] 3`},
	})

	register(&Entry{
		Word:    "shift",
		Summary: "Remove the first element of a list, returning the list and the element.",
		Description: "Leaves the shortened list (deeper) and the removed first element (on top). The inverse of " +
			"unshift.",
		Examples: []string{`shift [1 2 3]   ;# => [2 3] 1`},
	})

	register(&Entry{
		Word:        "unshift",
		Summary:     "Add a value to the front of a list, returning the new list.",
		Description: "`unshift v xs` yields xs with v prepended. Pairs with shift (which removes from the front).",
		Examples:    []string{`unshift 0 [1 2]   ;# => [0 1 2]`},
	})

	register(&Entry{
		Word:        "size",
		Summary:     "The number of elements, characters, or entries in a value.",
		Description: "Length of a list, string, or map.",
		Examples: []string{
			`size [1 2 3]   ;# => 3`,
			`size "abc"     ;# => 3`,
		},
	})

	register(&Entry{
		Word:    "enum",
		Summary: "Build an enumeration type from a list of atoms.",
		Description: "`enum [a/q b/q c/q]` makes the disjoint type `a|b|c` whose only inhabitants are those atoms — " +
			"a closed set of named constants.",
		Examples: []string{`enum [a/q b/q c/q]   ;# => a|b|c`},
	})

	register(&Entry{
		Word:    "node",
		Summary: "Wrap a value as a flex Node that adopts its children.",
		Description: "Marks a map or list as a flex Node so nested writes stay inside the flex tree rather than " +
			"copying. See flex.",
	})

	register(&Entry{
		Word:    "flex",
		Summary: "Make a list or map a mutable flex tree (FlexList / FlexMap).",
		Description: "A flex node supports in-place growth (append, push) and adoption of nested nodes, unlike the " +
			"immutable List/Map defaults.",
		Examples: []string{`flex [1 2 3]   ;# => [1 2 3]  (now mutable)`},
	})

	register(&Entry{
		Word:    "xml-elem",
		Summary: "The element children of an Xml node (text nodes dropped).",
		Description: "Returns the child elements of a Node/Xml as a List — the DOM `children` view. Text children " +
			"are dropped; use the `cren` field (e.g. x.cren) for all children including text.",
		Examples: []string{`xml-elem <a>t<b/><c/></a>   ;# => [<b/> <c/>]`},
	})

	register(&Entry{
		Word:    "xml-text",
		Summary: "The concatenated text content of an Xml subtree.",
		Description: "Walks a Node/Xml element and concatenates every text node in document order — the DOM " +
			"`textContent` view.",
		Examples: []string{`xml-text <a>x<b>y</b>z</a>   ;# => 'xyz'`},
	})

	register(&Entry{
		Word:    "xml-attr",
		Summary: "Read one attribute value of an Xml element, or none.",
		Description: "Looks up an attribute by name on a Node/Xml element, returning its String value or none when " +
			"the attribute is absent. The full attribute map is the `attr` field (x.attr).",
		Examples: []string{`xml-attr 'x' <a x="1"/>   ;# => '1'`},
	})
}
