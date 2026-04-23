package gather

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildInteractProbePoints_StartsAtCenterThenAlternatesRows(t *testing.T) {
	points := buildInteractProbePoints(100, 200, 16, 8, WindowRect{Left: 0, Top: 0, Right: 400, Bottom: 400})

	require.NotEmpty(t, points)
	require.Equal(t, probePoint{X: 100, Y: 200}, points[0])
	require.Contains(t, points, probePoint{X: 100, Y: 200})
	require.Contains(t, points, probePoint{X: 100, Y: 192})
	require.Contains(t, points, probePoint{X: 100, Y: 208})
}

func TestBuildInteractProbePoints_ClampsToWindowBounds(t *testing.T) {
	points := buildInteractProbePoints(10, 10, 20, 10, WindowRect{Left: 0, Top: 0, Right: 25, Bottom: 25})

	require.NotEmpty(t, points)
	for _, point := range points {
		require.GreaterOrEqual(t, point.X, 0)
		require.GreaterOrEqual(t, point.Y, 0)
		require.LessOrEqual(t, point.X, 25)
		require.LessOrEqual(t, point.Y, 25)
	}
}

func TestDominantCursorHandle_ReturnsMostCommonHandle(t *testing.T) {
	handle := dominantCursorHandle([]uintptr{11, 22, 11, 33, 11, 22})

	require.Equal(t, uintptr(11), handle)
}
