//go:build windows

// Package winutil holds small Windows helpers shared by several packages.
package winutil

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// EncodeCommand prepares a script for powershell.exe -EncodedCommand (UTF-16LE, base64),
// which sidesteps all command-line quoting and code-page issues.
func EncodeCommand(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// PowerShell runs a read-only query script without flashing a console window and returns
// UTF-8 stdout. Never use it for anything elevated or hidden-window: antivirus/EDR treats
// "unsigned exe → powershell -WindowStyle Hidden -ExecutionPolicy Bypass -EncodedCommand"
// as malware, kills the caller and quarantines its exe.
func PowerShell(ctx context.Context, script string) (string, error) {
	script = "[Console]::OutputEncoding=[Text.Encoding]::UTF8\n$ProgressPreference='SilentlyContinue'\n" + script
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", EncodeCommand(script))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// ErrCancelled is returned when the user declines the UAC prompt.
var ErrCancelled = errors.New("cancelled by user")

var procShellExecuteExW = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// shellExecuteInfo mirrors SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.Handle
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hIcon        windows.Handle
	hProcess     windows.Handle
}

const seeMaskNoCloseProcess = 0x00000040

// RunElevated starts exe with args as administrator (the UAC prompt names exe, so the
// user sees exactly who asks) and waits for it; a non-zero exit code is an error.
func RunElevated(exe, args string) error { return shellExecuteWait("runas", exe, args) }

// shellExecuteWait runs ShellExecuteEx on its own locked OS thread with an STA, as the
// shell expects COM on the calling thread; the thread is discarded afterwards.
func shellExecuteWait(verbName, file, args string) error {
	res := make(chan error, 1)
	go func() {
		runtime.LockOSThread() // never unlocked: the thread exits with the goroutine
		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
			defer windows.CoUninitialize()
		}
		res <- shellExecuteWaitRaw(verbName, file, args)
	}()
	return <-res
}

func shellExecuteWaitRaw(verbName, fileName, args string) error {
	verb, _ := windows.UTF16PtrFromString(verbName)
	file, err := windows.UTF16PtrFromString(fileName)
	if err != nil {
		return err
	}
	params, err := windows.UTF16PtrFromString(args)
	if err != nil {
		return err
	}
	info := shellExecuteInfo{fMask: seeMaskNoCloseProcess, lpVerb: verb, lpFile: file, lpParameters: params, nShow: windows.SW_HIDE}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if ok, _, err := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrCancelled
		}
		return fmt.Errorf("ShellExecuteEx: %w", err)
	}
	if info.hProcess == 0 {
		return nil
	}
	defer windows.CloseHandle(info.hProcess)
	if _, err := windows.WaitForSingleObject(info.hProcess, 120_000); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("%s exited with code %d", fileName, code)
	}
	return nil
}
