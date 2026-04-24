package gather

import "math"

const isoAxis = 0.70710678118

func NormalizeCalibration(cal CalibrationPayload) CalibrationPayload {
	if cal.SafeMarginX <= 0 {
		cal.SafeMarginX = 24
	}
	if cal.SafeMarginY <= 0 {
		cal.SafeMarginY = 24
	}
	if cal.MoveRadiusPx <= 0 {
		cal.MoveRadiusPx = 220
	}
	if cal.InteractRadiusPx <= 0 {
		cal.InteractRadiusPx = 80
	}
	if cal.InteractThresholdG <= 0 {
		cal.InteractThresholdG = 14
	}
	if cal.MountKey == "" {
		cal.MountKey = "A"
	}
	if cal.MountDelayMs <= 0 {
		// Tuned for ox — large mounts have a long mount animation. Horses usually
		// only need ~1200ms; users on faster mounts can lower this in the UI.
		cal.MountDelayMs = 3500
	}
	if cal.DismountDelayMs <= 0 {
		cal.DismountDelayMs = 1500
	}
	if cal.ClickRefreshMs <= 0 {
		cal.ClickRefreshMs = 1400
	}
	if cal.ClickDriftPx <= 0 {
		cal.ClickDriftPx = 35
	}
	return cal
}

func computeCenter(cal CalibrationPayload) (centerX int, centerY int) {
	return cal.WindowRect.Left + cal.WindowRect.Width()/2 + cal.CenterOffsetX,
		cal.WindowRect.Top + cal.WindowRect.Height()/2 + cal.CenterOffsetY
}

func ComputeStep(cal CalibrationPayload, player, target Point) StepResult {
	cal = NormalizeCalibration(cal)
	dx := target.X - player.X
	dy := target.Y - player.Y
	distance := math.Hypot(dx, dy)

	mode := StepModeMove
	if distance <= float64(cal.InteractThresholdG) {
		mode = StepModeInteract
	}
	return computeStepWithMode(cal, dx, dy, distance, mode)
}

func ComputeStepForMode(cal CalibrationPayload, player, target Point, mode StepMode) StepResult {
	cal = NormalizeCalibration(cal)
	dx := target.X - player.X
	dy := target.Y - player.Y
	distance := math.Hypot(dx, dy)
	return computeStepWithMode(cal, dx, dy, distance, mode)
}

func computeStepWithMode(cal CalibrationPayload, dx, dy, distance float64, mode StepMode) StepResult {
	// Match the renderer orientation from DrawingUtils.transformPoint. Using
	// isoAxis keeps the rotated vector magnitude equal to the world-space
	// distance, which lets us scale movement in pixels per game unit.
	screenDX := (dx + dy) * isoAxis
	screenDY := (dx - dy) * isoAxis

	centerX, centerY := computeCenter(cal)
	leftBound := cal.WindowRect.Left + cal.SafeMarginX
	rightBound := cal.WindowRect.Right - cal.SafeMarginX
	topBound := cal.WindowRect.Top + cal.SafeMarginY
	bottomBound := cal.WindowRect.Bottom - cal.SafeMarginY

	offsetX, offsetY := computeProjectedOffset(cal, mode, distance, screenDX, screenDY)

	// If the target lands outside the safe rectangle, shrink the vector
	// uniformly so the direction stays intact.
	offsetX, offsetY = clampVector(offsetX, offsetY, centerX, centerY, leftBound, rightBound, topBound, bottomBound)

	clickX := centerX + int(math.Round(offsetX))
	clickY := centerY + int(math.Round(offsetY))

	clickX = clamp(clickX, leftBound, rightBound)
	clickY = clamp(clickY, topBound, bottomBound)

	message := "movement click dispatched"
	if mode == StepModeInteract {
		message = "interaction click dispatched"
	}

	return StepResult{
		Mode:     mode,
		ClickX:   clickX,
		ClickY:   clickY,
		Distance: distance,
		Message:  message,
	}
}

