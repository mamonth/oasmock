package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
)

// asyncMessageHandler returns a MessageHandler that renders an AsyncAPI route's
// message examples through the shared selection pipeline (design D5).
func (s *Server) asyncMessageHandler(mapping *RouteMapping) MessageHandler {
	return MessageHandlerFunc(func(ctx context.Context, in InboundMessage) ([]byte, error) {
		count, body, err := s.renderAsyncMessage(mapping, in)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			// No message/reply produced (e.g. send with no reply): nil body.
			s.recordAsyncExchange(in, mapping.Path, http.StatusOK, nil)
			return nil, nil
		}
		s.recordAsyncExchange(in, mapping.Path, http.StatusOK, body)
		return body, nil
	})
}

// Message rendering lives in exampleEngine; these forwarders keep the protocol
// adapters and tests working through Server.

func (s *Server) renderAsyncMessage(mapping *RouteMapping, in InboundMessage) (int, []byte, error) {
	return s.engine.renderAsyncMessage(mapping, in)
}

func (s *Server) newAsyncEvaluator(mapping *RouteMapping, in InboundMessage) runtime.Evaluator {
	return s.engine.newAsyncEvaluator(mapping, in)
}

func (s *Server) renderMessageSpecs(messages []*loader.MessageSpec, prefix, opID string, in InboundMessage) (int, []byte, error) {
	return s.engine.RenderMessageSpecs(messages, prefix, opID, in)
}

func (s *Server) asyncRequestSource(in InboundMessage) *runtime.RequestSource {
	return s.engine.asyncRequestSource(in)
}

func (s *Server) selectAsyncExample(message *loader.MessageSpec, evaluator runtime.Evaluator, opID string) (*MessageExampleView, string) {
	return s.engine.SelectAsyncExample(message, evaluator, opID)
}

func (s *Server) renderAsyncPayload(example *MessageExampleView, evaluator runtime.Evaluator) ([]byte, error) {
	return s.engine.RenderAsyncPayload(example, evaluator)
}

func (s *Server) recordAsyncExchange(in InboundMessage, address string, status int, responseBody []byte) {
	s.engine.recordAsyncExchange(in, address, status, responseBody)
}

// MessageExampleView adapts an AsyncAPI message example to the ExampleValue
// contract so extension extraction and selection are source-agnostic (D5).
type MessageExampleView struct {
	spec *loader.MessageExampleSpec
}

// Get implements extensions.ExampleValue.
func (v *MessageExampleView) Get(key string) (any, bool) {
	if v == nil || v.spec == nil || v.spec.Extensions == nil {
		return nil, false
	}
	val, ok := v.spec.Extensions[key]
	return val, ok
}

// Payload implements extensions.ExampleValue.
func (v *MessageExampleView) Payload() any {
	if v == nil || v.spec == nil {
		return nil
	}
	return v.spec.Payload
}

// Headers implements extensions.ExampleValue.
func (v *MessageExampleView) Headers() map[string]any {
	if v == nil || v.spec == nil {
		return nil
	}
	return v.spec.Headers
}

func idxName(msgName string, idx int) string {
	if msgName == "" {
		return fmt.Sprintf("example-%d", idx)
	}
	return fmt.Sprintf("%s-%d", msgName, idx)
}

// jsonPayload parses inbound bytes as JSON, falling back to a raw string.
func jsonPayload(data []byte) any {
	if len(data) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return string(data)
	}
	return v
}
