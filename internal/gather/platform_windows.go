//go:build windows

package gather

import (
	"context"
	"errors"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	mouseeventfRightDown = 0x0008
	mouseeventfRightUp   = 0x0010
	keyeventfKeyUp       = 0x0002
	swRestore            = 9
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procEnumWindows      = user32.NewProc("EnumWindows")
	procGetForeground    = user32.NewProc("GetForegroundWindow")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
	procGetWindowTextW   = user32.NewProc("GetWindowTextW")
	procGetWindowTextLen = user32.NewProc("GetWindowTextLengthW")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procClientToScreen   = user32.NewProc("ClientToScreen")
	procShowWindow       = user32.NewProc("ShowWindow")
	procBringWindowToTop = user32.NewProc("BringWindowToTop")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procMouseEvent       = user32.NewProc("mouse_event")
	procKeybdEvent       = user32.NewProc("keybd_event")
)

type windowsExecutor struct {
	mu            sync.Mutex
	rightBtnHeld  bool
}

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type winPoint struct {
	X int32
	Y int32
}

func NewPlatformExecutor() Executor {
	return &windowsExecutor{}
}

func (e *windowsExecutor) Supported() bool {
	return true
}

func (e *windowsExecutor) Platform() string {
	return "windows"
}

func (e *windowsExecutor) CaptureCalibration(_ context.Context, input CalibrationPayload) (CalibrationPayload, error) {
	hwnd, title, err := resolveAlbionWindow(input.TitleHint)
	if err != nil {
		return CalibrationPayload{}, err
	}
	if err := focusWindow(hwnd); err != nil {
		return CalibrationPayload{}, err
	}

	rect, err := clientRectInScreen(hwnd)
	if err != nil {
		return CalibrationPayload{}, err
	}

	input = NormalizeCalibration(input)
	input.WindowRect = rect
	if input.TitleHint == "" {
		input.TitleHint = title
	}
	return input, nil
}

