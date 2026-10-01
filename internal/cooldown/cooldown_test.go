package cooldown

import (
	"context"
	"testing"
	"time"
)

func TestBlocksEscalateAndSuccessResets(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	gate := New()
	gate.now = func() time.Time { return now }
	for _, want := range []time.Duration{15 * time.Minute, 30 * time.Minute, 60 * time.Minute, 60 * time.Minute} {
		if until := gate.Block(); until.Sub(now) != want {
			t.Fatalf("pause = %v, want %v", until.Sub(now), want)
		}
	}
	// A block never shortens a pause that is still running.
	if until := gate.Block(); until.Sub(now) != 60*time.Minute {
		t.Fatalf("pause shortened to %v", until.Sub(now))
	}
	now = now.Add(2 * time.Hour) // the pause has ended and a download succeeded
	gate.Success()
	if until := gate.Block(); until.Sub(now) != 15*time.Minute {
		t.Fatalf("after success, pause = %v, want 15m", until.Sub(now))
	}
	if _, active := gate.Until(); !active {
		t.Fatal("gate should be active")
	}
	now = now.Add(16 * time.Minute)
	if _, active := gate.Until(); active {
		t.Fatal("gate should expire")
	}
}

func TestWaitEndsOnResumeOrExpiry(t *testing.T) {
	gate := New()
	gate.steps = []time.Duration{time.Hour}
	gate.Block()
	done := make(chan error, 1)
	go func() { done <- gate.Wait(context.Background()) }()
	select {
	case <-done:
		t.Fatal("Wait returned while paused")
	case <-time.After(50 * time.Millisecond):
	}
	gate.Resume()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Resume did not wake Wait")
	}

	gate.steps = []time.Duration{30 * time.Millisecond}
	gate.Success()
	gate.Block()
	started := time.Now()
	if err := gate.Wait(context.Background()); err != nil || time.Since(started) < 20*time.Millisecond {
		t.Fatalf("Wait = %v after %v", err, time.Since(started))
	}

	gate.steps = []time.Duration{time.Hour}
	gate.Block()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gate.Wait(ctx); err == nil {
		t.Fatal("Wait ignored a cancelled context")
	}
}
