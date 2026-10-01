// Package cooldown pauses YouTube requests after YouTube starts limiting this network.
// Retrying immediately tends to extend a block, so downloads and format checks wait,
// with longer pauses if blocks keep happening.
package cooldown

import (
	"context"
	"sync"
	"time"
)

// Gate is shared by the download queue and format inspection.
type Gate struct {
	mu      sync.Mutex
	until   time.Time
	strikes int
	steps   []time.Duration
	now     func() time.Time
	changed chan struct{} // closed when the pause is lifted early
}

func New() *Gate {
	return WithSteps(15*time.Minute, 30*time.Minute, 60*time.Minute)
}

// WithSteps returns a gate whose successive pauses last the given durations.
func WithSteps(steps ...time.Duration) *Gate {
	return &Gate{steps: steps, now: time.Now, changed: make(chan struct{})}
}

// Block starts or extends a pause after YouTube limited a request, and returns its end.
func (gate *Gate) Block() time.Time {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	step := gate.steps[min(gate.strikes, len(gate.steps)-1)]
	gate.strikes++
	if until := gate.now().Add(step); until.After(gate.until) {
		gate.until = until
	}
	return gate.until
}

// Success resets the escalation after a request went through.
func (gate *Gate) Success() {
	gate.mu.Lock()
	gate.strikes = 0
	gate.mu.Unlock()
}

// Resume lifts the pause now, for example after the user changed networks.
func (gate *Gate) Resume() {
	gate.mu.Lock()
	gate.until = time.Time{}
	close(gate.changed)
	gate.changed = make(chan struct{})
	gate.mu.Unlock()
}

// Until returns the end of the current pause and whether one is active.
func (gate *Gate) Until() (time.Time, bool) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return gate.until, gate.now().Before(gate.until)
}

// Wait blocks until no pause is active, Resume is called, or ctx ends.
func (gate *Gate) Wait(ctx context.Context) error {
	for {
		gate.mu.Lock()
		remaining := gate.until.Sub(gate.now())
		changed := gate.changed
		gate.mu.Unlock()
		if remaining <= 0 {
			return nil
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-changed:
		case <-timer.C:
		}
		timer.Stop()
	}
}
