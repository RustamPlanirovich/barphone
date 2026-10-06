//go:build windows

package launch

import (
	"fmt"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comdlg32                 = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	procCommDlgExtendedError = comdlg32.NewProc("CommDlgExtendedError")
	pickTitle                = "barphone: выберите программу или файл"
	pickFilter               = "Программы и ярлыки\x00*.exe;*.lnk;*.bat;*.cmd;*.ps1;*.url\x00Все файлы\x00*.*\x00\x00"
)

// openFileName mirrors OPENFILENAMEW.
type openFileName struct {
	lStructSize       uint32
	hwndOwner         windows.Handle
	hInstance         windows.Handle
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

const (
	ofnNoChangeDir        = 0x00000008
	ofnPathMustExist      = 0x00000800
	ofnFileMustExist      = 0x00001000
	ofnExplorer           = 0x00080000
	ofnNoDereferenceLinks = 0x00100000 // keep .lnk paths: shortcuts are fine deck targets
	ofnDontAddToRecent    = 0x02000000
	maxPath               = 4096
)

// openFileDialog returns the chosen path, or "" when the user cancels.
// Must run on a COM STA thread.
func openFileDialog(title string) (string, error) {
	buf := make([]uint16, maxPath)
	filter := utf16.Encode([]rune(pickFilter)) // contains NULs, so not UTF16FromString
	t, _ := windows.UTF16PtrFromString(title)
	ofn := openFileName{
		lpstrFilter:  &filter[0],
		nFilterIndex: 1,
		lpstrFile:    &buf[0],
		nMaxFile:     uint32(len(buf)),
		lpstrTitle:   t,
		flags:        ofnExplorer | ofnPathMustExist | ofnFileMustExist | ofnNoChangeDir | ofnNoDereferenceLinks | ofnDontAddToRecent,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	if ok, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); ok == 0 {
		if code, _, _ := procCommDlgExtendedError.Call(); code != 0 {
			return "", fmt.Errorf("GetOpenFileName failed: 0x%x", code)
		}
		return "", nil // cancelled
	}
	return windows.UTF16ToString(buf), nil
}
