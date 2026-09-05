package extensions

import (
	"github.com/mamonth/oasmock/internal/runtime"
)

// newMatchEvaluator builds a real runtime evaluator exposing the request,
// event, connection and state contexts for match-condition tests. Nil sources
// are simply not registered, so the same builder serves reply-path, event-path
// and connection-path cases.
func newMatchEvaluator(request *runtime.RequestSource, event *runtime.EventSource, conn *runtime.ConnectionSource, state map[string]any) runtime.Evaluator {
	eval := runtime.NewEvaluator()
	if request != nil {
		eval.AddSource("request", request)
	}
	eval.AddSource("event", event)
	if conn != nil {
		eval.AddSource("connection", conn)
	}
	if state != nil {
		eval.AddSource("state", &runtime.StateSource{Data: state})
	}
	return eval
}