func computeProjectedOffset(cal CalibrationPayload, mode StepMode, distance, screenDX, screenDY float64) (outX float64, outY float64) {
	threshold := math.Max(float64(cal.InteractThresholdG), 1)

	// Interact mode: click where the resource actually renders on screen, not
	// at a fixed radius. Using the same pixels-per-game-unit scale as move mode
	// puts the click on the sprite, which is what Albion needs to register a
	// harvest. A fixed 80px radius either overshot the sprite (tree trunk) or
	// landed on empty ground next to it. We still enforce a minimum magnitude
	// so the cursor doesn't land on the player themselves when they're standing
	// right on top of the node (in which case any small offset toward the node
	// direction works — the sprite's hitbox is forgiving at point-blank range).
	if mode == StepModeInteract {
		pixelsPerGameUnit := float64(cal.InteractRadiusPx) / threshold
		if pixelsPerGameUnit <= 0 {
			pixelsPerGameUnit = 1
		}
		offX := screenDX * pixelsPerGameUnit
		offY := screenDY * pixelsPerGameUnit

		// Floor the magnitude at ~1/3 of InteractRadiusPx so we never right-click
		// the player's own sprite when the projected delta rounds to near zero.
		minMag := math.Max(float64(cal.InteractRadiusPx)/3, 24)
		if mag := math.Hypot(offX, offY); mag < minMag {
			if mag == 0 {
				// No directional signal at all — nudge up-right (default iso
				// "away from player" direction) so the click leaves the body.
				return minMag * isoAxis, -minMag * isoAxis
			}
			scale := minMag / mag
			offX *= scale
			offY *= scale
		}
		return offX, offY
	}

	// Move mode: keep the cursor in a controlled ring around the player's
	// screen position instead of projecting all the way toward the target sprite.
	// That keeps travel clicks in the walkable world around the avatar and away
	// from HUD elements like the mount button. As the target gets close (but is
	// still outside interact range), shrink the travel radius so we approach the
	// node smoothly before switching to the precise interact click.
	if distance == 0 {
		return 0, 0
	}

	moveRadius := computeMoveClickRadius(cal, distance, threshold)
	scale := moveRadius / distance
	offX := screenDX * scale
	offY := screenDY * scale

	return offX, offY
}

func computeMoveClickRadius(cal CalibrationPayload, distance, threshold float64) float64 {
	maxRadius := math.Max(float64(cal.MoveRadiusPx), 1)
	// Keep travel clicks far from the player for most of the approach so the
	// character commits to movement instead of "micro-stepping" around their own
	// feet. We only compress the radius a little bit when we're just outside the
	// interact handoff.
	minRadius := math.Max(maxRadius*0.72, math.Max(float64(cal.InteractRadiusPx)*1.35, 96))
	if minRadius > maxRadius {
		minRadius = maxRadius
	}

	// Only the area immediately outside interact range uses the reduced travel
	// radius. Once we're modestly farther away, push all the way to MoveRadiusPx.
	rampDistance := math.Max(threshold*0.75, 1)
	progress := (distance - threshold) / rampDistance
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	return minRadius + ((maxRadius - minRadius) * progress)
}

// clampVector scales (dx, dy) down uniformly so that (centerX+dx, centerY+dy)
// lies within [left,right] x [top,bottom]. Preserves direction; returns (0,0) if
// the center is already outside the rect (caller's final clamp still applies).
func clampVector(dx, dy float64, centerX, centerY, left, right, top, bottom int) (outDx float64, outDy float64) {
	scale := 1.0
	if dx > 0 {
		if limit := float64(right - centerX); dx > limit && limit > 0 {
			if s := limit / dx; s < scale {
				scale = s
			}
		}
	} else if dx < 0 {
		if limit := float64(centerX - left); -dx > limit && limit > 0 {
			if s := limit / -dx; s < scale {
				scale = s
			}
		}
	}
	if dy > 0 {
		if limit := float64(bottom - centerY); dy > limit && limit > 0 {
			if s := limit / dy; s < scale {
				scale = s
			}
		}
	} else if dy < 0 {
		if limit := float64(centerY - top); -dy > limit && limit > 0 {
			if s := limit / -dy; s < scale {
				scale = s
			}
		}
	}
	if scale < 0 {
		scale = 0
	}
	return dx * scale, dy * scale
}

func clamp(value, minValue, maxValue int) int {
	if maxValue < minValue {
		return value
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
