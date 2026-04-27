//go:build windows

package gather

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWindowsExecutor_Platform(t *testing.T) {
	exec := NewPlatformExecutor()
	assert.Equal(t, "windows", exec.Platform())
	assert.True(t, exec.Supported())
}

// These functions make real Win32 API calls but shouldn't crash.
func TestWindowsExecutor_FocusedWindow(t *testing.T) {
	exec := NewPlatformExecutor()
	// Should not panic, may return false/error depending on environment
	focused, err := exec.FocusedWindow(context.Background(), CalibrationPayload{})
	// We don't strictly assert the return because CI might not have an active window
	t.Logf("Focused: %v, err: %v", focused, err)
}

func TestWindowsExecutor_Release(t *testing.T) {
	exec := NewPlatformExecutor()

	// Release should not panic
	err := exec.Release(context.Background())
	assert.NoError(t, err)
}

func TestResolveAlbionWindow_Safe(t *testing.T) {
    hwnd, title, err := resolveAlbionWindow("fake_title_that_does_not_exist_123")
    assert.Equal(t, uintptr(0), hwnd)
    assert.Equal(t, "", title)
    assert.Error(t, err)
    assert.Equal(t, errAlbionWindowNotFound, err)
}
