package server

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/loader"
)

// runtimeExampleInfo tracks a dynamically added async example so DELETE can
// unregister the broker subscription or cancel the interval job.
type runtimeExampleInfo struct {
	id      string
	address string
	prefix  string
	trigger extensions.TriggerKind
	jobID   string // scheduler job id (interval triggers)
}

// runtimeExampleRegistry owns the live async-driven examples added via
// POST /_mock/examples (RS.MAPI.24-26, RS.MAPI.30).
type runtimeExampleRegistry struct {
	mu   sync.RWMutex
	byID map[string]runtimeExampleInfo
}

func newRuntimeExampleRegistry() *runtimeExampleRegistry {
	return &runtimeExampleRegistry{byID: make(map[string]runtimeExampleInfo)}
}

func (r *runtimeExampleRegistry) add(info runtimeExampleInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[info.id] = info
}

// remove deletes an example by id and reports whether it existed.
func (r *runtimeExampleRegistry) remove(id string) (runtimeExampleInfo, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info, ok := r.byID[id]
	if ok {
		delete(r.byID, id)
	}
	return info, ok
}

// registerRuntimeExample routes a runtime async example through the event
// broker's classification path and records its registration for DELETE.
func (s *Server) registerRuntimeExample(id string, mapping *RouteMapping, spec *loader.MessageExampleSpec) (extensions.TriggerKind, string, error) {
	address := mapping.Path
	prefix := mapping.Prefix

	trigger, jobID, err := s.eventBus.registerRuntimeExample(id, address, prefix, spec)
	if err != nil {
		return 0, "", err
	}
	s.runtimeExamples.add(runtimeExampleInfo{
		id:      id,
		address: address,
		prefix:  prefix,
		trigger: trigger,
		jobID:   jobID,
	})
	return trigger, jobID, nil
}

// deleteExample removes a runtime async example (cancelling its interval job
// where one exists) or a sync dynamic example by id (RS.MAPI.30-31).
func (s *Server) deleteExample(id string) bool {
	if info, ok := s.runtimeExamples.remove(id); ok {
		switch info.trigger {
		case extensions.TriggerPeriodic:
			if s.eventBus != nil {
				s.eventBus.removeIntervalJob(info.jobID)
			}
		case extensions.TriggerEvent:
			if s.eventBus != nil {
				s.eventBus.removeEventSubscription(info.prefix, id)
			}
		}
		return true
	}
	if s.registry != nil && s.registry.removeDynamic(id) {
		return true
	}
	return false
}

// handleDeleteExample implements DELETE /_mock/examples/{exampleId}.
func (s *Server) handleDeleteExample(w http.ResponseWriter, r *http.Request) {
	exampleID := chi.URLParam(r, "exampleId")
	if !s.deleteExample(exampleID) {
		writeJSONError(w, http.StatusNotFound, "unknown exampleId")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
