//go:build windows

package tray

import (
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A notification has to come from the tray icon itself. fyne.io/systray (v1.12) keeps
// that icon on a hidden window of class "SystrayClass" with icon ID 100; if a future
// version changes either, notifications are skipped and everything else keeps working.
const (
	trayWindowClass = "SystrayClass"
	trayIconID      = 100
)

var shellNotifyIcon = windows.NewLazySystemDLL("shell32.dll").NewProc("Shell_NotifyIconW")

// notifyIconData is NOTIFYICONDATAW (976 bytes on 64-bit Windows).
type notifyIconData struct {
	Size                       uint32
	Wnd                        windows.Handle
	ID, Flags, CallbackMessage uint32
	Icon                       windows.Handle
	Tip                        [128]uint16
	State, StateMask           uint32
	Info                       [256]uint16
	TimeoutOrVersion           uint32 // a union in the C struct
	InfoTitle                  [64]uint16
	InfoFlags                  uint32
	GuidItem                   windows.GUID
	BalloonIcon                windows.Handle
}

// EnumWindows callbacks can't be freed, so there is one, reporting into findResult.
var (
	findMu     sync.Mutex
	findResult windows.HWND
	findPID    = windows.GetCurrentProcessId()
	findWindow = windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil || pid != findPID {
			return 1
		}
		var name [64]uint16
		if n, err := windows.GetClassName(hwnd, &name[0], int32(len(name))); err == nil && windows.UTF16ToString(name[:n]) == trayWindowClass {
			findResult = hwnd
			return 0 // stop
		}
		return 1
	})
)

func trayWindow() windows.HWND {
	findMu.Lock()
	defer findMu.Unlock()
	if findResult == 0 {
		_ = windows.EnumWindows(findWindow, nil) // reports an error when the callback stops it early
	}
	return findResult
}

// notify shows a Windows notification from YTGrab's tray icon.
func notify(title, text string) {
	hwnd := trayWindow()
	if hwnd == 0 {
		return
	}
	const nimModify, nifInfo = 0x1, 0x10
	data := notifyIconData{Wnd: windows.Handle(hwnd), ID: trayIconID, Flags: nifInfo}
	data.Size = uint32(unsafe.Sizeof(data))
	fill(data.InfoTitle[:], title)
	fill(data.Info[:], text)
	_, _, _ = shellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&data)))
}

// fill copies s into a fixed, NUL-terminated buffer, shortening it with "…" if needed.
func fill(dst []uint16, s string) {
	if strings.ContainsRune(s, 0) {
		return
	}
	runes := []rune(s)
	for keep := len(runes); keep >= 0; keep-- {
		text := runes[:keep]
		if keep < len(runes) {
			text = append(text[:keep:keep], '…')
		}
		if encoded := utf16.Encode(text); len(encoded) < len(dst) { // dst is zeroed: room for NUL
			copy(dst, encoded)
			return
		}
	}
}
