package gather

import "strings"

// virtualKeyFromName maps a short key label (e.g. "A", "W") to its Windows
// virtual-key code. Only letters and a tiny whitelist of common game keys are
// accepted — we don't want callers passing arbitrary scancodes to keybd_event.
func virtualKeyFromName(name string) (uintptr, bool) {
	if len(name) == 0 {
		return 0, false
	}
	upper := strings.ToUpper(name)
	if len(upper) == 1 {
		c := upper[0]
		if c >= 'A' && c <= 'Z' {
			return uintptr(c), true
		}
		if c >= '0' && c <= '9' {
			return uintptr(c), true
		}
	}
	switch upper {
	case "SPACE":
		return 0x20, true
	case "ESC", "ESCAPE":
		return 0x1B, true
	}
	return 0, false
}

func buildInteractVisionRect(centerX, centerY, probeRadius int, bounds WindowRect) (WindowRect, bool) {
	size := clamp(probeRadius*3, 72, 160)
	half := size / 2

	left := clamp(centerX-half, bounds.Left, bounds.Right)
	top := clamp(centerY-half, bounds.Top, bounds.Bottom)
	right := clamp(centerX+half, bounds.Left, bounds.Right)
	bottom := clamp(centerY+half, bounds.Top, bounds.Bottom)

	rect := WindowRect{
		Left:   left,
		Top:    top,
		Right:  right,
		Bottom: bottom,
	}

	return rect, rect.Width() >= 32 && rect.Height() >= 32
}
