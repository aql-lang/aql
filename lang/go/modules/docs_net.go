package modules

func init() {
	registerDocs("aql:net", map[string]string{
		"direct":  "Build and send a request against an API descriptor.",
		"fetch":   "Perform an HTTP request.",
		"prepare": "Build a request from an API descriptor without sending.",

		// Accessor overloads on the Fetch family (word extensions of the
		// core accessors), so a Response answers `.status` directly.
		"dot":  "Read a field of a Fetch Request/Response by literal name (word extension of core dot).",
		"get":  "Read a field of a Fetch Request/Response by computed key (word extension of core get).",
		"dotr": "Strict `dot` on a Fetch value: a missing field raises (word extension of core dotr).",
		"getr": "Strict `get` on a Fetch value: a missing field raises (word extension of core getr).",
		"has":  "Whether a Fetch Request/Response carries the named field (word extension of core has).",

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
		"peer-cert":   "The VERIFIED peer certificate of a TLS Socket as a Map, or None (plain socket, or no client certificate demanded).",
		"addr":        "The bound {host port} of a Listener.",

		// Tier-2 codecs + endpoints (net_codec.go).
		"connect": "Dial an Endpoint over a codec: a Service whose call/send reach the remote peer.",
	})

	// The generated positional permutations are actively unhelpful for
	// these words: `Net.fetch 'a' {a:1,b:2}` is well-formed and teaches
	// nothing, because everything that matters about a capability word
	// is the shape of its options map. See docs.go.
	registerExamples("aql:net", map[string][]string{
		"fetch": {
			`Net.fetch "https://api.example.com/v1/orders"           ;# GET, returns a Response`,
			`(Net.fetch "https://api.example.com/health").status     ;# 200`,
			`(Net.fetch "https://api.example.com/health").ok         ;# true for 2xx`,
			`Net.fetch "https://api.example.com/orders" {`,
			`  method: "POST"`,
			`  headers: {content-type: "application/json"}`,
			`  body: '{"sku":"A1","qty":2}'`,
			`  timeout: 5000                                         ;# ms`,
			`}`,
			`Net.fetch "https://internal.example.com/v1" {           ;# mutual TLS`,
			`  tls: {identity: acme/q}                               ;# a HOST-registered credential`,
			`}`,
			`Net.fetch "https://private.example.com" {tls: {ca: pem}} ;# a private CA, replacing the system pool`,
		},
		"prepare": {
			`def api {kind: "api" spec: openapi-doc}`,
			`def req (Net.prepare api {op: "listOrders" params: {limit: 10}})`,
			`req.url                                                 ;# inspect before sending`,
		},
		"direct": {
			`def api {kind: "api" spec: openapi-doc}`,
			`Net.direct api {op: "listOrders" params: {limit: 10}}    ;# prepare + send in one step`,
		},

		// Tier-1 sockets. Every example binds {tcp: 0} — an ephemeral
		// port — so it is safe to paste and cannot collide.
		"listen": {
			`def l (Net.listen {tcp: 0})                             ;# ephemeral port, all interfaces`,
			`def l (Net.listen {tcp: 8080  host: "127.0.0.1"})       ;# loopback only`,
			`def l (Net.listen {unix: "/tmp/app.sock"})`,
			`def l (Net.listen {tcp: 8443  tls: {                    ;# TLS termination`,
			`  identity: gw/q                                        ;# the certificate to PRESENT`,
			`  require-client: ca-bundle                             ;# demand + verify a client cert`,
			`}})`,
		},
		"accept": {
			`def c (Net.accept l)                                    ;# blocks for the next connection`,
			`def c (Net.accept l {within: 5000})                     ;# raises timeout after 5s`,
		},
		"connect-raw": {
			`def s (Net.connect-raw {tcp: "127.0.0.1:8080"})`,
			`def s (Net.connect-raw {tcp: "api.example.com:443"  tls: {}})`,
			`def s (Net.connect-raw {tcp: "db.internal:5432"  tls: {`,
			`  ca: pem                                               ;# verify against a private CA`,
			`  identity: acme/q                                      ;# and present our own credential`,
			`}})`,
			`def s (Net.connect-raw {tcp: "slow.example:80"  within: 2000})  ;# dial deadline, ms`,
		},
		"serve-raw": {
			`def nl (convert Bytes "\n")`,
			`def l (Net.serve-raw {tcp: 0} ([c:Any] => [               ;# one recovered actor per connection`,
			`  Net.send-bytes (Net.recv-until c nl {within: 5000}) c   ;# line echo`,
			`]))`,
		},
		"recv": {
			`Net.recv s 0 {within: 5000}                             ;# whatever has arrived, up to 4096 bytes`,
			`Net.recv s 64                                           ;# up to 64 bytes, blocking`,
		},
		"recv-bytes": {
			`Net.recv-bytes s 4 {within: 5000}                       ;# EXACTLY 4 bytes, or raise`,
			`convert String (Net.recv-bytes s 4)`,
		},
		"recv-until": {
			`Net.recv-until s (convert Bytes "\n") {within: 5000}     ;# a line, delimiter stripped`,
			`Net.recv-until s (convert Bytes "\r\n\r\n")              ;# an HTTP header block`,
		},
		"send-bytes": {
			`Net.send-bytes (convert Bytes "ping\n") s               ;# writes all of it`,
		},
		"shutdown": {
			`Net.shutdown s "write"                                  ;# half-close: peer sees EOF, we can still read`,
			`Net.shutdown s "both"`,
		},
		"close": {
			`Net.close s                                             ;# Socket`,
			`Net.close l                                             ;# Listener — stops the accept loop`,
		},
		"peer": {
			`(Net.peer c).host                                       ;# '127.0.0.1'`,
			`(Net.peer c).port                                       ;# 54321`,
		},
		"peer-cert": {
			`def who (Net.peer-cert c)                               ;# none unless require-client: demanded one`,
			`who.common-name                                         ;# 'billing.internal'`,
			`who.dns-names                                           ;# ['billing.internal']`,
			`if (who eq none) [ raise "unauthenticated" ] [ … ]       ;# authorize in AQL, not in the host`,
		},
		"addr": {
			`(Net.addr l).port                                       ;# the REAL port behind an ephemeral {tcp: 0}`,
		},
		"connect": {
			`def e (Net.connect {tcp: "127.0.0.1:9000"  codec: (length-prefixed [len:u32 payload:bytes(len)])})`,
			`e.call {op: "get"  key: "k"}                            ;# an Endpoint is a Service`,
		},
	})
}
