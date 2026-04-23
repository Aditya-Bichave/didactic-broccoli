package gather

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeExecutor struct {
	supported      bool
	focused        bool
	focusedErr     error
	calibration    CalibrationPayload
	calibrationErr error
	executeErr     error
	executeCalls   int
	captureCalls   int
	lastStep       StepResult
	releaseCalls   int
}

func (f *fakeExecutor) Supported() bool {
	return f.supported
}

func (f *fakeExecutor) Platform() string {
	return "test"
}

func (f *fakeExecutor) CaptureCalibration(context.Context, CalibrationPayload) (CalibrationPayload, error) {
	f.captureCalls++
	if f.calibrationErr != nil {
		return CalibrationPayload{}, f.calibrationErr
	}
	return f.calibration, nil
}

func (f *fakeExecutor) FocusedWindow(context.Context, CalibrationPayload) (bool, error) {
	return f.focused, f.focusedErr
}

func (f *fakeExecutor) ExecuteStep(_ context.Context, _ CalibrationPayload, step *StepResult) error {
	f.executeCalls++
	f.lastStep = *step
	return f.executeErr
}

func (f *fakeExecutor) Release(context.Context) error {
	f.releaseCalls++
	return nil
}

func TestServiceStatus_UnsupportedExecutor(t *testing.T) {
	svc := NewService(nil, &fakeExecutor{supported: false})

	status := svc.Status(context.Background())

	require.False(t, status.Supported)
	require.Equal(t, RunStateIdle, status.State)
}

func TestServiceToggle_RequiresCalibration(t *testing.T) {
	svc := NewService(nil, &fakeExecutor{supported: true, focused: true})

	_, err := svc.Toggle(context.Background(), true)

	require.Error(t, err)
	require.Contains(t, err.Error(), "calibration required")
}

func TestServiceCalibrate_PersistsCalibration(t *testing.T) {
	executor := &fakeExecutor{
		supported:   true,
		focused:     true,
		calibration: CalibrationPayload{WindowRect: WindowRect{Left: 1, Top: 2, Right: 3, Bottom: 4}},
	}
	svc := NewService(nil, executor)

	status, err := svc.Calibrate(context.Background(), CalibrationPayload{})

	require.NoError(t, err)
	require.True(t, status.Calibrated)
	require.NotNil(t, status.Calibration)
	require.Equal(t, 220, status.Calibration.MoveRadiusPx)
	require.Equal(t, 1, executor.captureCalls)
}

func TestServiceToggle_RefreshesCalibrationOnStart(t *testing.T) {
	executor := &fakeExecutor{
		supported:   true,
		focused:     true,
		calibration: CalibrationPayload{WindowRect: WindowRect{Left: 10, Top: 10, Right: 200, Bottom: 200}},
	}
	svc := NewService(nil, executor)

	_, err := svc.Calibrate(context.Background(), CalibrationPayload{})
	require.NoError(t, err)

	status, err := svc.Toggle(context.Background(), true)

	require.NoError(t, err)
	require.True(t, status.Active)
	require.Equal(t, 2, executor.captureCalls)
}

func TestServiceExecuteTarget_StopsAfterRepeatedErrors(t *testing.T) {
	executor := &fakeExecutor{
		supported:   true,
		focused:     true,
		calibration: CalibrationPayload{WindowRect: WindowRect{Left: 10, Top: 20, Right: 500, Bottom: 600}},
		executeErr:  errors.New("click failed"),
	}
	svc := NewService(nil, executor)

	_, err := svc.Calibrate(context.Background(), CalibrationPayload{})
	require.NoError(t, err)
	_, err = svc.Toggle(context.Background(), true)
	require.NoError(t, err)

	req := GatherCommandRequest{
		Player: &Point{X: 0, Y: 0},
		Target: &GatherTarget{
			ID:       99,
			Type:     "Fiber",
			Position: Point{X: 40, Y: 20},
		},
	}

	for i := 0; i < maxConsecutiveExecutorErrors; i++ {
		_, err = svc.ExecuteTarget(context.Background(), req)
		require.Error(t, err)
	}

	status := svc.Status(context.Background())
	require.False(t, status.Active)
	require.Equal(t, RunStateError, status.State)
	require.Equal(t, maxConsecutiveExecutorErrors, status.ConsecutiveErr)
}

