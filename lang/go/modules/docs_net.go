package modules

func init() {
	registerDocs("aql:net", map[string]string{
		"direct":  "Build and send a request against an API descriptor.",
		"fetch":   "Perform an HTTP request.",
		"prepare": "Build a request from an API descriptor without sending.",

		// Tier-1 low-level sockets (net_socket.go).
		"listen":      "Bind a listening socket ({tcp: port}); with a codec and a Service, expose the service on the wire.",
		"accept":      "Block for the next connection on a Listener; returns a Socket. {within: ms} bounds the wait.",
		"connect-raw": "Dial a raw connection ({tcp: \"host:port\"}); returns a Socket.",
		"serve-raw":   "Accept-loop sugar: one recovered handler invocation per connection, each on its own fork.",
		"recv":        "Read up to n bytes from a Socket (n=0: whatever arrives); {within: ms} deadline.",
		"recv-bytes":  "Read exactly n bytes from a Socket; {within: ms} deadline.",
		"recv-until":  "Read through a Bytes delimiter (stripped); {within: ms} deadline.",
		"send-bytes":  "Write all the given Bytes to a Socket.",
		"shutdown":    "Half-close a TCP Socket (\"read\", \"write\" or \"both\").",
		"close":       "Close a Socket, Listener, or connected Endpoint.",
		"peer":        "The remote {host port} of a Socket.",
		"addr":        "The bound {host port} of a Listener.",

		// Tier-2 codecs + endpoints (net_codec.go).
		"connect": "Dial an Endpoint over a codec: a Service whose call/send reach the remote peer.",
	})
}
