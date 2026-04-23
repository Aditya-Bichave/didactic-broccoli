package gather

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/nospy/albion-openradar/internal/logger"
)

const maxConsecutiveExecutorErrors = 3

const (
	interactExitHysteresisGu = 8.0
	mountDistanceThresholdGu = 40.0
	mountedInteractScale     = 0.6
	mountedInteractMinGu     = 6.0
	remountCooldown          = 2 * time.Second
)

type Service struct {
	executor Executor
	logger   *logger.Logger

	mu              sync.RWMutex
	active          bool
	state           RunState
	calibration     *CalibrationPayload
	currentTarget   *GatherTarget
	lastError       string
	lastAction      string
	consecutiveErrs int
	mountStateKnown bool
	assumedMounted  bool
	interactLocked  bool
	interactTarget  int64
	lastDismountAt  time.Time
}

func NewService(log *logger.Logger, executor Executor) *Service {
	return &Service{
		executor: executor,
		logger:   log,
		state:    RunStateIdle,
	}
}

func (s *Service) Status(ctx context.Context) GatherStatusResponse {
	s.mu.RLock()
	status := s.snapshotLocked()
	calibration := status.Calibration
	s.mu.RUnlock()

	if calibration != nil {
		focused, err := s.executor.FocusedWindow(ctx, *calibration)
		if err != nil {
			status.FocusedWindow = false
			if status.LastError == "" {
				status.LastError = err.Error()
			}
			return status
		}
		status.FocusedWindow = focused
	}

	return status
}

func (s *Service) Toggle(ctx context.Context, active bool) (GatherStatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.executor.Supported() {
		return s.snapshotLocked(), errors.New("gather automation is unsupported on this platform")
	}

	if active {
		if s.calibration == nil {
			s.state = RunStateError
			s.lastError = "calibration required before starting gather mode"
			return s.snapshotLocked(), errors.New(s.lastError)
		}

		calibration, err := s.executor.CaptureCalibration(ctx, *s.calibration)
		if err != nil {
			s.state = RunStateError
			s.lastError = err.Error()
			return s.snapshotLocked(), err
		}
		calibration = NormalizeCalibration(calibration)
		s.calibration = &calibration

		focused, err := s.executor.FocusedWindow(ctx, calibration)
		if err != nil {
			s.state = RunStateError
			s.lastError = err.Error()
			return s.snapshotLocked(), err
		}
		if !focused {
			s.state = RunStateError
			s.lastError = "Albion window must be focused before starting gather mode"
			return s.snapshotLocked(), errors.New(s.lastError)
		}

		s.active = true
		s.state = RunStateRunning
		s.lastError = ""
		s.lastAction = "gather mode started"
		s.consecutiveErrs = 0
		s.resetRuntimeLocked()
		return s.snapshotLocked(), nil
	}

	_ = s.executor.Release(context.Background())
	s.active = false
	s.state = RunStateStopped
	s.lastAction = "gather mode stopped"
	s.consecutiveErrs = 0
	s.resetRuntimeLocked()
	return s.snapshotLocked(), nil
}

func (s *Service) Calibrate(ctx context.Context, input CalibrationPayload) (GatherStatusResponse, error) {
	s.mu.Lock()
	s.state = RunStateCalibrating
	s.mu.Unlock()

	calibration, err := s.executor.CaptureCalibration(ctx, input)
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state = RunStateError
		s.lastError = err.Error()
		return s.snapshotLocked(), err
	}

	calibration = NormalizeCalibration(calibration)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.calibration = &calibration
	s.lastError = ""
	s.lastAction = "gather calibration saved"
	if s.active {
		s.state = RunStateRunning
	} else {
		s.state = RunStateIdle
	}
	return s.snapshotLocked(), nil
}

