package test

import "testing"

// TestStampDisjunctArgNarrowsToParam pins the last mini-s3 wall: a
// recovered overload's DISJUNCT-carrier result (`slice` over a dynamic
// service read → Bytes|List|String) flowing into a CONCRETELY-typed
// param (`s3-send-resp`'s body:Bytes). The nested body compile now
// narrows the binding to the declared type — by the same entry-guard
// contract as the Any-arg narrowing — so the callee's chunk loop
// (`slice i hi body` under a `for`) matches mono instead of cascading
// no_signature into "for: body nets multiple values per iteration".
func TestStampDisjunctArgNarrowsToParam(t *testing.T) {
	evs := stampEventsForNet(t, `
module [
  import "aql:net"
  def s2-send-resp fn [[conn:Any status:Integer extra:List body:Bytes] [Any] [
    def hdrs (push (join "" ["content-length: " (convert String (size body))]) extra)
    def head (join "\r\n" (unshift (join "" ["HTTP/1.1 " (convert String status) " S"]) hdrs))
    Net.send-bytes (convert Bytes (join "" [head "\r\n\r\n"])) conn
    def total (size body)
    for [0 total 65536] [
      def hi (if ((add i 65536) lt total) [ add i 65536 ] [ total ])
      Net.send-bytes (slice i hi body) conn
    ]
    0
  ]]
  def s2-get fn [[conn:Any store:Any okey:String] [Any] [
    def data (call {op:"read" key: okey} store)
    def total (size data)
    def part (slice 0 total data)
    def cr (join "" ["content-range: bytes 0-1/" (convert String total)])
    s2-send-resp conn 206 [cr] part
  ]]
  export "DnT" { send: s2-send-resp/r get: s2-get/r }
] import
`)
	for _, name := range []string{"s2-send-resp", "s2-get"} {
		if reason, ok := stampOutcome(evs, name); !ok {
			t.Errorf("%s did not stamp: %s", name, reason)
		}
	}
}
