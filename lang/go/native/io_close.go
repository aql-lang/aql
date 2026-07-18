package native

import "fmt"

// io_close.go — the polymorphic IO.close. Like aql:net's one closeHandler
// over Socket/Listener/Service, it closes whichever resource the argument
// holds: a File handle (IO.open) or a Watcher (IO.watch). IO.unwatch stays
// as the Watcher-specific alias. The dispatch is structural (each asXxx
// unwrap checks the payload's Go type), so a single TAny signature admits
// both resource types and a non-resource value is refused cleanly.
func doCloseWord(args []Value, r *Registry) ([]Value, error) {
	if fh, ok := asFileHandle(args[0]); ok {
		if err := fh.Close(); err != nil {
			return nil, r.AqlError("close_error", fmt.Sprintf("close: %v", err), "close")
		}
		return nil, nil
	}
	if wi, ok := asWatcherInfo(args[0]); ok {
		if err := wi.Stop(); err != nil {
			return nil, r.AqlError("close_error", fmt.Sprintf("close: %v", err), "close")
		}
		return nil, nil
	}
	return nil, r.AqlError("close_error",
		fmt.Sprintf("close: not a closable handle (got %s)", args[0].Parent), "close")
}
