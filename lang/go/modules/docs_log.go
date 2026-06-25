package modules

func init() {
	registerDocs("aql:log", map[string]string{
		"trace":        "Emit a TRACE-level record (optional structured fields).",
		"debug":        "Emit a DEBUG-level record (optional structured fields).",
		"info":         "Emit an INFO-level record (optional structured fields).",
		"warn":         "Emit a WARN-level record (optional structured fields).",
		"error":        "Emit an ERROR-level record (optional structured fields).",
		"fatal":        "Emit a FATAL-level record (optional structured fields).",
		"log":          "Emit a record at a computed level: log level message [fields].",
		"set-level":    "Set the global minimum severity; lower records are dropped.",
		"get-level":    "The current global severity threshold, as a level atom.",
		"enabled":      "Would a record at the given level be emitted?",
		"set-format":   "Set the console render format: text or json.",
		"get-format":   "The current console render format, as an atom.",
		"add-sink":     "Attach a registered sink (console, memory, null) to the pipeline.",
		"remove-sink":  "Detach a sink from the pipeline.",
		"sinks":        "List the names of the currently attached sinks.",
		"dump":         "Return the records captured by the memory sink.",
		"clear":        "Empty the memory sink's capture buffer.",
		"logger":       "Create a named logger instance: logger NAME.",
		"with":         "Create a named logger with default fields: with NAME FIELDS.",
		"register":     "Install an AQL function as a sink: register FN NAME MIN-LEVEL.",
		"span":         "Start a span and return its handle: span NAME [ATTRS].",
		"with-span":    "Run a body inside a span, ending it on exit: with-span NAME BODY.",
		"end-span":     "End a started span given its handle: end-span SPAN.",
		"current-span": "The active span handle, or None.",
		"traces":       "Return the ended spans captured by the memory sink.",
	})
}
