//go:build windows

package tray

import (
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestFillShortensToFit(t *testing.T) {
	var buf [8]uint16
	fill(buf[:], "short")
	if got := windows.UTF16ToString(buf[:]); got != "short" {
		t.Fatalf("fill = %q", got)
	}
	buf = [8]uint16{}
	fill(buf[:], "a much longer title")
	if got := windows.UTF16ToString(buf[:]); got != "a much…" || buf[7] != 0 {
		t.Fatalf("fill = %q", got)
	}
	var tiny [2]uint16
	fill(tiny[:], strings.Repeat("😀", 5)) // surrogate pairs never split
	if got := windows.UTF16ToString(tiny[:]); got != "…" {
		t.Fatalf("fill = %q", got)
	}
}

func TestNotifyIconDataSize(t *testing.T) {
	// NOTIFYICONDATAW is 976 bytes on 64-bit Windows; a wrong layout puts the
	// notification's title and flags in the wrong place.
	if size := unsafe.Sizeof(notifyIconData{}); unsafe.Sizeof(uintptr(0)) == 8 && size != 976 {
		t.Fatalf("size = %d, want 976", size)
	}
}
