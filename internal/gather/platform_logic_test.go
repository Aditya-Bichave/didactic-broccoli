package gather

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVirtualKeyFromName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uintptr
		ok       bool
	}{
		{"Empty", "", 0, false},
		{"Lowercase letter", "a", 0x41, true},
		{"Uppercase letter", "A", 0x41, true},
		{"Number", "5", 0x35, true},
		{"Space", "SPACE", 0x20, true},
		{"Space mixed case", "Space", 0x20, true},
		{"Escape", "ESC", 0x1B, true},
		{"Escape full", "ESCAPE", 0x1B, true},
		{"Invalid", "INVALID", 0, false},
		{"Symbol", "!", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := virtualKeyFromName(tt.input)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.expected, val)
			}
		})
	}
}

func TestBuildInteractVisionRect(t *testing.T) {
	bounds := WindowRect{Left: 0, Top: 0, Right: 1920, Bottom: 1080}

	tests := []struct {
		name        string
		centerX     int
		centerY     int
		probeRadius int
		bounds      WindowRect
		expectOk    bool
		expectRect  WindowRect
	}{
		{
			name:        "Normal center point",
			centerX:     500,
			centerY:     500,
			probeRadius: 40, // size will be clamp(120, 72, 160) = 120, half = 60
			bounds:      bounds,
			expectOk:    true,
			expectRect:  WindowRect{Left: 440, Top: 440, Right: 560, Bottom: 560},
		},
		{
			name:        "Clamped small radius",
			centerX:     500,
			centerY:     500,
			probeRadius: 10, // size clamp(30, 72, 160) = 72, half = 36
			bounds:      bounds,
			expectOk:    true,
			expectRect:  WindowRect{Left: 464, Top: 464, Right: 536, Bottom: 536},
		},
		{
			name:        "Clamped large radius",
			centerX:     500,
			centerY:     500,
			probeRadius: 100, // size clamp(300, 72, 160) = 160, half = 80
			bounds:      bounds,
			expectOk:    true,
			expectRect:  WindowRect{Left: 420, Top: 420, Right: 580, Bottom: 580},
		},
		{
			name:        "Edge collision top-left",
			centerX:     10,
			centerY:     10,
			probeRadius: 40, // size 120, half 60. Should clamp to bounds
			bounds:      bounds,
			expectOk:    true,
			expectRect:  WindowRect{Left: 0, Top: 0, Right: 70, Bottom: 70}, // actually wait: centerX(10) - half(60) = -50 -> clamp(0). centerX+half = 70 -> clamp(70). So 0 to 70.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rect, ok := buildInteractVisionRect(tt.centerX, tt.centerY, tt.probeRadius, tt.bounds)
			assert.Equal(t, tt.expectOk, ok)
			if ok {
				assert.Equal(t, tt.expectRect, rect)
			}
		})
	}
}

func TestBuildInteractVisionRect_TooSmall(t *testing.T) {
	// A boundary too small for the rect should return false
	bounds := WindowRect{Left: 0, Top: 0, Right: 10, Bottom: 10}
	_, ok := buildInteractVisionRect(5, 5, 40, bounds)
	assert.False(t, ok)
}
