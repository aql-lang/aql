//go:build unix

package termback

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// The real SIGWINCH wiring: a delivered signal becomes a resize tick,
// and closing stop ends the forwarder (proved by the tick channel
// closing behind it).
func TestPlatformResizeSignal(t *testing.T) {
	stop := make(chan struct{})
	ticks := platformResizeSignal(stop)
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Signal(syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ticks:
	case <-time.After(2 * time.Second):
		t.Fatal("no tick for SIGWINCH")
	}
	close(stop)
	select {
	case _, open := <-ticks:
		if open {
			// a second pending tick is fine; the close must follow
			select {
			case _, still := <-ticks:
				if still {
					t.Fatal("tick channel stayed open after stop")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("tick channel did not close after stop")
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tick channel did not close after stop")
	}
}
