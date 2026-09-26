// Package picker opens the operating system's folder chooser. Browsers can't reveal real
// folder paths, so the local server shows the dialog on the user's desktop instead.
package picker

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"
)

var (
	ErrCancelled   = errors.New("no folder was chosen")
	ErrUnavailable = errors.New("no folder window is available on this system")
)

const title = "Choose where YTGrab saves downloads"

type runFunc func(ctx context.Context, name string, args []string, env []string) (stdout string, exitCode int, err error)

type Picker struct {
	goos     string
	run      runFunc
	lookPath func(string) (string, error)
}

func New() Picker {
	return Picker{goos: runtime.GOOS, run: runCommand, lookPath: exec.LookPath}
}

// Available reports whether Pick can show a dialog.
func (p Picker) Available() bool {
	_, _, err := p.command()
	return err == nil
}

// Pick shows the folder chooser, starting in initial, and returns the chosen path.
// The dialog scripts are constants; initial reaches them only through the environment.
func (p Picker) Pick(ctx context.Context, initial string) (string, error) {
	name, args, err := p.command()
	if err != nil {
		return "", err
	}
	env := append(os.Environ(), "YTGRAB_PICK_INITIAL="+initial)
	stdout, exitCode, err := p.run(ctx, name, args, env)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(stdout)
	if exitCode != 0 || path == "" {
		return "", ErrCancelled
	}
	return path, nil
}

func (p Picker) command() (string, []string, error) {
	switch p.goos {
	case "windows":
		shell, err := p.lookPath("powershell.exe")
		if err != nil {
			return "", nil, ErrUnavailable
		}
		return shell, []string{"-NoProfile", "-NonInteractive", "-STA", "-EncodedCommand", encodePowerShell(windowsScript)}, nil
	case "darwin":
		osascript, err := p.lookPath("osascript")
		if err != nil {
			return "", nil, ErrUnavailable
		}
		return osascript, []string{"-e", macScript}, nil
	default:
		if zenity, err := p.lookPath("zenity"); err == nil {
			return "sh", []string{"-c", `exec "$0" --file-selection --directory --title="$1" --filename="$YTGRAB_PICK_INITIAL/"`, zenity, title}, nil
		}
		if kdialog, err := p.lookPath("kdialog"); err == nil {
			return "sh", []string{"-c", `exec "$0" --title "$1" --getexistingdirectory "$YTGRAB_PICK_INITIAL"`, kdialog, title}, nil
		}
		return "", nil, ErrUnavailable
	}
}

// encodePowerShell returns the base64 UTF-16LE form accepted by -EncodedCommand, which
// avoids any command-line quoting.
func encodePowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	buf := make([]byte, len(units)*2)
	for i, unit := range units {
		buf[2*i] = byte(unit)
		buf[2*i+1] = byte(unit >> 8)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func runCommand(ctx context.Context, name string, args []string, env []string) (string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.String(), exitErr.ExitCode(), nil
	}
	if err != nil {
		return "", 0, err
	}
	return stdout.String(), 0, nil
}

const macScript = `set startFolder to system attribute "YTGRAB_PICK_INITIAL"
try
	set chosen to choose folder with prompt "` + title + `" default location (POSIX file startFolder)
on error number -128
	return ""
end try
return POSIX path of chosen`

// windowsScript shows the modern Explorer folder dialog (IFileOpenDialog with
// FOS_PICKFOLDERS), owned by an invisible topmost form so it opens in front of the browser.
const windowsScript = `$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
Add-Type -AssemblyName System.Windows.Forms
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class YTGrabFolderPicker {
    [ComImport, Guid("DC1C5A9C-E88A-4dde-A5A1-60F82A20AEF7")] class FileOpenDialog {}
    [ComImport, Guid("42f85136-db7e-439c-85f1-e4075d135fc8"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
    interface IFileOpenDialog {
        [PreserveSig] int Show(IntPtr parent);
        void SetFileTypes(); void SetFileTypeIndex(); void GetFileTypeIndex(); void Advise(); void Unadvise();
        void SetOptions(uint options); void GetOptions(out uint options);
        void SetDefaultFolder(IShellItem item); void SetFolder(IShellItem item);
        void GetFolder(); void GetCurrentSelection(); void SetFileName(); void GetFileName();
        void SetTitle([MarshalAs(UnmanagedType.LPWStr)] string title);
        void SetOkButtonLabel([MarshalAs(UnmanagedType.LPWStr)] string label);
        void SetFileNameLabel(); void GetResult(out IShellItem item);
    }
    [ComImport, Guid("43826D1E-E718-42EE-BC55-A1E261C37BFE"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
    interface IShellItem {
        void BindToHandler(); void GetParent();
        void GetDisplayName(uint type, [MarshalAs(UnmanagedType.LPWStr)] out string name);
    }
    [DllImport("shell32.dll", CharSet = CharSet.Unicode, PreserveSig = false)]
    static extern void SHCreateItemFromParsingName(string path, IntPtr bindContext, [MarshalAs(UnmanagedType.LPStruct)] Guid riid, out IShellItem item);

    public static string Pick(IntPtr owner, string title, string initial) {
        var dialog = (IFileOpenDialog)new FileOpenDialog();
        dialog.SetOptions(0x20 | 0x40 | 0x8); // PICKFOLDERS | FORCEFILESYSTEM | NOCHANGEDIR
        dialog.SetTitle(title);
        dialog.SetOkButtonLabel("Use this folder");
        if (!string.IsNullOrEmpty(initial) && System.IO.Directory.Exists(initial)) {
            IShellItem folder;
            SHCreateItemFromParsingName(initial, IntPtr.Zero, typeof(IShellItem).GUID, out folder);
            dialog.SetFolder(folder);
        }
        if (dialog.Show(owner) != 0) return "";
        IShellItem result;
        dialog.GetResult(out result);
        string path;
        result.GetDisplayName(0x80058000, out path); // SIGDN_FILESYSPATH
        return path;
    }
}
'@
$owner = New-Object System.Windows.Forms.Form
$owner.TopMost = $true
$owner.ShowInTaskbar = $false
$owner.Opacity = 0
$owner.StartPosition = 'CenterScreen'
$owner.Show()
$owner.Activate()
try {
    $path = [YTGrabFolderPicker]::Pick($owner.Handle, '` + title + `', $env:YTGRAB_PICK_INITIAL)
} finally {
    $owner.Close()
}
if (-not $path) { exit 1 }
[Console]::Out.Write($path)
`
