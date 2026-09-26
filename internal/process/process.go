package process

import (
	"context"
	"os/exec"
)

// Start runs cmd and watches ctx. The returned function stops the watcher after
// cmd.Wait. Platform-specific termination stops child processes as well.
func Start(ctx context.Context, cmd *exec.Cmd) (func(), error) {
	prepare(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			terminate(cmd.Process)
		case <-done:
		}
	}()
	return func() { close(done) }, nil
}
