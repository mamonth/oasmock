package server

import (
	"encoding/json"

	"github.com/mamonth/oasmock/internal/runtime"
)

// fireConnectBuiltIn fires the connect built-in schema-local for a freshly
// connected consumer. The recipient set is the single connecting connection
// (RS.EVT.24, RS.EXT.26); it is a no-op when nothing subscribes to connect on
// the channel's schema.
func (s *Server) fireConnectBuiltIn(channel string, info ConsumerInfo) {
	prefix := s.prefixForChannel(channel)
	if !s.eventBus.hasSubscribers("connect", prefix) {
		return
	}
	payload := map[string]any{
		"connectionId": info.ConnectionID,
	}
	s.eventBus.fireTargeted("connect", payload, prefix, info)
}

// fireReceiveBuiltIn fires the receive built-in schema-local with the inbound
// client message exposed in the event context (RS.EVT.25). It is a no-op when
// nothing subscribes to receive on the channel's schema.
func (s *Server) fireReceiveBuiltIn(channel string, in InboundMessage, prefix string) {
	if !s.eventBus.hasSubscribers("receive", prefix) {
		return
	}
	payload := map[string]any{}
	var parsed any
	if err := json.Unmarshal(in.Payload, &parsed); err == nil && parsed != nil {
		if obj, ok := parsed.(map[string]any); ok {
			payload = obj
		} else {
			payload["data"] = parsed
		}
	} else {
		payload["data"] = string(in.Payload)
	}
	s.eventBus.fire("receive", payload, prefix, false, nil)
}

// wireBuiltInHooks connects the ws adapter and SignalR hub lifecycle/inbound
// hooks to the built-in trigger firings (design D5).
func (s *Server) wireBuiltInHooks() {
	hookSet := builtInHooks{
		Connect: func(channel, connID string, info ConsumerInfo) {
			s.fireConnectBuiltIn(channel, info)
		},
		Receive: func(channel string, in InboundMessage) {
			prefix := s.prefixForChannel(channel)
			s.fireReceiveBuiltIn(channel, in, prefix)
		},
		OnConnect: func(channel, connID string, info ConsumerInfo) {
			s.notifyConsumerLifecycle("connected", channel, info)
		},
		OnDisconnect: func(channel, connID string) {
			s.notifyConsumerLifecycle("disconnected", channel, ConsumerInfo{ConnectionID: connID, Channel: channel})
		},
	}
	if adapter, ok := s.protocolAdapters[asyncWSProtocol].(*wsProtocolAdapter); ok && adapter != nil {
		adapter.hooks = hookSet
	}
	for _, hub := range s.hubMgr.hubs {
		hub.setHooks(hookSet)
	}
}

// notifyConsumerLifecycle forwards consumer lifecycle events to the management
// stream subscribers (RS.AMG.26).
func (s *Server) notifyConsumerLifecycle(action, channel string, info ConsumerInfo) {
	if s.manageStream != nil {
		s.manageStream.notifyConsumer(action, channel, info)
	}
}

// connectionSourceFromInfo builds a runtime connection data source for an
// upgrade-captured consumer.
func connectionSourceFromInfo(info ConsumerInfo) *runtime.ConnectionSource {
	return &runtime.ConnectionSource{
		ID:      info.ConnectionID,
		Channel: info.Channel,
		Query:   info.Query,
		Headers: info.Headers,
	}
}
