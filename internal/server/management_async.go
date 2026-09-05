package server

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/runtime"
)

// asyncPushRequest is the payload of POST /_mock/ws/push (RS.AMG.1-7, RS.AMG.10-11).
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
		go func() {
			time.Sleep(time.Duration(req.Delay) * time.Millisecond)
			s.pushToChannel(req.Channel, req.ConnectionID, payload)
		}()
	} else {
		s.pushToChannel(req.Channel, req.ConnectionID, payload)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// evaluatePushPayload evaluates runtime expressions in a pushed payload using
// the channel's schema namespace and the environment (RS.AMG.10-11).
func (s *Server) evaluatePushPayload(payload map[string]any, channel string) (any, error) {
	prefix := s.prefixForChannel(channel)
	evaluator := runtime.NewEvaluator()
	evaluator.AddSource("state", s.newStateSource(prefix))
	evaluator.AddSource("env", s.newEnvSource())
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
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if err != nil {
		return req, err
	}
	if err := json.Unmarshal(body, &req); err != nil {
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

// pushToChannel delivers a payload to a channel's consumers. A connection id
// targets one consumer; otherwise it broadcasts. Both raw ws consumers and
// SignalR hub connections are targeted.
func (s *Server) pushToChannel(channel, connectionID string, payload []byte) {
	hub := s.hubForAddress(channel)
	if hub != nil {
		if hubChannelID := matchingHubChannel(hub, channel); hubChannelID != "" {
			if connectionID != "" {
				hub.pushToConnection(connectionID, hubChannelID, payload, hubChannelID)
			} else {
				hub.pushPayload(hubChannelID, payload)
			}
		}
	}
	if reg := s.wsRegistry(); reg != nil {
		targets := reg.connections(channel)
		for _, ws := range targets {
			if connectionID == "" || ws.id == connectionID {
				ws.writer.write(payload)
			}
		}
	}
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
				for _, st := range hub.openStreamsForChannel(channelID) {
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
			for _, st := range hub.openStreamsForChannel(id) {
				consumers = append(consumers, consumerInfo{
					ConnectionID: st["connectionId"],
					Channel:      channel,
					Streams:      []map[string]string{st},
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"consumers": consumers})
}

// pushPayload emits a payload into a hub channel's open streams or as a server
// invocation when no stream is open.
func (h *signalRHub) pushPayload(channelID string, payload []byte) {
	h.pushToStreams(channelID, payload, channelID)
}

// handleGoneSchedule answers the removed /_mock/ws/schedule surface with HTTP
// 410 Gone pointing at POST /_mock/examples (design D1). The recurring-delivery
// capability now lives on unified example injection with a runtime interval.
func (s *Server) handleGoneSchedule(w http.ResponseWriter, r *http.Request) {
	writeJSONError(w, http.StatusGone,
		"the async schedule endpoint is removed; use POST /_mock/examples with an AsyncAPI target, response.body and interval (and DELETE /_mock/examples/{exampleId} to stop)")
}

// handleAsyncDisconnect force-disconnects a consumer (RS.AMG.14-17).
func (s *Server) handleAsyncDisconnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConnectionID string `json:"connectionId"`
		Reason       string `json:"reason"`
		Code         int    `json:"code"`
		Abrupt       bool   `json:"abrupt"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
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
		hub.mu.Lock()
		if sc, ok := hub.conns[req.ConnectionID]; ok {
			hub.mu.Unlock()
			s.disconnectWS(sc.writer, req)
			return
		}
		hub.mu.Unlock()
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// disconnectWS closes a WebSocket connection with a close frame or abruptly.
func (s *Server) disconnectWS(w *wsWriter, req struct {
	ConnectionID string `json:"connectionId"`
	Reason       string `json:"reason"`
	Code         int    `json:"code"`
	Abrupt       bool   `json:"abrupt"`
}) {
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
