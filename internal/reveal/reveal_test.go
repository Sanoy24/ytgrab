package reveal

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestCommands(t *testing.T) {
	if got := windowsCommandLine(`C:\Users\me\Videos\Title [id] 1080p.mp4`); got != `explorer.exe /select,"C:\Users\me\Videos\Title [id] 1080p.mp4"` {
		t.Errorf("windows command line = %s", got)
	}
	if got, want := macCommand("/Users/me/Movies/a b.mp4"), []string{"open", "-R", "/Users/me/Movies/a b.mp4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("mac command = %v", got)
	}
	got := linuxShowItems("/home/me/Videos/a b#1.mp4")
	want := []string{"dbus-send", "--session", "--print-reply", "--dest=org.freedesktop.FileManager1", "--type=method_call",
		"/org/freedesktop/FileManager1", "org.freedesktop.FileManager1.ShowItems",
		"array:string:file:///home/me/Videos/a%20b%231.mp4", "string:"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("linux command = %v", got)
	}
	if got, want := linuxOpenFolder("/home/me/Videos/a.mp4"), []string{"xdg-open", filepath.Dir("/home/me/Videos/a.mp4")}; !reflect.DeepEqual(got, want) {
		t.Errorf("linux fallback = %v", got)
	}
}
