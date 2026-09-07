package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/runtime"
)

// asyncPushRequest is the payload of POST /_mock/async/push (RS.AMG.1-7, RS.AMG.10-11).
type asyncPushRequest struct {
	Channel      string         `json:"channel"`
	ConnectionID string         `json:"connectionId"`
	Payload      map[string]any `json:"payload"`
	Delay        int            `json:"delay"`
}

// handleAsyncPush pushes a message to channel consumers (immediate or delayed,
// targeted or broadcast).
func (s *Server) handleAsyncPush(w http.ResponseWriter, r *http.Request) {
	req, err := decodeAsyncPush(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Channel == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required field 'channel'")
		return
	}
	if req.Delay < 0 {
		writeJSONError(w, http.StatusBadRequest, "delay cannot be negative")
		return
	}

	registry := s.wsRegistry()
	if req.ConnectionID != "" {
		if registry == nil || !s.hasConnection(req.ConnectionID) {
			writeJSONError(w, http.StatusNotFound, "unknown connectionId")
			return
		}
	}

	// Evaluate runtime expressions in the payload against the schema's state
	// and environment at delivery time (RS.AMG.10-11).
	resolved, err := s.evaluatePushPayload(req.Payload, req.Channel)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload, err := json.Marshal(resolved)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if req.Delay > 0 {
		done := s.eventBus.doneChannel()
		go func() {
			select {
			case <-done:
				return
			case <-time.After(time.Duration(req.Delay) * time.Millisecond):
			}
			s.pushToChannel(req.Channel, req.ConnectionID, payload)
		}()
	} else {
		s.pushToChannel(req.Channel, req.ConnectionID, payload)
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// evaluatePushPayload evaluates runtime expressions in a pushed payload using
// the channel's schema namespace and the environment (RS.AMG.10-11).
func (s *Server) evaluatePushPayload(payload map[string]any, channel string) (any, error) {
	prefix := s.prefixForChannel(channel)
	evaluator := runtime.NewEvaluator()
	evaluator.AddSource(runtime.SourceState, s.newStateSource(prefix))
	evaluator.AddSource(runtime.SourceEnv, s.newEnvSource())
	return s.evaluateValue(payload, evaluator)
}

// prefixForChannel finds the schema prefix owning a channel address.
func (s *Server) prefixForChannel(channel string) string {
	for _, m := range s.mappings {
		if m.Protocol != "" && m.Path == channel {
			return m.Prefix
		}
	}
	for _, hub := range s.hubMgr.hubs {
		for _, ch := range hub.channels {
			if asyncAddressWithPrefix(hub.prefix, ch.Address) == channel {
				return hub.prefix
			}
		}
	}
	return ""
}

// decodeAsyncPush parses and validates a push request body.
func decodeAsyncPush(r *http.Request) (asyncPushRequest, error) {
	var req asyncPushRequest
	if err := decodeJSONBody(r, &req); err != nil {
		return req, err
	}
	return req, nil
}

// hasConnection reports whether a ws connection id is active (registry or hub).
func (s *Server) hasConnection(id string) bool {
	if reg := s.wsRegistry(); reg != nil {
		reg.mu.RLock()
		_, ok := reg.byID[id]
		reg.mu.RUnlock()
		if ok {
			return true
		}
	}
	return s.hubMgr.hasConnection(id)
}

// pushToChannel delivers a payload to a channel's consumers through the
// ConsumerBus (hubManager) so the targeted and broadcast paths match the
// event-delivery pipeline instead of re-implementing the registry writes. A
// connection id targets one consumer; otherwise the payload is broadcast to
// every SignalR open-stream and raw ws consumer of the channel.
func (s *Server) pushToChannel(channel, connectionID string, payload []byte) {
	if connectionID != "" {
		for _, candidate := range s.hubMgr.Candidates(channel) {
			if candidate.ConnectionID != connectionID {
				continue
			}
			s.hubMgr.PushTo(candidate, channel, payload)
			return
		}
		return
	}
	s.hubMgr.SignalRPush(channel, payload)
	s.hubMgr.WSBroadcast(channel, payload)
}

// matchingHubChannel finds the channel ID within a hub serving the address.
func matchingHubChannel(hub *signalRHub, address string) string {
	for id, ch := range hub.channels {
		if asyncAddressWithPrefix(hub.prefix, ch.Address) == address {
			return id
		}
	}
	return ""
}

// handleAsyncConsumers lists active consumers per channel (RS.AMG.8-9) or
// across all channels when the channel filter is omitted (RS.AMG.22).
func (s *Server) handleAsyncConsumers(w http.ResponseWriter, r *http.Request) {
	channel := r.URL.Query().Get("channel")
	type consumerInfo struct {
		ConnectionID string              `json:"connectionId"`
		Channel      string              `json:"channel"`
		Streams      []map[string]string `json:"streams,omitempty"`
	}
	consumers := []consumerInfo{}

	if reg := s.wsRegistry(); reg != nil {
		var conns []*wsConnection
		if channel == "" {
			conns = reg.allConnections()
		} else {
			conns = reg.connections(channel)
		}
		for _, ws := range conns {
			consumers = append(consumers, consumerInfo{ConnectionID: ws.id, Channel: ws.channel})
		}
	}
	if channel == "" {
		// Flat union across every hub channel's open streams (RS.AMG.22).
		for _, hub := range s.hubMgr.hubs {
			for channelID := range hub.channels {
				address := asyncAddressWithPrefix(hub.prefix, hub.channels[channelID].Address)
				for _, st := range hub.conns.openStreamsForChannel(channelID) {
					consumers = append(consumers, consumerInfo{
						ConnectionID: st["connectionId"],
						Channel:      address,
						Streams:      []map[string]string{st},
					})
				}
			}
		}
	} else if hub := s.hubForAddress(channel); hub != nil {
		if id := matchingHubChannel(hub, channel); id != "" {
			for _, st := range hub.conns.openStreamsForChannel(id) {
				consumers = append(consumers, consumerInfo{
					ConnectionID: st["connectionId"],
					Channel:      channel,
					Streams:      []map[string]string{st},
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"consumers": consumers})
}

// disconnectRequest is the payload of POST /_mock/async/disconnect.
type disconnectRequest struct {
	ConnectionID string `json:"connectionId"`
	Reason       string `json:"reason"`
	Code         int    `json:"code"`
	Abrupt       bool   `json:"abrupt"`
}

// handleAsyncDisconnect force-disconnects a consumer (RS.AMG.14-17).
func (s *Server) handleAsyncDisconnect(w http.ResponseWriter, r *http.Request) {
	var req disconnectRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ConnectionID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required field 'connectionId'")
		return
	}

	if !s.hasConnection(req.ConnectionID) {
		writeJSONError(w, http.StatusNotFound, "unknown connectionId")
		return
	}

	// SignalR hub connection (RS.AMG.14-15).
	for _, hub := range s.hubMgr.hubs {
		if sc, ok := hub.conns.connection(req.ConnectionID); ok {
			s.disconnectWS(sc.writer, req)
			hub.conns.unregister(req.ConnectionID)
			return
		}
	}

	// Raw ws connection.
	if reg := s.wsRegistry(); reg != nil {
		reg.mu.RLock()
		ws, ok := reg.byID[req.ConnectionID]
		reg.mu.RUnlock()
		if ok {
			s.disconnectWS(ws.writer, req)
			reg.unregister(req.ConnectionID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// disconnectWS closes a WebSocket connection with a close frame or abruptly.
func (s *Server) disconnectWS(w *wsWriter, req disconnectRequest) {
	if w == nil {
		return
	}
	if req.Abrupt {
		// Abrupt drop: abort without a close frame (RS.AMG.17).
		w.abort()
		return
	}
	code := websocket.CloseNormalClosure
	if req.Code != 0 {
		code = req.Code
	}
	w.writeClose(code, req.Reason)
}
