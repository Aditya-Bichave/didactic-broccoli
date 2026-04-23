package gather

type probePoint struct {
	X int
	Y int
}

func buildInteractProbePoints(centerX, centerY, radius, step int, rect WindowRect) []probePoint {
	if radius <= 0 {
		radius = 32
	}
	if step <= 0 {
		step = 8
	}

	points := make([]probePoint, 0, ((radius*2)/step+1)*((radius*2)/step+1))
	rowOffsets := makeAlternatingOffsets(radius, step)
	colOffsets := makeAlternatingOffsets(radius, step)

	for rowIndex, rowOffset := range rowOffsets {
		y := clamp(centerY+rowOffset, rect.Top, rect.Bottom)
		offsets := colOffsets

		if rowIndex%2 == 1 {
			offsets = make([]int, 0, len(colOffsets))
			for index := len(colOffsets) - 1; index >= 0; index-- {
				offsets = append(offsets, colOffsets[index])
			}
		}

		for _, colOffset := range offsets {
			x := clamp(centerX+colOffset, rect.Left, rect.Right)
			points = append(points, probePoint{X: x, Y: y})
		}
	}

	return dedupeProbePoints(points)
}

func makeAlternatingOffsets(radius, step int) []int {
	offsets := []int{0}
	for delta := step; delta <= radius; delta += step {
		offsets = append(offsets, -delta, delta)
	}
	return offsets
}

func dedupeProbePoints(points []probePoint) []probePoint {
	if len(points) == 0 {
		return nil
	}

	seen := make(map[probePoint]struct{}, len(points))
	deduped := make([]probePoint, 0, len(points))
	for _, point := range points {
		if _, ok := seen[point]; ok {
			continue
		}
		seen[point] = struct{}{}
		deduped = append(deduped, point)
	}
	return deduped
}

func dominantCursorHandle(handles []uintptr) uintptr {
	if len(handles) == 0 {
		return 0
	}

	counts := make(map[uintptr]int, len(handles))
	var winner uintptr
	bestCount := 0
	for _, handle := range handles {
		counts[handle]++
		if counts[handle] > bestCount {
			bestCount = counts[handle]
			winner = handle
		}
	}
	return winner
}
