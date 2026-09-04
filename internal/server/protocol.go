package server

import (
	"context"
	"net/http"
)

// InboundMessage is a message received from a client on an AsyncAPI channel.
type InboundMessage struct {
	// Payload is the raw message bytes received from the client.
	Payload []byte
	// Headers carries request headers (HTTP) or message headers (ws).
	Headers map[string]string
	// PathParams carries resolved channel address parameters.
	PathParams map[string]string
	// ConnectionID identifies the ws consumer connection.
	ConnectionID string
}

// MessageHandler renders an AsyncAPI message example for a route through the
// shared selection pipeline. Implementations return the outbound payload bytes
// to write back to the client, or nil to emit nothing.
type MessageHandler interface {
	HandleMessage(ctx context.Context, in InboundMessage) ([]byte, error)
}

// MessageHandlerFunc adapts a function to the MessageHandler interface.
type MessageHandlerFunc func(ctx context.Context, in InboundMessage) ([]byte, error)

// HandleMessage implements MessageHandler.
func (f MessageHandlerFunc) HandleMessage(ctx context.Context, in InboundMessage) ([]byte, error) {
	return f(ctx, in)
}

// ProtocolAdapter serves AsyncAPI routes for one protocol binding (design D4).
// The adapter owns protocol-specific transport concerns; message rendering is
// delegated to the shared MessageHandler pipeline.
type ProtocolAdapter interface {
	// Protocol returns the binding name this adapter serves (e.g. "http", "ws").
	Protocol() string
	// Handler builds the HTTP handler that serves an AsyncAPI route mapping.
	Handler(mapping *RouteMapping, handler MessageHandler) http.HandlerFunc
}

// defaultProtocolAdapters is the set of adapters seeded for a new server.
func defaultProtocolAdapters() map[string]ProtocolAdapter {
	return map[string]ProtocolAdapter{
		asyncHTTPProtocol: &httpProtocolAdapter{},
		asyncWSProtocol:   newWSProtocolAdapter(),
	}
}

// wsRegistry returns the ws protocol adapter's connection registry, or nil
// when the ws adapter is not registered.
func (s *Server) wsRegistry() *connectionRegistry {
	if a, ok := s.protocolAdapters[asyncWSProtocol].(*wsProtocolAdapter); ok && a != nil {
		return a.registry
	}
	return nil
}

// adapterForProtocol returns the registered protocol adapter for a binding.
func (s *Server) adapterForProtocol(protocol string) ProtocolAdapter {
	return s.protocolAdapters[protocol]
}

const (
	asyncHTTPProtocol = "http"
	asyncWSProtocol   = "ws"
)
