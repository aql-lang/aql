package repl

import (
	"fmt"
	"io"
	"sort"
	"strings"

	jsonic "github.com/jsonicjs/jsonic/go"

	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/native/help"
)

// MetaContext provides context to meta command handlers.
type MetaContext struct {
	Out      io.Writer        // output writer
	Registry *native.Registry // the engine registry
	Stack    []native.Value   // last evaluation result (the "stack")
}

// MetaHandler is the function signature for a meta command.
type MetaHandler func(args []any, ctx *MetaContext) error

// MetaCommand describes a registered meta command.
type MetaCommand struct {
	Name    string
	Summary string
	Handler MetaHandler
}

// MetaRegistry holds registered meta commands.
type MetaRegistry struct {
	commands map[string]*MetaCommand
}

// NewMetaRegistry creates a MetaRegistry with built-in commands.
func NewMetaRegistry() *MetaRegistry {
	mr := &MetaRegistry{
		commands: make(map[string]*MetaCommand),
	}
	mr.registerBuiltins()
	return mr
}

// Register adds a meta command.
func (mr *MetaRegistry) Register(cmd *MetaCommand) {
	mr.commands[cmd.Name] = cmd
}

// Lookup returns a meta command by name, or nil.
func (mr *MetaRegistry) Lookup(name string) *MetaCommand {
	return mr.commands[name]
}

// Names returns all registered command names sorted alphabetically.
func (mr *MetaRegistry) Names() []string {
	names := make([]string, 0, len(mr.commands))
	for k := range mr.commands {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// ParseAndRun checks if a line is a meta command (starts with /) and
// executes it. Returns true if the line was handled as a meta command.
func (mr *MetaRegistry) ParseAndRun(line string, ctx *MetaContext) (bool, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "/") {
		return false, nil
	}

	// Split into command name and arg string.
	rest := line[1:] // strip leading /
	name, argStr := splitCommand(rest)
	if name == "" {
		return true, fmt.Errorf("empty meta command")
	}

	cmd := mr.Lookup(name)
	if cmd == nil {
		return true, fmt.Errorf("unknown meta command: /%s", name)
	}

	args, err := parseMetaArgs(argStr)
	if err != nil {
		return true, fmt.Errorf("/%s: %w", name, err)
	}

	return true, cmd.Handler(args, ctx)
}

// splitCommand splits "name arg1 arg2" into ("name", "arg1 arg2").
func splitCommand(s string) (string, string) {
	s = strings.TrimSpace(s)
	idx := strings.IndexAny(s, " \t")
	if idx < 0 {
		return s, ""
	}
	return s[:idx], strings.TrimSpace(s[idx+1:])
}

// parseMetaArgs parses the argument string using a plain jsonic parser.
// Returns a slice of raw Go values (string, float64, bool, nil, etc.).
func parseMetaArgs(s string) ([]any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	j := jsonic.Make(jsonic.Options{})
	result, err := j.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("arg parse error: %w", err)
	}

	// jsonic returns a single value or a list for implicit top-level items.
	switch val := result.(type) {
	case []any:
		return val, nil
	default:
		return []any{val}, nil
	}
}

// registerBuiltins adds the built-in meta commands.
func (mr *MetaRegistry) registerBuiltins() {
	mr.Register(&MetaCommand{
		Name:    "help",
		Summary: "Show help for a word or list all commands",
		Handler: metaHelp(mr),
	})

	mr.Register(&MetaCommand{
		Name:    "stack",
		Summary: "Print the current stack entries",
		Handler: metaStack,
	})
}

// metaHelp returns the /help handler. It closes over the MetaRegistry
// so it can list available meta commands.
func metaHelp(mr *MetaRegistry) MetaHandler {
	return func(args []any, ctx *MetaContext) error {
		if len(args) == 0 {
			// List meta commands and general help.
			fmt.Fprintln(ctx.Out, "Meta commands (type /name to run):")
			for _, name := range mr.Names() {
				cmd := mr.Lookup(name)
				fmt.Fprintf(ctx.Out, "  /%-12s %s\n", name, cmd.Summary)
			}
			fmt.Fprintln(ctx.Out)
			fmt.Fprintln(ctx.Out, "Word help (type /help <word>):")
			words := help.Words()
			sort.Strings(words)
			for _, w := range words {
				e := help.Lookup(w)
				fmt.Fprintf(ctx.Out, "  %-12s %s\n", w, e.Summary)
			}
			return nil
		}

		// /help <word> — look up a specific word.
		name := fmt.Sprint(args[0])
		entry := help.Lookup(name)
		if entry == nil {
			fmt.Fprintf(ctx.Out, "help: no help available for %q\n", name)
			return nil
		}
		fmt.Fprint(ctx.Out, help.Format(entry))
		return nil
	}
}

// metaStack prints the current stack entries without consuming them.
// An optional integer argument limits output to the top n items.
func metaStack(args []any, ctx *MetaContext) error {
	if len(ctx.Stack) == 0 {
		fmt.Fprintln(ctx.Out, "(empty stack)")
		return nil
	}

	start := 0
	if len(args) > 0 {
		n, ok := toInt(args[0])
		if !ok {
			return fmt.Errorf("expected integer argument, got %v", args[0])
		}
		if n < 0 {
			return fmt.Errorf("argument must be non-negative, got %d", n)
		}
		if n < len(ctx.Stack) {
			start = len(ctx.Stack) - n
		}
	}

	for i := start; i < len(ctx.Stack); i++ {
		fmt.Fprintf(ctx.Out, "  [%d] %s\n", i, ctx.Stack[i].String())
	}
	return nil
}

// toInt converts a jsonic-parsed value to an int. Handles float64 (jsonic
// default for numbers) and int.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	case int:
		return n, true
	}
	return 0, false
}
