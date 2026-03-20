package engine

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine/help"
)

func registerHelp(r *Registry) {
	// help: [] -> [] (print self-help)
	selfHandler := func(args []Value) ([]Value, error) {
		fmt.Fprintln(r.Output, "help — Show help for an AQL word.")
		fmt.Fprintln(r.Output, "")
		fmt.Fprintln(r.Output, "Usage:")
		fmt.Fprintln(r.Output, "  help              Show this message.")
		fmt.Fprintln(r.Output, "  <word> help       Show help for a word (e.g. add help).")
		fmt.Fprintln(r.Output, "  \"<name>\" help     Show help by string name (e.g. \"concat\" help).")
		return nil, nil
	}

	// help: [atom] -> [] or [string] -> []
	wordHandler := func(args []Value) ([]Value, error) {
		name := valToString(args[0])
		entry := help.Lookup(name)
		if entry == nil {
			fmt.Fprintf(r.Output, "help: no help available for %q\n", name)
			return nil, nil
		}
		fmt.Fprint(r.Output, help.Format(entry))
		return nil, nil
	}

	// help: [word] -> [] (captures registered words like add, concat)
	wordRefHandler := func(args []Value) ([]Value, error) {
		name := args[0].AsWord().Name
		entry := help.Lookup(name)
		if entry == nil {
			fmt.Fprintf(r.Output, "help: no help available for %q\n", name)
			return nil, nil
		}
		fmt.Fprint(r.Output, help.Format(entry))
		return nil, nil
	}

	r.Register("help",
		Signature{Args: []Type{TWord}, Handler: wordRefHandler},
		Signature{Args: []Type{TString}, Handler: wordHandler},
		Signature{Args: []Type{TAtom}, Handler: wordHandler},
		Signature{Args: []Type{}, Handler: selfHandler},
	)
}

