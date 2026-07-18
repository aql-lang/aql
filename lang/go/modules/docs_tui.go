package modules

func init() {
	registerDocs("aql:tui", map[string]string{
		"open":           "Take the terminal over (raw mode + alt-screen) via the registered backend, returning a Terminal handle.",
		"close":          "Restore the terminal and release the handle; idempotent.",
		"dims":           "The terminal's current dimensions, as {cols rows}.",
		"read-event":     "The next decoded input event as a tagged map; with {within: ms}, None on deadline.",
		"deliver-events": "Deliver decoded input events to a process mailbox instead of pulling: the Tier-1 active mode (one delivery per terminal; read-event refuses while it owns the stream).",
		"print-at":       "Write styled text into the offscreen grid at cell (x, y), clipping at the edges.",
		"clear":          "Clear the offscreen grid.",
		"show":           "Present the offscreen grid to the terminal (the backend diffs).",
		"title":          "Set the terminal window title.",
		"bell":           "Ring the terminal bell.",
		"run":            "Run an app map {init update view}: events fold through update, the pure view renders, run blocks until quit and returns the final state.",
		"serve":          "Serve an app map to remote viewers over {tcp token viewers reattach}: trees down, events up (attach with `aql attach`); up to viewers concurrent viewers (default 1), reattach keeps the app alive across viewer loss; returns the final state.",
		"quit":           "Wrap the final state in the quit marker an update returns to end the app.",
		"text":           "Build a text widget: a styled leaf line (wrap: \"wrap\" for multi-line).",
		"rows":           "Build a vertical stack of child widgets.",
		"cols":           "Build a horizontal stack of child widgets.",
		"box":            "Build a bordered box around a child widget (title, border, pad).",
		"list-view":      "Build a scrolling list with a highlighted cursor row.",
		"table":          "Build a table from column specs and data rows, with a cursor row.",
		"input":          "Build a single-line input widget rendering state-owned text.",
		"viewport":       "Build a scroll window over a child widget at an offset.",
		"spacer":         "Build a flexible filler widget.",
		"style":          "Merge a style-override map over a base style map.",
		"edit":           "Fold one key event into an input widget: the standard line-editing moves.",
		"focusable":      "The id-carrying widgets of a tree, in document order.",
		"colorize":       "Wrap text in a style map's ANSI sequence plus a reset; {profile: \"256\"|\"16\"|\"none\"} degrades like the renderer. Pure — no terminal needed.",
		"strip-ansi":     "Remove ANSI escape sequences from a string (round-trips colorize).",
		"text-width":     "The display width of text in terminal cells: escapes stripped, wide runes 2, combining marks 0.",
	})
}
