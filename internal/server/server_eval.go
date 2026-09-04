package server

import (
	"github.com/mamonth/oasmock/internal/runtime"
)

// Runtime-expression evaluation lives in exampleEngine; these forwarders keep
// the HTTP pipeline and tests working through Server.

func (s *Server) replaceEmbeddedExpressions(str string, eval runtime.Evaluator) (string, error) {
	return s.engine.replaceEmbeddedExpressions(str, eval)
}

func (s *Server) evaluateExpressionInString(str string, eval runtime.Evaluator) (string, error) {
	return s.engine.evaluateExpressionInString(str, eval)
}

func (s *Server) evaluateValue(val any, eval runtime.Evaluator) (any, error) {
	return s.engine.evaluateValue(val, eval)
}
