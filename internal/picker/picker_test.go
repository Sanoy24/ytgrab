package picker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRun struct {
	stdout   string
	exitCode int
	err      error
	name     string
	args     []string
	env      []string
}

func (fake *fakeRun) run(_ context.Context, name string, args []string, env []string) (string, int, error) {
	fake.name, fake.args, fake.env = name, args, env
	return fake.stdout, fake.exitCode, fake.err
}

func TestWindowsPickerPassesInitialFolderByEnvironment(t *testing.T) {
	fake := &fakeRun{stdout: `D:\Videos` + "\r\n"}
	p := Picker{goos: "windows", run: fake.run, lookPath: func(string) (string, error) { return "powershell.exe", nil }}
	got, err := p.Pick(context.Background(), `C:\Users\me\Downloads"; Remove-Item x`)
	if err != nil || got != `D:\Videos` {
		t.Fatalf("Pick = %q, %v", got, err)
	}
	joined := strings.Join(fake.args, " ")
	if fake.name != "powershell.exe" || !strings.Contains(joined, "-EncodedCommand") || !strings.Contains(joined, "-STA") {
		t.Fatalf("command = %s %v", fake.name, fake.args)
	}
	if strings.Contains(joined, "Remove-Item") {
		t.Fatal("initial folder leaked into the command line")
	}
	if !contains(fake.env, `YTGRAB_PICK_INITIAL=C:\Users\me\Downloads"; Remove-Item x`) {
		t.Fatalf("initial folder not passed by environment: %v", fake.env)
	}
}

func TestPickerCancelAndUnavailable(t *testing.T) {
	cancelled := &fakeRun{stdout: "", exitCode: 1}
	p := Picker{goos: "darwin", run: cancelled.run, lookPath: func(string) (string, error) { return "/usr/bin/osascript", nil }}
	if _, err := p.Pick(context.Background(), "/Users/me"); !errors.Is(err, ErrCancelled) {
		t.Fatalf("cancel error = %v", err)
	}
	none := Picker{goos: "linux", run: cancelled.run, lookPath: func(string) (string, error) { return "", errors.New("not found") }}
	if none.Available() {
		t.Fatal("linux without zenity or kdialog reported available")
	}
	if _, err := none.Pick(context.Background(), "/home/me"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unavailable error = %v", err)
	}
}

func TestLinuxPrefersZenityThenKdialog(t *testing.T) {
	fake := &fakeRun{stdout: "/home/me/Videos\n"}
	p := Picker{goos: "linux", run: fake.run, lookPath: func(name string) (string, error) {
		if name == "kdialog" {
			return "/usr/bin/kdialog", nil
		}
		return "", errors.New("not found")
	}}
	got, err := p.Pick(context.Background(), "/home/me")
	if err != nil || got != "/home/me/Videos" || fake.name != "sh" || !contains(fake.args, "/usr/bin/kdialog") {
		t.Fatalf("Pick = %q, %v via %s", got, err, fake.name)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
