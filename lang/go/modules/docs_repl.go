package modules

func init() {
	registerDocs("aql:repl", map[string]string{
		"serve":   "Start a line-protocol REPL server on {port: N}; returns the Listener.",
		"connect": "Dial a REPL server (\"host:port\"); returns an Endpoint.",
		"eval":    "Evaluate one source line against a connected REPL; returns the reply text.",
		"close":   "Close a REPL Listener or Endpoint.",
	})
}