func (s *Service) ExecuteTarget(ctx context.Context, req GatherCommandRequest) (GatherCommandResponse, error) {
	s.mu.Lock()
	if !s.active {
		status := s.snapshotLocked()
		s.mu.Unlock()
		return GatherCommandResponse{Status: status}, errors.New("gather mode is not active")
	}
	if s.calibration == nil {
		s.state = RunStateError
		s.lastError = "calibration required before executing gather steps"
		status := s.snapshotLocked()
		s.mu.Unlock()
		return GatherCommandResponse{Status: status}, errors.New(s.lastError)
	}
	if req.Player == nil || req.Target == nil {
		status := s.snapshotLocked()
		s.mu.Unlock()
		return GatherCommandResponse{Status: status}, errors.New("player and target payloads are required")
	}

	calibration := *s.calibration
	target := *req.Target
	mounted := s.mountStateKnown && s.assumedMounted
	if req.IsMounted {
		mounted = true
		s.mountStateKnown = true
		s.assumedMounted = true
	}
	interactLocked := s.interactLocked
	interactTarget := s.interactTarget
	lastDismountAt := s.lastDismountAt
	s.currentTarget = &target
	s.mu.Unlock()

	calibration = NormalizeCalibration(calibration)

	focused, err := s.executor.FocusedWindow(ctx, calibration)
	if err != nil {
		return s.failExecutionLocked(err)
	}
	if !focused {
		return s.failExecutionLocked(errors.New("Albion window focus lost"))
	}

	rawStep := ComputeStep(calibration, *req.Player, req.Target.Position)
	mode, keepInteractLocked := resolveStepMode(
		req.Target.ID,
		rawStep.Distance,
		float64(calibration.InteractThresholdG),
		mounted,
		interactLocked,
		interactTarget,
	)
	step := rawStep
	if mode != rawStep.Mode {
		step = ComputeStepForMode(calibration, *req.Player, req.Target.Position, mode)
	}
	step.FocusedWindow = true
	plannedClickX := step.ClickX
	plannedClickY := step.ClickY

	// Mount state machine. Runs *before* the click is sent:
	//   - If we want to move and we're not mounted (and user opted in), tap the
	//     mount key and skip this tick's click. The next tick drives movement.
	//   - If we want to interact but we are mounted, dismount first; again skip
	//     the click so the interact click lands after the animation.
	//   - Otherwise: send the click as computed.
	// Doing mount-then-click in the same tick risks the click arriving during
	// the mount animation, which Albion drops on the floor.
	nextMounted := mounted
	if calibration.DesiredMount &&
		step.Mode == StepModeMove &&
		!mounted &&
		step.Distance >= mountDistanceThresholdGu &&
		time.Since(lastDismountAt) >= remountCooldown {
		step.KeyTap = strings.ToUpper(calibration.MountKey)
		step.SkipClick = true
		step.ReleaseHold = true
		step.Message = "mounting up"
		nextMounted = true
	} else if step.Mode == StepModeInteract && mounted {
		step.KeyTap = strings.ToUpper(calibration.MountKey)
		step.SkipClick = true
		step.ReleaseHold = true
		step.Message = "dismounting to interact"
		nextMounted = false
	}

	// Travel uses press-and-hold right-click: the executor presses the button
	// on the first Move step, keeps it held across ticks, and releases on any
	// non-Move step (interact, mount, stop). Albion treats held right-click as
	// "follow cursor," giving smooth continuous travel with no per-tick
	// restart. This replaces the old per-click throttle — there is nothing to
	// throttle when we only press the button once.
	if step.Mode == StepModeMove && !step.SkipClick && step.KeyTap == "" {
		step.HoldMove = true
		step.Message = "travel (right-click held)"
	} else {
		step.ReleaseHold = true
	}

	if s.logger != nil {
		s.logger.Debug("gather", "step_decision", map[string]interface{}{
			"targetId":                 req.Target.ID,
			"targetType":               req.Target.Type,
			"playerX":                  req.Player.X,
			"playerY":                  req.Player.Y,
			"targetX":                  req.Target.Position.X,
			"targetY":                  req.Target.Position.Y,
			"distance":                 step.Distance,
			"mounted":                  mounted,
			"interactLocked":           interactLocked,
			"interactTarget":           interactTarget,
			"rawMode":                  rawStep.Mode,
			"resolvedMode":             step.Mode,
			"interactThresholdGu":      float64(calibration.InteractThresholdG),
			"mountedInteractThreshold": mountedInteractThreshold(float64(calibration.InteractThresholdG)),
			"plannedClickX":            plannedClickX,
			"plannedClickY":            plannedClickY,
			"skipClick":                step.SkipClick,
			"keyTap":                   step.KeyTap,
			"holdMove":                 step.HoldMove,
			"releaseHold":              step.ReleaseHold,
			"message":                  step.Message,
		}, nil)
	}

	if err := s.executor.ExecuteStep(ctx, calibration, &step); err != nil {
		return s.failExecutionLocked(err)
	}

	if s.logger != nil {
		s.logger.Debug("gather", "step_executed", map[string]interface{}{
			"targetId":      req.Target.ID,
			"distance":      step.Distance,
			"mode":          step.Mode,
			"mounted":       mounted,
			"plannedClickX": plannedClickX,
			"plannedClickY": plannedClickY,
			"finalClickX":   step.ClickX,
			"finalClickY":   step.ClickY,
			"visionAdjusted": plannedClickX != step.ClickX ||
				plannedClickY != step.ClickY,
			"skipClick":   step.SkipClick,
			"keyTap":      step.KeyTap,
			"holdMove":    step.HoldMove,
			"releaseHold": step.ReleaseHold,
			"message":     step.Message,
		}, nil)
	}

	// Block here (not holding the service lock) so the caller's retry cadence
	// naturally skips the animation window and avoids double mount toggles.
	if step.SkipClick && step.KeyTap != "" {
		if step.Mode == StepModeMove {
			time.Sleep(time.Duration(calibration.MountDelayMs) * time.Millisecond)
		} else {
			time.Sleep(time.Duration(calibration.DismountDelayMs) * time.Millisecond)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.calibration = &calibration
	s.consecutiveErrs = 0
	s.lastError = ""
	if req.IsMounted || step.KeyTap != "" {
		s.mountStateKnown = true
		s.assumedMounted = nextMounted
	}
	s.interactLocked = keepInteractLocked
	if keepInteractLocked {
		s.interactTarget = req.Target.ID
	} else {
		s.interactTarget = 0
	}
	if step.Mode == StepModeInteract && step.KeyTap != "" {
		s.lastDismountAt = time.Now()
	}
	switch {
	case step.SkipClick && step.KeyTap != "":
		s.lastAction = fmt.Sprintf("%s (target %d)", step.Message, req.Target.ID)
	default:
		s.lastAction = fmt.Sprintf("%s target %d", step.Mode, req.Target.ID)
	}
	s.state = RunStateRunning
	status := s.snapshotLocked()
	status.FocusedWindow = true
	return GatherCommandResponse{Status: status, Step: &step}, nil
}

// ExecuteTest fires a single movement click toward one of the four cardinal
// world-space directions (N/E/S/W). Used by the UI's debug "test click"
// buttons to verify the projection is aligned with the radar render. Does not
// need the run loop to be active; only requires calibration.
func (s *Service) ExecuteTest(ctx context.Context, direction string) (GatherCommandResponse, error) {
	s.mu.Lock()
	if s.calibration == nil {
		status := s.snapshotLocked()
		s.mu.Unlock()
		return GatherCommandResponse{Status: status}, errors.New("calibration required before test click")
	}
	calibration := *s.calibration
	s.mu.Unlock()

	calibration, err := s.executor.CaptureCalibration(ctx, calibration)
	if err != nil {
		return s.failExecutionLocked(err)
	}
	calibration = NormalizeCalibration(calibration)

	focused, err := s.executor.FocusedWindow(ctx, calibration)
	if err != nil {
		return s.failExecutionLocked(err)
	}
	if !focused {
		return s.failExecutionLocked(errors.New("Albion window must be focused for test click"))
	}

	var target Point
	switch strings.ToUpper(strings.TrimSpace(direction)) {
	case "N":
		target = Point{X: 0, Y: 100}
	case "S":
		target = Point{X: 0, Y: -100}
	case "E":
		target = Point{X: 100, Y: 0}
	case "W":
		target = Point{X: -100, Y: 0}
	default:
		return GatherCommandResponse{Status: s.Status(ctx)}, fmt.Errorf("unknown test direction %q", direction)
	}

	step := ComputeStep(calibration, Point{X: 0, Y: 0}, target)
	step.FocusedWindow = true
	step.Message = "test click (" + strings.ToUpper(direction) + ")"

	if err := s.executor.ExecuteStep(ctx, calibration, &step); err != nil {
		return s.failExecutionLocked(err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.calibration = &calibration
	s.lastError = ""
	s.lastAction = step.Message
	status := s.snapshotLocked()
	status.FocusedWindow = true
	return GatherCommandResponse{Status: status, Step: &step}, nil
}

func (s *Service) Stop(reason string) GatherStatusResponse {
	_ = s.executor.Release(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = false
	if reason != "" {
		s.lastAction = reason
	} else {
		s.lastAction = "gather mode stopped"
	}
	s.state = RunStateStopped
	s.consecutiveErrs = 0
	s.resetRuntimeLocked()
	return s.snapshotLocked()
}

func (s *Service) failExecutionLocked(err error) (GatherCommandResponse, error) {
	_ = s.executor.Release(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()

	s.consecutiveErrs++
	s.lastError = err.Error()
	s.lastAction = "gather worker error"
	s.state = RunStateError

	if s.consecutiveErrs >= maxConsecutiveExecutorErrors {
		s.active = false
		s.lastAction = "gather worker stopped after repeated errors"
		s.resetRuntimeLocked()
	}

	status := s.snapshotLocked()
	return GatherCommandResponse{Status: status}, err
}

func (s *Service) resetRuntimeLocked() {
	s.currentTarget = nil
	s.mountStateKnown = false
	s.assumedMounted = false
	s.interactLocked = false
	s.interactTarget = 0
	s.lastDismountAt = time.Time{}
}

func mountedInteractThreshold(threshold float64) float64 {
	enterThreshold := math.Max(threshold, 1)
	scaled := enterThreshold * mountedInteractScale
	if scaled < mountedInteractMinGu {
		scaled = mountedInteractMinGu
	}
	if scaled > enterThreshold {
		scaled = enterThreshold
	}
	return scaled
}

func resolveStepMode(targetID int64, distance, threshold float64, mounted, interactLocked bool, interactTarget int64) (StepMode, bool) {
	enterThreshold := math.Max(threshold, 1)
	if mounted {
		enterThreshold = mountedInteractThreshold(enterThreshold)
	}
	exitThreshold := enterThreshold + interactExitHysteresisGu

	if interactLocked && interactTarget == targetID {
		if distance <= exitThreshold {
			return StepModeInteract, true
		}
		return StepModeMove, false
	}

	if distance <= enterThreshold {
		return StepModeInteract, true
	}

	return StepModeMove, false
}

func (s *Service) snapshotLocked() GatherStatusResponse {
	status := GatherStatusResponse{
		Supported:      s.executor.Supported(),
		Platform:       s.executor.Platform(),
		Active:         s.active,
		State:          s.state,
		Calibrated:     s.calibration != nil,
		FocusedWindow:  false,
		LastError:      s.lastError,
		LastAction:     s.lastAction,
		ConsecutiveErr: s.consecutiveErrs,
	}

	if s.currentTarget != nil {
		targetCopy := *s.currentTarget
		status.CurrentTarget = &targetCopy
	}
	if s.calibration != nil {
		calibrationCopy := *s.calibration
		status.Calibration = &calibrationCopy
	}

	return status
}