func (e *windowsExecutor) FocusedWindow(_ context.Context, _ CalibrationPayload) (bool, error) {
	_, _, err := foregroundAlbionWindow()
	if err != nil {
		if errors.Is(err, errForegroundWindowNotAlbion) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (e *windowsExecutor) ExecuteStep(_ context.Context, calibration CalibrationPayload, step *StepResult) error {
	// Release any held right-click first for steps that aren't continuing travel.
	// Applies to: interact clicks, key taps (mount/dismount), SkipClick steps,
	// and any Move step that explicitly asked for a release.
	if step.ReleaseHold || step.Mode != StepModeMove || step.KeyTap != "" || step.SkipClick {
		e.releaseRightLocked()
	}

	// Key tap (mount toggle, test clicks).
	if step.KeyTap != "" {
		vk, ok := virtualKeyFromName(step.KeyTap)
		if !ok {
			return errors.New("unsupported key tap: " + step.KeyTap)
		}
		tapKey(vk)
	}

	if step.SkipClick {
		return nil
	}

	if step.Mode == StepModeInteract {
		if err := resolveInteractClick(calibration, step); err != nil {
			return err
		}
	}

	if _, _, callErr := procSetCursorPos.Call(uintptr(step.ClickX), uintptr(step.ClickY)); callErr != syscall.Errno(0) {
		return callErr
	}

	// Press-and-hold travel: keep the right button down across ticks while we
	// steer via SetCursorPos. Albion follows the cursor as long as the button
	// stays pressed, giving smooth continuous movement.
	if step.Mode == StepModeMove && step.HoldMove {
		e.mu.Lock()
		if !e.rightBtnHeld {
			time.Sleep(15 * time.Millisecond)
			procMouseEvent.Call(mouseeventfRightDown, 0, 0, 0, 0)
			e.rightBtnHeld = true
		}
		e.mu.Unlock()
		return nil
	}

	// Default: discrete right-click (used for interact and for movement when
	// HoldMove is not requested).
	time.Sleep(35 * time.Millisecond)
	procMouseEvent.Call(mouseeventfRightDown, 0, 0, 0, 0)
	time.Sleep(20 * time.Millisecond)
	procMouseEvent.Call(mouseeventfRightUp, 0, 0, 0, 0)
	time.Sleep(25 * time.Millisecond)

	return nil
}

func (e *windowsExecutor) Release(_ context.Context) error {
	e.releaseRightLocked()
	return nil
}

func (e *windowsExecutor) releaseRightLocked() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.rightBtnHeld {
		return
	}
	procMouseEvent.Call(mouseeventfRightUp, 0, 0, 0, 0)
	e.rightBtnHeld = false
}

func resolveInteractClick(calibration CalibrationPayload, step *StepResult) error {
	probeRadius := clamp(calibration.InteractRadiusPx/2, 24, 60)
	rect := calibration.WindowRect

	if visionRect, ok := buildInteractVisionRect(step.ClickX, step.ClickY, probeRadius, rect); ok {
		if screenshot, err := captureScreenRect(visionRect); err == nil {
			if point, ok := findBestVisionPoint(
				screenshot,
				visionRect,
				probePoint{X: step.ClickX, Y: step.ClickY},
			); ok {
				step.ClickX = point.X
				step.ClickY = point.Y
				step.Message = "interaction click dispatched (vision confirmed)"
				return nil
			}
		}
	}

	step.Message = "interaction click dispatched (projected fallback)"
	return nil
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
func tapKey(vk uintptr) {
	procKeybdEvent.Call(vk, 0, 0, 0)
	time.Sleep(30 * time.Millisecond)
	procKeybdEvent.Call(vk, 0, keyeventfKeyUp, 0)
}

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

var errForegroundWindowNotAlbion = errors.New("foreground window is not Albion Online")
var errAlbionWindowNotFound = errors.New("could not find Albion Online window")

func foregroundAlbionWindow() (uintptr, string, error) {
	hwnd, _, err := procGetForeground.Call()
	if hwnd == 0 {
		if err != syscall.Errno(0) {
			return 0, "", err
		}
		return 0, "", errors.New("no foreground window detected")
	}

	title, err := windowTitle(hwnd)
	if err != nil {
		return 0, "", err
	}
	if !strings.Contains(strings.ToLower(title), "albion") {
		return 0, title, errForegroundWindowNotAlbion
	}
	return hwnd, title, nil
}

func windowTitle(hwnd uintptr) (string, error) {
	length, _, err := procGetWindowTextLen.Call(hwnd)
	if length == 0 {
		if err != syscall.Errno(0) {
			return "", err
		}
		return "", nil
	}

	buffer := make([]uint16, length+1)
	length, _, err = procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if length == 0 {
		if err != syscall.Errno(0) {
			return "", err
		}
		return "", nil
	}
	return syscall.UTF16ToString(buffer[:length]), nil
}

func resolveAlbionWindow(titleHint string) (uintptr, string, error) {
	if hwnd, title, err := foregroundAlbionWindow(); err == nil {
		return hwnd, title, nil
	}

	preferred := strings.ToLower(strings.TrimSpace(titleHint))
	var matchedHWND uintptr
	var matchedTitle string

	callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
			return 1
		}

		title, err := windowTitle(hwnd)
		if err != nil || title == "" {
			return 1
		}

		lowerTitle := strings.ToLower(title)
		if preferred != "" && strings.Contains(lowerTitle, preferred) {
			matchedHWND = hwnd
			matchedTitle = title
			return 0
		}

		if strings.Contains(lowerTitle, "albion") && matchedHWND == 0 {
			matchedHWND = hwnd
			matchedTitle = title
			if preferred == "" {
				return 0
			}
		}

		return 1
	})

	if ok, _, err := procEnumWindows.Call(callback, 0); ok == 0 && err != syscall.Errno(0) {
		return 0, "", err
	}

	if matchedHWND == 0 {
		return 0, "", errAlbionWindowNotFound
	}

	return matchedHWND, matchedTitle, nil
}

func focusWindow(hwnd uintptr) error {
	procShowWindow.Call(hwnd, swRestore)
	procBringWindowToTop.Call(hwnd)
	procSetForeground.Call(hwnd)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		foreground, _, err := procGetForeground.Call()
		if foreground == hwnd {
			return nil
		}
		if foreground == 0 && err != syscall.Errno(0) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}

	return errors.New("failed to focus Albion Online window")
}

func clientRectInScreen(hwnd uintptr) (WindowRect, error) {
	var rect winRect
	if ok, _, err := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok == 0 {
		if err != syscall.Errno(0) {
			return WindowRect{}, err
		}
		return WindowRect{}, errors.New("failed to read Albion client rect")
	}

	origin := winPoint{}
	if ok, _, err := procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&origin))); ok == 0 {
		if err != syscall.Errno(0) {
			return WindowRect{}, err
		}
		return WindowRect{}, errors.New("failed to convert Albion client rect to screen coordinates")
	}

	width := int(rect.Right - rect.Left)
	height := int(rect.Bottom - rect.Top)

	return WindowRect{
		Left:   int(origin.X),
		Top:    int(origin.Y),
		Right:  int(origin.X) + width,
		Bottom: int(origin.Y) + height,
	}, nil
}
