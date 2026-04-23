package gather

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildVisionProbePoints_PrioritizesVisibleResourceBlob(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 80))
	fillRect(img, img.Bounds(), color.RGBA{R: 92, G: 124, B: 78, A: 255})
	fillRect(img, image.Rect(34, 20, 48, 60), color.RGBA{R: 123, G: 79, B: 42, A: 255})

	points := buildVisionProbePoints(
		img,
		WindowRect{Left: 200, Top: 300, Right: 280, Bottom: 380},
		probePoint{X: 240, Y: 340},
		28,
	)

	require.NotEmpty(t, points)
	require.Equal(t, probePoint{X: 240, Y: 340}, points[0])

	foundBlobCandidate := false
	for _, point := range points[:minInt(len(points), 8)] {
		if point.X >= 234 && point.X <= 248 && point.Y >= 324 && point.Y <= 360 {
			foundBlobCandidate = true
			break
		}
	}
	require.True(t, foundBlobCandidate, "expected a top-ranked candidate inside the resource blob")
}

func TestFindBestVisionPoint_ReturnsBlobCandidate(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 80))
	fillRect(img, img.Bounds(), color.RGBA{R: 92, G: 124, B: 78, A: 255})
	fillRect(img, image.Rect(34, 20, 48, 60), color.RGBA{R: 175, G: 78, B: 26, A: 255})

	point, ok := findBestVisionPoint(
		img,
		WindowRect{Left: 200, Top: 300, Right: 280, Bottom: 380},
		probePoint{X: 240, Y: 340},
	)

	require.True(t, ok)
	require.GreaterOrEqual(t, point.X, 234)
	require.LessOrEqual(t, point.X, 248)
	require.GreaterOrEqual(t, point.Y, 324)
	require.LessOrEqual(t, point.Y, 360)
}

func TestBuildVisionProbePoints_FallsBackWhenImageIsFlat(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	fillRect(img, img.Bounds(), color.RGBA{R: 100, G: 110, B: 100, A: 255})

	points := buildVisionProbePoints(
		img,
		WindowRect{Left: 50, Top: 60, Right: 98, Bottom: 108},
		probePoint{X: 74, Y: 84},
		24,
	)

	require.NotEmpty(t, points)
	require.Equal(t, probePoint{X: 74, Y: 84}, points[0])
	require.Contains(t, points, probePoint{X: 66, Y: 84})
}

func fillRect(img *image.RGBA, rect image.Rectangle, c color.RGBA) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
