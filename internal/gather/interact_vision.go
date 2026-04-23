package gather

import (
	"image"
	"math"
	"sort"
)

type scoredProbePoint struct {
	probePoint
	score float64
}

func findBestVisionPoint(img image.Image, region WindowRect, predicted probePoint) (probePoint, bool) {
	scored := collectVisionCandidates(img, region, predicted)
	if len(scored) == 0 {
		return probePoint{}, false
	}
	return scored[0].probePoint, true
}

func buildVisionProbePoints(img image.Image, region WindowRect, predicted probePoint, fallbackRadius int) []probePoint {
	points := []probePoint{{X: predicted.X, Y: predicted.Y}}
	minSpacing := 7
	scored := collectVisionCandidates(img, region, predicted)
	for _, candidate := range scored {
		if len(points) >= 32 {
			break
		}
		if tooCloseToExisting(points, candidate.probePoint, minSpacing) {
			continue
		}
		points = append(points, candidate.probePoint)
	}

	fallback := buildInteractProbePoints(predicted.X, predicted.Y, fallbackRadius, 8, region)
	points = append(points, fallback...)
	return dedupeProbePoints(points)
}

func collectVisionCandidates(img image.Image, region WindowRect, predicted probePoint) []scoredProbePoint {
	bounds := img.Bounds()
	if bounds.Empty() {
		return nil
	}

	bgR, bgG, bgB := averageBorderColor(img)
	centerLocal := probePoint{
		X: clamp(predicted.X-region.Left, bounds.Min.X, bounds.Max.X-1),
		Y: clamp(predicted.Y-region.Top, bounds.Min.Y, bounds.Max.Y-1),
	}

	scored := make([]scoredProbePoint, 0, bounds.Dx()*bounds.Dy()/16)
	step := 4
	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y += step {
		for x := bounds.Min.X + 1; x < bounds.Max.X-1; x += step {
			score := scoreVisionPoint(img, x, y, centerLocal, bgR, bgG, bgB)
			if score <= 0 {
				continue
			}
			scored = append(scored, scoredProbePoint{
				probePoint: probePoint{X: x + region.Left, Y: y + region.Top},
				score:      score,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			dxI := scored[i].X - predicted.X
			dyI := scored[i].Y - predicted.Y
			dxJ := scored[j].X - predicted.X
			dyJ := scored[j].Y - predicted.Y
			return dxI*dxI+dyI*dyI < dxJ*dxJ+dyJ*dyJ
		}
		return scored[i].score > scored[j].score
	})

	return scored
}

func averageBorderColor(img image.Image) (float64, float64, float64) {
	b := img.Bounds()
	if b.Empty() {
		return 0, 0, 0
	}

	var sumR, sumG, sumB float64
	var count float64

	sample := func(x, y int) {
		r, g, bl, _ := img.At(x, y).RGBA()
		sumR += float64(r >> 8)
		sumG += float64(g >> 8)
		sumB += float64(bl >> 8)
		count++
	}

	for x := b.Min.X; x < b.Max.X; x++ {
		sample(x, b.Min.Y)
		sample(x, b.Max.Y-1)
	}
	for y := b.Min.Y + 1; y < b.Max.Y-1; y++ {
		sample(b.Min.X, y)
		sample(b.Max.X-1, y)
	}

	if count == 0 {
		return 0, 0, 0
	}
	return sumR / count, sumG / count, sumB / count
}

func scoreVisionPoint(img image.Image, x, y int, center probePoint, bgR, bgG, bgB float64) float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	pr := float64(r >> 8)
	pg := float64(g >> 8)
	pb := float64(b >> 8)

	colorDelta := math.Sqrt(square(pr-bgR)+square(pg-bgG)+square(pb-bgB)) / 441.67295593
	if colorDelta < 0.08 {
		return 0
	}

	left := grayscaleAt(img, x-1, y)
	right := grayscaleAt(img, x+1, y)
	up := grayscaleAt(img, x, y-1)
	down := grayscaleAt(img, x, y+1)
	gradient := (math.Abs(right-left) + math.Abs(down-up)) / 510.0

	bounds := img.Bounds()
	dist := math.Hypot(float64(x-center.X), float64(y-center.Y))
	maxDist := math.Hypot(float64(bounds.Dx()), float64(bounds.Dy()))
	distanceBias := 1.0 - (dist / math.Max(maxDist, 1))
	if distanceBias < 0 {
		distanceBias = 0
	}

	verticalBias := float64(y-bounds.Min.Y) / math.Max(float64(bounds.Dy()-1), 1)
	score := (colorDelta * 0.55) + (gradient * 0.30) + (distanceBias * 0.10) + (verticalBias * 0.05)

	if gradient < 0.04 && colorDelta < 0.16 {
		return 0
	}

	return score
}

func grayscaleAt(img image.Image, x, y int) float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	return (0.299 * float64(r>>8)) + (0.587 * float64(g>>8)) + (0.114 * float64(b>>8))
}

func tooCloseToExisting(points []probePoint, candidate probePoint, minSpacing int) bool {
	minSpacingSq := minSpacing * minSpacing
	for _, point := range points {
		dx := point.X - candidate.X
		dy := point.Y - candidate.Y
		if dx*dx+dy*dy <= minSpacingSq {
			return true
		}
	}
	return false
}

func square(value float64) float64 {
	return value * value
}
