package server

import (
	"net/http"

	"github.com/mamonth/oasmock/internal/runtime"
)

// fireEventRequest is the payload of POST /_mock/events (RS.MAPI.22-23,
// RS.MAPI.32).
type fireEventRequest struct {
	Name    string         `json:"name"`
	Payload map[string]any `json:"payload"`
	Delay   int            `json:"delay"`
	Global  bool           `json:"global"`
}

// handleEvents fires a named event through the event broker. A POST to the
// /events collection is the fire action itself (no type discriminator needed);
// the fired event identity is the required `name` (RS.MAPI.32).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	var req fireEventRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	s.dispatchFireEvent(w, req)
}

// dispatchFireEvent validates and executes a fired event.
func (s *Server) dispatchFireEvent(w http.ResponseWriter, req fireEventRequest) {
	if s.eventBus == nil {
		writeJSONError(w, http.StatusInternalServerError, "event broker not initialized")
		return
	}
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required field 'name'")
		return
	}
	if req.Delay < 0 {
		writeJSONError(w, http.StatusBadRequest, "delay cannot be negative")
		return
	}

	// Fired-event payload expressions {$state.*}/{$env.*} are evaluated against
	// the schema's state namespace and the environment before delivery
	// (RS.MAPI.23).
	if len(req.Payload) > 0 {
		eval := runtime.NewEvaluator()
		eval.AddSource(runtime.SourceState, s.newStateSource(""))
		eval.AddSource(runtime.SourceEnv, s.newEnvSource())
		resolved, err := s.evaluateValue(req.Payload, eval)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		var ok bool
		req.Payload, ok = resolved.(map[string]any)
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "payload must be a JSON object")
			return
		}
	}
	// Global events apply to all schemas; otherwise they are schema-local. The
	// management endpoint has no schema context of its own, so schema-local
	// fires only reach empty-prefix subscriptions (use global: true for
	// prefixed channels).
	s.eventBus.fire(req.Name, req.Payload, "", req.Global, triggerDelay(req.Delay))
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"name":    req.Name,
	})
}
