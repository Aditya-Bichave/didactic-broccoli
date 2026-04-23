//go:build !windows

package gather

import (
	"context"
	"errors"
)

type unsupportedExecutor struct{}

func NewPlatformExecutor() Executor {
	return unsupportedExecutor{}
}

func (unsupportedExecutor) Supported() bool {
	return false
}

func (unsupportedExecutor) Platform() string {
	return "unsupported"
}

func (unsupportedExecutor) CaptureCalibration(context.Context, CalibrationPayload) (CalibrationPayload, error) {
	return CalibrationPayload{}, errors.New("gather automation is only available on Windows")
}

func (unsupportedExecutor) FocusedWindow(context.Context, CalibrationPayload) (bool, error) {
	return false, nil
}

func (unsupportedExecutor) ExecuteStep(context.Context, CalibrationPayload, *StepResult) error {
	return errors.New("gather automation is only available on Windows")
}

func (unsupportedExecutor) Release(context.Context) error {
	return nil
}
