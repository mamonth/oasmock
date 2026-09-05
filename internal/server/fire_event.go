package server

import (
	"net/http"

	"github.com/mamonth/oasmock/internal/runtime"
)

// fireEventRequest is the payload of POST /_mock/events (RS.MAPI.22-23,
// RS.MAPI.32). Type discriminates the action; V1 supports "fire" only.
type fireEventRequest struct {
	Type    string         `json:"type"`
	Event   string         `json:"event"`
	Payload map[string]any `json:"payload"`
	Delay   int            `json:"delay"`
	Global  bool           `json:"global"`
}

// handleFireEventLegacy serves the deprecated /_mock/events/fire alias,
// accepting the pre-discriminator body (no "type" field). The alias keeps the
// old contract so pre-change clients keep working (design D1); the canonical
// /_mock/events endpoint still requires the type discriminator.
func (s *Server) handleFireEventLegacy(w http.ResponseWriter, r *http.Request) {
	var req fireEventRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Type = "fire"
	s.dispatchFireEvent(w, req)
}

// handleEvents dispatches a discriminated event action through the event
// broker. The type discriminator is required and only "fire" is accepted
// (RS.MAPI.32); fire reuses the existing ad-hoc fire semantics.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	var req fireEventRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	s.dispatchFireEvent(w, req)
}

// dispatchFireEvent validates and executes a fired event. Both the canonical
// and the legacy alias decode into the shared request, so the discriminator
// default lives next to the type checks instead of a body re-encode round trip.
func (s *Server) dispatchFireEvent(w http.ResponseWriter, req fireEventRequest) {
	if s.eventBus == nil {
		writeJSONError(w, http.StatusInternalServerError, "event broker not initialized")
		return
	}
	if req.Type == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required field 'type'")
		return
	}
	if req.Type != "fire" {
		writeJSONErrorf(w, http.StatusBadRequest, "unsupported event type %q (supported: fire)", req.Type)
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
	s.eventBus.fire(req.Event, req.Payload, "", req.Global, triggerDelay(req.Delay))
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"event":   req.Event,
	})
}
