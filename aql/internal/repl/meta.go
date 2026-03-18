package repl

import (
	"fmt"
	"io"
	"sort"
	"strings"

	jsonic "github.com/jsonicjs/jsonic/go"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine/help"
)

// MetaContext provides context to meta command handlers.
type MetaContext struct {
	Out      io.Writer        // output writer
	Registry *engine.Registry // the engine registry
	Stack    []engine.Value   // last evaluation result (the "stack")
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
		fmt.Fprint(ctx.Out, formatHelp(entry))
		return nil
	}
}

// metaStack prints the current stack entries without consuming them.
func metaStack(args []any, ctx *MetaContext) error {
	if len(ctx.Stack) == 0 {
		fmt.Fprintln(ctx.Out, "(empty stack)")
		return nil
	}
	for i, v := range ctx.Stack {
		fmt.Fprintf(ctx.Out, "  [%d] %s\n", i, v.String())
	}
	return nil
}

// formatHelp formats a help entry for display. Mirrors the engine's
// formatHelp to keep output consistent.
func formatHelp(e *help.Entry) string {
	var b strings.Builder

	b.WriteString(e.Word)
	b.WriteString(" — ")
	b.WriteString(e.Summary)
	b.WriteByte('\n')

	b.WriteString("\nSignatures:\n")
	for _, sig := range e.Signatures {
		b.WriteString("  ")
		b.WriteString(sig)
		b.WriteByte('\n')
	}

	b.WriteString("\nDescription:\n  ")
	b.WriteString(e.Description)
	b.WriteByte('\n')

	if len(e.Examples) > 0 {
		b.WriteString("\nExamples:\n")
		for _, ex := range e.Examples {
			b.WriteString("  ")
			b.WriteString(ex)
			b.WriteByte('\n')
		}
	}

	if len(e.Notes) > 0 {
		b.WriteString("\nNotes:\n")
		for _, n := range e.Notes {
			b.WriteString("  - ")
			b.WriteString(n)
			b.WriteByte('\n')
		}
	}

	return b.String()
}
