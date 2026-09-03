package server

import (
	"encoding/json"
	"io"
	"net/http"
)

// fireEventRequest is the payload of POST /_mock/events/fire (RS.EVT.16-17).
type fireEventRequest struct {
	Event   string         `json:"event"`
	Payload map[string]any `json:"payload"`
	Delay   int            `json:"delay"`
	Global  bool           `json:"global"`
}

// handleFireEvent fires a named event ad-hoc through the event broker.
func (s *Server) handleFireEvent(w http.ResponseWriter, r *http.Request) {
	if s.eventBus == nil {
		writeJSONError(w, http.StatusInternalServerError, "event broker not initialized")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	var req fireEventRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Event == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required field 'event'")
		return
	}
	if req.Delay < 0 {
		writeJSONError(w, http.StatusBadRequest, "delay cannot be negative")
		return
	}
	// Global events apply to all schemas; otherwise they are schema-local. The
	// management endpoint has no single schema context, so global: true is
	// honored and local events are delivered to subscribed schemas.
	s.eventBus.fire(req.Event, req.Payload, "", req.Global, triggerDelay(req.Delay))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"event":   req.Event,
	})
}
