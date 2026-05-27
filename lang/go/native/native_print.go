package native

import "github.com/aql-lang/aql/eng/go"

// printNatives covers the diagnostic output words. `print` writes its
// argument's formatted representation to the registry's Output writer
// followed by a newline; `printstr` does the same without the newline.
//
// Algorithms (FormatForPrint and the rest of print.go) live in eng;
// this file owns the word names and dispatch wiring.
var printNatives = []NativeFunc{
	{
		Name: "print",

		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: eng.PrintHandler,
			Returns: []*Type{}, BarrierPos: -1,
		}},
	},
	{
		Name: "printstr",

		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: eng.PrintstrHandler,
			Returns: []*Type{}, BarrierPos: -1,
		}},
	},
}
