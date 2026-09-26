package app

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"ytgrab/internal/app/deps"
	"ytgrab/internal/config"
)

func TestServerStopsAfterContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{ShutdownTimeout: 2 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- serve(ctx, cfg, io.Discard, listener) }()

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/api/system/health")
	if err != nil {
		cancel()
		<-finished
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET health status = %d, want 200", response.StatusCode)
	}
	var report deps.Report
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if len(report.Dependencies) != 4 {
		t.Fatalf("health response lists %d dependencies, want 4", len(report.Dependencies))
	}

	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("serve after cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down after cancellation")
	}

	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err == nil {
		_ = connection.Close()
		t.Fatal("server still accepts connections after shutdown")
	}
}
