// Wave-4 coverage for the service package: the State String mapping,
// including the unknown fallback.
package service

import "testing"

func TestW4StateString(t *testing.T) {
	cases := []struct {
		state State
		want  string
	}{
		{StateStopped, "stopped"},
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StatePaused, "paused"},
		{StateStopping, "stopping"},
		{State(99), "unknown"},
		{State(-1), "unknown"},
	}
	for _, c := range cases {
		if got := c.state.String(); got != c.want {
			t.Errorf("State(%d).String() = %q, want %q", c.state, got, c.want)
		}
	}
}