func TestServiceExecuteTarget_ReturnsStepOnSuccess(t *testing.T) {
	executor := &fakeExecutor{
		supported: true,
		focused:   true,
		calibration: CalibrationPayload{
			WindowRect:      WindowRect{Left: 10, Top: 20, Right: 500, Bottom: 600},
			DesiredMount:    true,
			MountDelayMs:    1,
			DismountDelayMs: 1,
		},
	}
	svc := NewService(nil, executor)

	_, err := svc.Calibrate(context.Background(), CalibrationPayload{})
	require.NoError(t, err)
	_, err = svc.Toggle(context.Background(), true)
	require.NoError(t, err)

	resp, err := svc.ExecuteTarget(context.Background(), GatherCommandRequest{
		Player: &Point{X: 0, Y: 0},
		Target: &GatherTarget{
			ID:       321,
			Type:     "Ore",
			Position: Point{X: 60, Y: 0},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Step)
	require.Equal(t, StepModeMove, resp.Step.Mode)
	require.Equal(t, 1, executor.executeCalls)
	require.Equal(t, 2, executor.captureCalls)
}

func TestServiceExecuteTarget_UsesAssumedMountStateForInteract(t *testing.T) {
	executor := &fakeExecutor{
		supported: true,
		focused:   true,
		calibration: CalibrationPayload{
			WindowRect:      WindowRect{Left: 10, Top: 20, Right: 500, Bottom: 600},
			DesiredMount:    true,
			MountDelayMs:    1,
			DismountDelayMs: 1,
		},
	}
	svc := NewService(nil, executor)

	_, err := svc.Calibrate(context.Background(), CalibrationPayload{})
	require.NoError(t, err)
	_, err = svc.Toggle(context.Background(), true)
	require.NoError(t, err)

	resp, err := svc.ExecuteTarget(context.Background(), GatherCommandRequest{
		Player: &Point{X: 0, Y: 0},
		Target: &GatherTarget{
			ID:       501,
			Type:     "Ore",
			Position: Point{X: 60, Y: 0},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Step)
	require.True(t, resp.Step.SkipClick)
	require.Equal(t, "A", resp.Step.KeyTap)
	require.Equal(t, "mounting up", resp.Step.Message)

	resp, err = svc.ExecuteTarget(context.Background(), GatherCommandRequest{
		Player: &Point{X: 0, Y: 0},
		Target: &GatherTarget{
			ID:       501,
			Type:     "Ore",
			Position: Point{X: 1, Y: 1},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Step)
	require.Equal(t, StepModeInteract, resp.Step.Mode)
	require.True(t, resp.Step.SkipClick)
	require.Equal(t, "A", resp.Step.KeyTap)
	require.Equal(t, "dismounting to interact", resp.Step.Message)
}

func TestServiceExecuteTarget_DelaysMountedInteractUntilCloser(t *testing.T) {
	executor := &fakeExecutor{
		supported: true,
		focused:   true,
		calibration: CalibrationPayload{
			WindowRect:      WindowRect{Left: 10, Top: 20, Right: 500, Bottom: 600},
			DesiredMount:    true,
			MountDelayMs:    1,
			DismountDelayMs: 1,
		},
	}
	svc := NewService(nil, executor)

	_, err := svc.Calibrate(context.Background(), CalibrationPayload{})
	require.NoError(t, err)
	_, err = svc.Toggle(context.Background(), true)
	require.NoError(t, err)

	resp, err := svc.ExecuteTarget(context.Background(), GatherCommandRequest{
		Player:    &Point{X: 0, Y: 0},
		IsMounted: true,
		Target: &GatherTarget{
			ID:       601,
			Type:     "Fiber",
			Position: Point{X: 10, Y: 0},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Step)
	require.Equal(t, StepModeMove, resp.Step.Mode)
	require.False(t, resp.Step.SkipClick)
	require.Empty(t, resp.Step.KeyTap)
	require.True(t, resp.Step.HoldMove)
}

func TestServiceExecuteTarget_KeepsInteractModeUntilExitThreshold(t *testing.T) {
	executor := &fakeExecutor{
		supported: true,
		focused:   true,
		calibration: CalibrationPayload{
			WindowRect:      WindowRect{Left: 10, Top: 20, Right: 500, Bottom: 600},
			DesiredMount:    true,
			MountDelayMs:    1,
			DismountDelayMs: 1,
		},
	}
	svc := NewService(nil, executor)

	_, err := svc.Calibrate(context.Background(), CalibrationPayload{})
	require.NoError(t, err)
	_, err = svc.Toggle(context.Background(), true)
	require.NoError(t, err)

	_, err = svc.ExecuteTarget(context.Background(), GatherCommandRequest{
		Player: &Point{X: 0, Y: 0},
		Target: &GatherTarget{
			ID:       777,
			Type:     "Ore",
			Position: Point{X: 60, Y: 0},
		},
	})
	require.NoError(t, err)

	resp, err := svc.ExecuteTarget(context.Background(), GatherCommandRequest{
		Player:    &Point{X: 0, Y: 0},
		IsMounted: true,
		Target: &GatherTarget{
			ID:       777,
			Type:     "Ore",
			Position: Point{X: 8, Y: 0},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Step)
	require.Equal(t, StepModeInteract, resp.Step.Mode)
	require.True(t, resp.Step.SkipClick)
	require.Equal(t, "dismounting to interact", resp.Step.Message)

	resp, err = svc.ExecuteTarget(context.Background(), GatherCommandRequest{
		Player: &Point{X: 0, Y: 0},
		Target: &GatherTarget{
			ID:       777,
			Type:     "Ore",
			Position: Point{X: 18, Y: 0},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Step)
	require.Equal(t, StepModeInteract, resp.Step.Mode)
	require.False(t, resp.Step.SkipClick)
	require.Empty(t, resp.Step.KeyTap)
}

func TestServiceExecuteTarget_DoesNotRemountImmediatelyAfterDismount(t *testing.T) {
	executor := &fakeExecutor{
		supported: true,
		focused:   true,
		calibration: CalibrationPayload{
			WindowRect:      WindowRect{Left: 10, Top: 20, Right: 500, Bottom: 600},
			DesiredMount:    true,
			MountDelayMs:    1,
			DismountDelayMs: 1,
		},
	}
	svc := NewService(nil, executor)

	_, err := svc.Calibrate(context.Background(), CalibrationPayload{})
	require.NoError(t, err)
	_, err = svc.Toggle(context.Background(), true)
	require.NoError(t, err)

	_, err = svc.ExecuteTarget(context.Background(), GatherCommandRequest{
		Player: &Point{X: 0, Y: 0},
		Target: &GatherTarget{
			ID:       888,
			Type:     "Ore",
			Position: Point{X: 60, Y: 0},
		},
	})
	require.NoError(t, err)

	_, err = svc.ExecuteTarget(context.Background(), GatherCommandRequest{
		Player:    &Point{X: 0, Y: 0},
		IsMounted: true,
		Target: &GatherTarget{
			ID:       888,
			Type:     "Ore",
			Position: Point{X: 8, Y: 0},
		},
	})
	require.NoError(t, err)

	resp, err := svc.ExecuteTarget(context.Background(), GatherCommandRequest{
		Player: &Point{X: 0, Y: 0},
		Target: &GatherTarget{
			ID:       889,
			Type:     "Ore",
			Position: Point{X: 60, Y: 0},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Step)
	require.Equal(t, StepModeMove, resp.Step.Mode)
	require.False(t, resp.Step.SkipClick)
	require.Empty(t, resp.Step.KeyTap)
	require.True(t, resp.Step.HoldMove)
}

func TestServiceStop_ReleasesHeldMovement(t *testing.T) {
	executor := &fakeExecutor{supported: true}
	svc := NewService(nil, executor)

	svc.Stop("manual stop")

	require.Equal(t, 1, executor.releaseCalls)
}

func TestMountAnimationDefaults_AreConservativeForSlowMounts(t *testing.T) {
	cal := NormalizeCalibration(CalibrationPayload{})
	require.GreaterOrEqual(t, time.Duration(cal.MountDelayMs)*time.Millisecond, 2*time.Second)
	require.GreaterOrEqual(t, time.Duration(cal.DismountDelayMs)*time.Millisecond, 900*time.Millisecond)
}
