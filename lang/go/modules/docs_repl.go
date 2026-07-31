package modules

func init() {
	registerDocs("boru:repl", map[string]string{
		"serve":   "Start a line-protocol REPL server on {port: N}; returns the Listener.",
		"connect": "Dial a REPL server (\"host:port\"); returns an Endpoint.",
		"eval":    "Evaluate one source line against a connected REPL; returns the reply text.",
		"close":   "Close a REPL Listener or Endpoint.",
	})

	// A REPL server is only useful as a PAIR of ends, and the flow —
	// serve, connect, eval, close both — is what the four signatures
	// cannot show. Results are from verified lang/spec/module-repl.tsv
	// rows.
	registerExamples("boru:repl", map[string][]string{
		"serve": {
			`import "boru:net"`,
			`def l (Repl.serve {port: 0})                     ;# 0 = an ephemeral port`,
			`def a (Net.addr l)                               ;# read back the real one`,
			`;# … Repl.close l when done; the listener holds the port until then.`,
		},
		"connect": {
			`def e (Repl.connect "127.0.0.1:7777")            ;# an endpoint to a running Repl.serve`,
		},
		"eval": {
			`def r (Repl.eval e "1 add 2")                    ;# '3' — a STRING, always`,
			`(Repl.eval e "def x 21") drop`,
			`Repl.eval e "x mul 2"                            ;# '42' — the session keeps its bindings`,
			`;# An error comes back as text beginning "error:", not as a raise.`,
		},
		"close": {`Repl.close e                                     ;# closes an endpoint or a listener`},
	})
}
