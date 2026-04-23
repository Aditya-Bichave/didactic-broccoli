package server

import (
	"errors"
	"io"
	"net/http"

	"github.com/segmentio/encoding/json"

	"github.com/nospy/albion-openradar/internal/gather"
	"github.com/nospy/albion-openradar/internal/logger"
)

func newGatherAPIHandler(service *gather.Service, log *logger.Logger) http.Handler {
	mux := http.NewServeMux()

	writeJSON := func(w http.ResponseWriter, statusCode int, payload interface{}) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(payload)
	}

	decodeBody := func(r *http.Request) (gather.GatherCommandRequest, error) {
		req := gather.GatherCommandRequest{}
		if r.Body == nil {
			return req, nil
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if errors.Is(err, http.ErrBodyNotAllowed) || errors.Is(err, io.EOF) {
				return req, nil
			}
			return req, err
		}
		return req, nil
	}

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, service.Status(r.Context()))
	})

	mux.HandleFunc("/toggle", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, err := decodeBody(r)
		if err != nil {
			http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		active := true
		if req.Active != nil {
			active = *req.Active
		}
		status, err := service.Toggle(r.Context(), active)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gather.GatherCommandResponse{Status: status})
			return
		}
		writeJSON(w, http.StatusOK, gather.GatherCommandResponse{Status: status})
	})

	mux.HandleFunc("/calibrate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, err := decodeBody(r)
		if err != nil {
			http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		payload := gather.CalibrationPayload{}
		if req.Calibration != nil {
			payload = *req.Calibration
		}
		status, err := service.Calibrate(r.Context(), payload)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gather.GatherCommandResponse{Status: status})
			return
		}
		writeJSON(w, http.StatusOK, gather.GatherCommandResponse{Status: status})
	})

	mux.HandleFunc("/target", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, err := decodeBody(r)
		if err != nil {
			http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		resp, err := service.ExecuteTarget(r.Context(), req)
		if err != nil {
			statusCode := http.StatusBadRequest
			if log != nil {
				log.Warn("gather", "target_error", map[string]interface{}{
					"error": err.Error(),
				}, nil)
			}
			writeJSON(w, statusCode, resp)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("/test-click", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, err := decodeBody(r)
		if err != nil {
			http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		resp, err := service.ExecuteTest(r.Context(), req.TestDirection)
		if err != nil {
			if log != nil {
				log.Warn("gather", "test_click_error", map[string]interface{}{
					"error":     err.Error(),
					"direction": req.TestDirection,
				}, nil)
			}
			writeJSON(w, http.StatusBadRequest, resp)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, err := decodeBody(r)
		if err != nil {
			http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		status := service.Stop(req.Reason)
		writeJSON(w, http.StatusOK, gather.GatherCommandResponse{Status: status})
	})

	return http.StripPrefix("/api/gather", mux)
}
