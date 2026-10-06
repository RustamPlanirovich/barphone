//go:build windows

package launch

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shellItemImage asks the shell for an item's icon via IShellItemImageFactory. Unlike
// ExtractIcon this works for packaged (Store) apps and returns large, alpha-blended icons.
// Must run on a COM-initialized thread.

var (
	shell32                         = windows.NewLazySystemDLL("shell32.dll")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	gdi32                           = windows.NewLazySystemDLL("gdi32.dll")
	procGetObjectW                  = gdi32.NewProc("GetObjectW")
	procGetDIBits                   = gdi32.NewProc("GetDIBits")
	procDeleteObject                = gdi32.NewProc("DeleteObject")
	procGetDC                       = user32.NewProc("GetDC")
	procReleaseDC                   = user32.NewProc("ReleaseDC")

	iidShellItemImageFactory = windows.GUID{Data1: 0xbcc18b79, Data2: 0xba16, Data3: 0x442f,
		Data4: [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b}}
)

const (
	siigbfResizeToFit = 0x00
	siigbfIconOnly    = 0x04
)

type bitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type bitmapInfo struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
	bmiColors       [4]uint32 // unused for 32bpp BI_RGB, but GetDIBits expects room for it
}

func shellItemImage(parsingName string, size int) (image.Image, error) {
	name, err := windows.UTF16PtrFromString(parsingName)
	if err != nil {
		return nil, err
	}
	var factory unsafe.Pointer
	hr, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(name)), 0,
		uintptr(unsafe.Pointer(&iidShellItemImageFactory)), uintptr(unsafe.Pointer(&factory)))
	if hr != 0 || factory == nil {
		return nil, fmt.Errorf("SHCreateItemFromParsingName(%q): HRESULT 0x%08x", parsingName, uint32(hr))
	}
	// vtable: QueryInterface, AddRef, Release, GetImage
	vtbl := *(**[4]uintptr)(factory)
	defer syscall.SyscallN(vtbl[2], uintptr(factory))

	// GetImage takes SIZE by value; on amd64/arm64 an 8-byte struct travels in one register.
	sz := uintptr(uint32(size)) | uintptr(uint32(size))<<32
	var hbm windows.Handle
	hr, _, _ = syscall.SyscallN(vtbl[3], uintptr(factory), sz, siigbfResizeToFit|siigbfIconOnly,
		uintptr(unsafe.Pointer(&hbm)))
	if hr != 0 || hbm == 0 {
		return nil, fmt.Errorf("IShellItemImageFactory.GetImage: HRESULT 0x%08x", uint32(hr))
	}
	defer procDeleteObject.Call(uintptr(hbm))
	return hbitmapToImage(hbm)
}

func hbitmapToImage(hbm windows.Handle) (image.Image, error) {
	var bm bitmap
	if r, _, _ := procGetObjectW.Call(uintptr(hbm), unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); r == 0 {
		return nil, fmt.Errorf("GetObject failed")
	}
	w, h := int(bm.bmWidth), int(bm.bmHeight)
	if h < 0 {
		h = -h
	}
	if w <= 0 || h <= 0 || w > 2048 || h > 2048 {
		return nil, fmt.Errorf("unexpected bitmap size %dx%d", w, h)
	}
	bi := bitmapInfo{biWidth: int32(w), biHeight: -int32(h), biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Offsetof(bi.bmiColors))
	buf := make([]byte, w*h*4)
	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc)
	if r, _, _ := procGetDIBits.Call(hdc, uintptr(hbm), 0, uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0); r == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}

	// BGRA -> RGBA. The shell normally hands out premultiplied alpha, but some legacy
	// icons come straight (or with no alpha at all), so detect which one we got.
	hasAlpha, premultiplied := false, true
	for i := 0; i < len(buf); i += 4 {
		a := buf[i+3]
		if a != 0 {
			hasAlpha = true
		}
		if buf[i] > a || buf[i+1] > a || buf[i+2] > a {
			premultiplied = false
		}
	}
	pix := make([]byte, len(buf))
	for i := 0; i < len(buf); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = buf[i+2], buf[i+1], buf[i], buf[i+3]
		if !hasAlpha {
			pix[i+3] = 0xff
		}
	}
	rect := image.Rect(0, 0, w, h)
	if hasAlpha && premultiplied {
		return &image.RGBA{Pix: pix, Stride: w * 4, Rect: rect}, nil
	}
	return &image.NRGBA{Pix: pix, Stride: w * 4, Rect: rect}, nil
}
