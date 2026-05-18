package engine

import "github.com/aql-lang/aql/eng/go"

// printNatives covers the diagnostic output words. `print` writes its
// argument's formatted representation to the registry's Output writer
// followed by a newline; `printstr` does the same without the newline.
//
// Algorithms (FormatForPrint and the rest of print.go) live in eng;
// this file owns the word names and dispatch wiring.
var printNatives = []NativeFunc{
	{
		Name:        "print",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: eng.PrintHandler,
			Returns: []*Type{},
		}},
	},
	{
		Name:        "printstr",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: eng.PrintstrHandler,
			Returns: []*Type{},
		}},
	},
}
