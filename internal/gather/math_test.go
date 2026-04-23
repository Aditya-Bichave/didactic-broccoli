package gather

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeStep_UsesMoveRadiusWhenFar(t *testing.T) {
	cal := CalibrationPayload{
		WindowRect:         WindowRect{Left: 100, Top: 200, Right: 900, Bottom: 1000},
		MoveRadiusPx:       160,
		InteractRadiusPx:   64,
		InteractThresholdG: 12,
		SafeMarginX:        20,
		SafeMarginY:        20,
	}

	step := ComputeStep(cal, Point{X: 0, Y: 0}, Point{X: 60, Y: 0})

	require.Equal(t, StepModeMove, step.Mode)
	require.Equal(t, 613, step.ClickX)
	require.Equal(t, 713, step.ClickY)
	require.Equal(t, 60.0, step.Distance)
}

func TestComputeStep_ShrinksTravelRadiusWhenJustOutsideInteractRange(t *testing.T) {
	cal := CalibrationPayload{
		WindowRect:         WindowRect{Left: 100, Top: 200, Right: 900, Bottom: 1000},
		MoveRadiusPx:       160,
		InteractRadiusPx:   64,
		InteractThresholdG: 12,
		SafeMarginX:        20,
		SafeMarginY:        20,
	}

	step := ComputeStep(cal, Point{X: 0, Y: 0}, Point{X: 13, Y: 0})

	require.Equal(t, StepModeMove, step.Mode)
	dx := float64(step.ClickX - 500)
	dy := float64(step.ClickY - 600)
	mag := math.Hypot(dx, dy)
	require.Greater(t, mag, 110.0)
	require.Less(t, mag, 130.0)
}

func TestComputeStep_UsesInteractRadiusWhenNear(t *testing.T) {
	cal := CalibrationPayload{
		WindowRect:         WindowRect{Left: 50, Top: 80, Right: 650, Bottom: 680},
		MoveRadiusPx:       180,
		InteractRadiusPx:   48,
		InteractThresholdG: 10,
		SafeMarginX:        16,
		SafeMarginY:        16,
	}

	step := ComputeStep(cal, Point{X: 10, Y: 10}, Point{X: 13, Y: 12})

	require.Equal(t, StepModeInteract, step.Mode)
	require.Greater(t, step.ClickX, 350)
	require.Greater(t, step.ClickY, 380)
	require.Less(t, step.ClickX, 650)
	require.Less(t, step.ClickY, 680)
}

func TestComputeStep_ClampsToSafeMargins(t *testing.T) {
	cal := CalibrationPayload{
		WindowRect:         WindowRect{Left: 0, Top: 0, Right: 200, Bottom: 200},
		MoveRadiusPx:       500,
		InteractRadiusPx:   60,
		InteractThresholdG: 5,
		SafeMarginX:        30,
		SafeMarginY:        40,
	}

	// World delta (100, -100) -> iso-projected screen delta (0, +k): straight down
	// on screen. With MoveRadius 500 the Y-offset wants to be 500, but the safe
	// bottom is centerY(100)+60=160. clampVector scales the vector uniformly so
	// the direction is preserved (here X stays 0 since the vector is purely Y).
	step := ComputeStep(cal, Point{X: 0, Y: 0}, Point{X: 100, Y: -100})

	require.Equal(t, 100, step.ClickX)
	require.Equal(t, 160, step.ClickY)
}

// TestComputeStep_AgreesWithRendererForCardinals guards against regressing the
// iso rotation fix. World-deltas for the four cardinal directions must click in
// the same screen half/quadrant as the radar renderer draws the entity (see
// web/scripts/utils/DrawingUtils.js transformPoint, which uses proportionalities
// screenX ∝ +dx + dy, screenY ∝ +dx - dy). If these break, the backend click
// direction is misaligned with the rendered map and the character walks into
// walls — the original bug.
func TestComputeStep_AgreesWithRendererForCardinals(t *testing.T) {
	cal := CalibrationPayload{
		WindowRect:         WindowRect{Left: 0, Top: 0, Right: 1000, Bottom: 1000},
		MoveRadiusPx:       100,
		InteractRadiusPx:   40,
		InteractThresholdG: 5,
		SafeMarginX:        10,
		SafeMarginY:        10,
	}

	// Screen origin is centerX=500, centerY=500. Expected signs of the offset
	// (clickX-centerX, clickY-centerY) are derived from DrawingUtils.transformPoint.
	cases := []struct {
		name   string
		target Point
		wantDX int // -1, 0, +1
		wantDY int
	}{
		// dx=+1, dy=0  → screen (+, +) — lower-right of player on-screen.
		{"world +X (east)", Point{X: 10, Y: 0}, +1, +1},
		// dx=0, dy=+1  → screen (+, -) — upper-right on-screen.
		{"world +Y (north)", Point{X: 0, Y: 10}, +1, -1},
		// dx=-1, dy=0  → screen (-, -) — upper-left on-screen.
		{"world -X (west)", Point{X: -10, Y: 0}, -1, -1},
		// dx=0, dy=-1  → screen (-, +) — lower-left on-screen.
		{"world -Y (south)", Point{X: 0, Y: -10}, -1, +1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			step := ComputeStep(cal, Point{X: 0, Y: 0}, c.target)
			dx := step.ClickX - 500
			dy := step.ClickY - 500
			requireSign(t, dx, c.wantDX, "ClickX offset")
			requireSign(t, dy, c.wantDY, "ClickY offset")
		})
	}
}

func requireSign(t *testing.T, value, want int, label string) {
	t.Helper()
	switch {
	case want > 0:
		require.Greater(t, value, 0, "%s expected positive, got %d", label, value)
	case want < 0:
		require.Less(t, value, 0, "%s expected negative, got %d", label, value)
	default:
		require.Equal(t, 0, value, "%s expected zero", label)
	}
}
