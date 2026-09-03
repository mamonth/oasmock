package server

import "github.com/mamonth/oasmock/internal/runtime"

// State mutation (x-mock-set-state) lives in exampleEngine; these forwarders
// keep the HTTP pipeline and tests working through Server.

func (s *Server) handleDeleteState(prefix, resolvedKey string) {
	s.engine.handleDeleteState(prefix, resolvedKey)
}

func (s *Server) handleIncrementState(prefix, resolvedKey string, incVal any, eval runtime.Evaluator) error {
	return s.engine.handleIncrementState(prefix, resolvedKey, incVal, eval)
}

func (s *Server) handleValueObjectState(prefix, resolvedKey string, valObj any, eval runtime.Evaluator) error {
	return s.engine.handleValueObjectState(prefix, resolvedKey, valObj, eval)
}

func (s *Server) handleMapState(prefix, resolvedKey string, m map[string]any, eval runtime.Evaluator) (handled bool, err error) {
	return s.engine.handleMapState(prefix, resolvedKey, m, eval)
}

func (s *Server) handleSimpleState(prefix, resolvedKey string, val any, eval runtime.Evaluator) error {
	return s.engine.handleSimpleState(prefix, resolvedKey, val, eval)
}

func (s *Server) applySetState(stateMap map[string]any, eval runtime.Evaluator, prefix string) {
	s.engine.ApplySetState(stateMap, eval, prefix)
}
