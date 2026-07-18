//go:build unix

package termback

import (
	"os"
	"os/signal"
	"syscall"
)

// platformResizeSignal wires SIGWINCH into resize ticks. The returned
// channel closes when stop closes.
func platformResizeSignal(stop <-chan struct{}) <-chan struct{} {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)
	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		defer signal.Stop(sig)
		for {
			select {
			case <-sig:
				select {
				case out <- struct{}{}:
				default:
				}
			case <-stop:
				return
			}
		}
	}()
	return out
}
