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
		"counter":      "Create a counter instrument: counter NAME (then c.add N).",
		"gauge":        "Create a gauge instrument: gauge NAME (then g.record V).",
		"histogram":    "Create a histogram instrument: histogram NAME (then h.record V).",
		"measurements": "Return the measurements captured by the memory sink.",
	})

	// Levels and sinks are ATOMS (info/q, console/q), which the [Atom]
	// signature names but does not enumerate — and the default sink set
	// matters as much as the API. Results are from verified
	// lang/spec/module-log.tsv rows.
	registerExamples("aql:log", map[string][]string{
		"info":  {`Log.info "started"                               ;# levels: trace debug info warn error fatal`},
		"warn":  {`Log.warn "retrying" {attempt: 2}                 ;# the second operand is structured context`},
		"error": {`Log.error "failed" {err: e.message}`},
		"set-level": {
			`Log.set-level warn/q`,
			`Log.get-level                                    ;# warn/q — an Atom, not a String`,
			`Log.enabled debug/q                              ;# false — cheap to ask before building a message`,
		},
		"get-level":  {`Log.get-level                                    ;# info/q by default`},
		"enabled":    {`Log.enabled debug/q                              ;# false at the default level`},
		"set-format": {`Log.set-format json/q                            ;# text/q by default`},
		"sinks":      {`Log.sinks                                        ;# [console/q] by default`},
		"add-sink": {
			`Log.add-sink memory/q`,
			`Log.remove-sink console/q                        ;# capture instead of printing`,
			`Log.info "a"`,
			`Log.dump size                                    ;# 1 — the memory sink's records`,
		},
		"dump": {`Log.dump                                         ;# the memory sink's records, as data`},
		"span": {
			`Log.span "request" [                             ;# times the body and nests inside any open span`,
			`  Log.info "handling"`,
			`]`,
		},
		"with":      {`Log.with {request-id: rid} [ Log.info "start" ]  ;# context added to every record in the body`},
		"counter":   {`Log.counter "requests" 1                         ;# a metric, read back with Log.measurements`},
		"gauge":     {`Log.gauge "queue-depth" 12`},
		"histogram": {`Log.histogram "latency-ms" 37`},
		"logger":    {`def l (Log.logger "billing")                     ;# a named logger; l.info "…" tags every record`},
	})
}
