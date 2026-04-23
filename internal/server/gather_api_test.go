package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/require"

	"github.com/nospy/albion-openradar/internal/gather"
)

type apiFakeExecutor struct {
	supported   bool
	focused     bool
	calibration gather.CalibrationPayload
}

func (f *apiFakeExecutor) Supported() bool {
	return f.supported
}

func (f *apiFakeExecutor) Platform() string {
	return "test"
}

func (f *apiFakeExecutor) CaptureCalibration(context.Context, gather.CalibrationPayload) (gather.CalibrationPayload, error) {
	return f.calibration, nil
}

func (f *apiFakeExecutor) FocusedWindow(context.Context, gather.CalibrationPayload) (bool, error) {
	return f.focused, nil
}

func (f *apiFakeExecutor) ExecuteStep(context.Context, gather.CalibrationPayload, *gather.StepResult) error {
	return nil
}

func (f *apiFakeExecutor) Release(context.Context) error {
	return nil
}

func TestGatherAPIStatus_ReportsUnsupportedPlatform(t *testing.T) {
	service := gather.NewService(nil, &apiFakeExecutor{supported: false})
	handler := newGatherAPIHandler(service, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/gather/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	resp := gather.GatherStatusResponse{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.False(t, resp.Supported)
}

func TestGatherAPIToggle_StartsSupportedSession(t *testing.T) {
	executor := &apiFakeExecutor{
		supported:   true,
		focused:     true,
		calibration: gather.CalibrationPayload{WindowRect: gather.WindowRect{Left: 1, Top: 2, Right: 300, Bottom: 400}},
	}
	service := gather.NewService(nil, executor)
	_, err := service.Calibrate(context.Background(), gather.CalibrationPayload{})
	require.NoError(t, err)

	handler := newGatherAPIHandler(service, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/gather/toggle", strings.NewReader(`{"active":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	resp := gather.GatherCommandResponse{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Status.Active)
	require.Equal(t, gather.RunStateRunning, resp.Status.State)
}
