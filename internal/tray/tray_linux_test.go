//go:build linux

package tray

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// watcher stands in for a desktop's tray host.
type watcher struct{ registered chan string }

func (w watcher) RegisterStatusNotifierItem(sender dbus.Sender, path string) *dbus.Error {
	w.registered <- string(sender)
	return nil
}

// Runs under dbus-run-session in CI; skipped where there is no session bus.
func TestLinuxTrayRegistersIconAndMenu(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("no session bus; CI runs this under dbus-run-session")
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	host := watcher{registered: make(chan string, 1)}
	if err := conn.Export(host, "/StatusNotifierWatcher", "org.kde.StatusNotifierWatcher"); err != nil {
		t.Fatal(err)
	}
	if reply, err := conn.RequestName("org.kde.StatusNotifierWatcher", dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("RequestName = %v, %v", reply, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Actions{Open: func(string) {}, OpenFolder: func(string) {}}, func(ctx context.Context, hooks Hooks) error {
			hooks.Ready("http://127.0.0.1:8787/", func() string { return t.TempDir() })
			<-ctx.Done()
			return nil
		})
	}()

	var item string
	select {
	case item = <-host.registered:
	case err := <-done:
		t.Fatalf("Run returned before showing an icon: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the tray icon never registered")
	}

	// Read the menu the icon offers.
	var revision uint32
	var layout struct {
		ID       int32
		Props    map[string]dbus.Variant
		Children []dbus.Variant
	}
	call := conn.Object(item, "/StatusNotifierMenu").Call("com.canonical.dbusmenu.GetLayout", 0, int32(0), int32(-1), []string{})
	if err := call.Store(&revision, &layout); err != nil {
		t.Fatalf("GetLayout: %v", err)
	}
	labels := map[string]bool{}
	for _, child := range layout.Children {
		fields, ok := child.Value().([]any)
		if !ok || len(fields) < 2 {
			continue
		}
		if props, ok := fields[1].(map[string]dbus.Variant); ok {
			if label, ok := props["label"].Value().(string); ok {
				labels[label] = true
			}
		}
	}
	for _, want := range []string{"Open YTGrab", "Open downloads folder", "Quit YTGrab"} {
		if !labels[want] {
			t.Errorf("menu has no %q item; it has %v", want, labels)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the server stopped")
	}
}
