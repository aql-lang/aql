package modules

func init() {
	registerDocs("aql:tui", map[string]string{
		"open":       "Take the terminal over (raw mode + alt-screen) via the registered backend, returning a Terminal handle.",
		"close":      "Restore the terminal and release the handle; idempotent.",
		"dims":       "The terminal's current dimensions, as {cols rows}.",
		"read-event": "The next decoded input event as a tagged map; with {within: ms}, None on deadline.",
		"print-at":   "Write styled text into the offscreen grid at cell (x, y), clipping at the edges.",
		"clear":      "Clear the offscreen grid.",
		"show":       "Present the offscreen grid to the terminal (the backend diffs).",
		"title":      "Set the terminal window title.",
		"bell":       "Ring the terminal bell.",
	})
}
