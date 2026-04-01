package engine

import "fmt"

// registerVar registers the "var" word for scoped variable definitions.
//
// var takes one list argument. The first element is a list of variable
// declarations. The remaining elements form the body. After the body,
// all variables are automatically undefined.
//
// Each declaration is either:
//   - A bare word: takes its value from the stack (def name end)
//   - A list [name value]: defines with the given value (def name value end)
//
// Example: var [[x] x mul x]  means  def x end x mul x undef x
// Example: var [[[x 2] y] x add y]  means  def x 2 end def y end x add y undef y undef x
func registerVar(r *Registry) {
	varHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		list := args[0]
		if !list.VType.Equal(TList) {
			return nil, fmt.Errorf("var: argument must be a list")
		}
		if list.Data == nil {
			return nil, fmt.Errorf("var: argument must be a concrete list, got type literal")
		}
		elems := list.AsList()
		if elems.Len() == 0 {
			return nil, fmt.Errorf("var: empty list")
		}

		// First element must be a list of variable declarations.
		declVal := elems.Get(0)
		if !declVal.VType.Equal(TList) || declVal.Data == nil {
			return nil, fmt.Errorf("var: first element must be a list of variable declarations")
		}
		decls := declVal.AsList()
		body := elems.Slice()[1:]

		var result []Value
		var varNames []string

		for _, decl := range decls.Slice() {
			switch {
			case decl.IsWord():
				// Bare word: take value from stack.
				name := decl.AsWord().Name
				varNames = append(varNames, name)
				result = append(result, NewWord("def"), NewWord(name), NewWord("end"))

			case decl.VType.Equal(TList) && decl.Data != nil:
				// List [name value...]: define with given value.
				declElems := decl.AsList()
				if declElems.Len() < 2 {
					return nil, fmt.Errorf("var: declaration list must have name and value")
				}
				var name string
				if declElems.Get(0).IsWord() {
					name = declElems.Get(0).AsWord().Name
				} else if declElems.Get(0).VType.Matches(TString) {
					name = declElems.Get(0).AsString()
				} else {
					return nil, fmt.Errorf("var: declaration name must be a word or string")
				}
				varNames = append(varNames, name)
				result = append(result, NewWord("def"), NewWord(name))
				result = append(result, declElems.Slice()[1:]...)
				result = append(result, NewWord("end"))

			case decl.VType.Matches(TString):
				// String name: take value from stack.
				name := decl.AsString()
				varNames = append(varNames, name)
				result = append(result, NewWord("def"), NewWord(name), NewWord("end"))

			default:
				return nil, fmt.Errorf("var: invalid declaration: %s", decl.String())
			}
		}

		// Append body.
		result = append(result, body...)

		// Append undefs in reverse order.
		for i := len(varNames) - 1; i >= 0; i-- {
			result = append(result, NewWord("undef"), NewWord(varNames[i]))
		}

		return result, nil
	}

	r.Register("var", Signature{
		Args:    []Type{TList},
		Handler: varHandler,
	})
}
