package debugserve

import "net"

// netListen is a test seam (design/TEST-SEAMS.10.md): tests swap it to
// hand ListenAndServe a pre-closed listener so the non-ErrServerClosed
// srv.Serve error arm is drivable without racing a real socket.
var netListen = net.Listen
