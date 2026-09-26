package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("YTGRAB_TEST_PROCESS_MODE")
	if mode == "" {
		return
	}
	directory := os.Getenv("YTGRAB_TEST_PROCESS_DIR")
	if mode == "parent" {
		child := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
		child.Env = append(os.Environ(), "YTGRAB_TEST_PROCESS_MODE=child", "YTGRAB_TEST_PROCESS_DIR="+directory)
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		_ = os.WriteFile(filepath.Join(directory, "child.pid"), []byte(strconv.Itoa(child.Process.Pid)), 0600)
		for {
			time.Sleep(time.Second)
		}
	}
	if mode == "child" {
		for {
			_ = os.WriteFile(filepath.Join(directory, "heartbeat"), []byte(time.Now().UTC().String()), 0600)
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestCancellationStopsProcessTree(t *testing.T) {
	directory := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
	cmd.Env = append(os.Environ(), "YTGRAB_TEST_PROCESS_MODE=parent", "YTGRAB_TEST_PROCESS_DIR="+directory)
	stop, err := Start(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	var childPID int
	defer func() {
		if childPID != 0 {
			if child, err := os.FindProcess(childPID); err == nil {
				_ = child.Kill() // Clean up a child if the tree-stop assertion fails.
			}
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(filepath.Join(directory, "child.pid"))
		if err == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(contents)))
		}
		if childPID != 0 {
			if _, err := os.Stat(filepath.Join(directory, "heartbeat")); err == nil {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if childPID == 0 {
		cancel()
		_ = cmd.Wait()
		t.Fatal("helper child did not start")
	}
	if _, err := os.Stat(filepath.Join(directory, "heartbeat")); err != nil {
		cancel()
		_ = cmd.Wait()
		t.Fatal("helper child did not heartbeat")
	}
	cancel()
	_ = cmd.Wait()
	// Give a child that survived the parent time to write another heartbeat.
	time.Sleep(150 * time.Millisecond)
	first, err := os.ReadFile(filepath.Join(directory, "heartbeat"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	second, err := os.ReadFile(filepath.Join(directory, "heartbeat"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("child process kept running after cancellation")
	}
}
