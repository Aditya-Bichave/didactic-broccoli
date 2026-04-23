package gather

import "context"

type Executor interface {
	Supported() bool
	Platform() string
	CaptureCalibration(context.Context, CalibrationPayload) (CalibrationPayload, error)
	FocusedWindow(context.Context, CalibrationPayload) (bool, error)
	ExecuteStep(context.Context, CalibrationPayload, *StepResult) error
	// Release lifts any held mouse button (from a HoldMove step). Called when
	// the gather loop stops so the character isn't left walking forever.
	Release(context.Context) error
}
