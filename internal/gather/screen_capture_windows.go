//go:build windows

package gather

import (
	"errors"
	"image"
	"syscall"
	"unsafe"
)

const (
	biRGB   = 0
	dibRGB  = 0
	srccopy = 0x00CC0020
)

var (
	gdi32                      = syscall.NewLazyDLL("gdi32.dll")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procGetDC                  = user32.NewProc("GetDC")
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

func captureScreenRect(rect WindowRect) (*image.RGBA, error) {
	width := rect.Width()
	height := rect.Height()
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid capture rectangle")
	}

	hdc, _, err := procGetDC.Call(0)
	if hdc == 0 {
		if err != syscall.Errno(0) {
			return nil, err
		}
		return nil, errors.New("failed to acquire screen DC")
	}
	defer procReleaseDC.Call(0, hdc)

	memDC, _, err := procCreateCompatibleDC.Call(hdc)
	if memDC == 0 {
		if err != syscall.Errno(0) {
			return nil, err
		}
		return nil, errors.New("failed to create memory DC")
	}
	defer procDeleteDC.Call(memDC)

	bitmap, _, err := procCreateCompatibleBitmap.Call(hdc, uintptr(width), uintptr(height))
	if bitmap == 0 {
		if err != syscall.Errno(0) {
			return nil, err
		}
		return nil, errors.New("failed to create bitmap")
	}
	defer procDeleteObject.Call(bitmap)

	previous, _, err := procSelectObject.Call(memDC, bitmap)
	if previous == 0 {
		if err != syscall.Errno(0) {
			return nil, err
		}
		return nil, errors.New("failed to select bitmap into DC")
	}
	defer procSelectObject.Call(memDC, previous)

	if ok, _, err := procBitBlt.Call(
		memDC,
		0,
		0,
		uintptr(width),
		uintptr(height),
		hdc,
		uintptr(rect.Left),
		uintptr(rect.Top),
		srccopy,
	); ok == 0 {
		if err != syscall.Errno(0) {
			return nil, err
		}
		return nil, errors.New("failed to copy screen pixels")
	}

	info := bitmapInfo{
		Header: bitmapInfoHeader{
			Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			Width:       int32(width),
			Height:      -int32(height),
			Planes:      1,
			BitCount:    32,
			Compression: biRGB,
		},
	}

	raw := make([]byte, width*height*4)
	if rows, _, err := procGetDIBits.Call(
		memDC,
		bitmap,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&raw[0])),
		uintptr(unsafe.Pointer(&info)),
		dibRGB,
	); rows == 0 {
		if err != syscall.Errno(0) {
			return nil, err
		}
		return nil, errors.New("failed to read bitmap pixels")
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(raw); i += 4 {
		img.Pix[i+0] = raw[i+2]
		img.Pix[i+1] = raw[i+1]
		img.Pix[i+2] = raw[i+0]
		img.Pix[i+3] = 0xFF
	}

	return img, nil
}
