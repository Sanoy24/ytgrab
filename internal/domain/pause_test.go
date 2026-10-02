package domain

import "testing"

func TestPauseAndResume(t *testing.T) {
	job, _ := NewJob("https://youtu.be/jNQXAC9IVRw", VideoBest)
	job.State = Downloading
	if err := job.Transition(Paused); err != nil || !job.State.Active() {
		t.Fatalf("pause = %v, active %v", err, job.State.Active())
	}
	if err := job.Transition(Completed); err == nil {
		t.Fatal("a paused job can't complete")
	}
	if err := job.Transition(Queued); err != nil {
		t.Fatalf("resume = %v", err)
	}
	job.State = Paused
	if err := job.Transition(Cancelled); err != nil {
		t.Fatalf("cancel while paused = %v", err)
	}
}
