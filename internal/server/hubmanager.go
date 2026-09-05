package server

import "strings"

// hubManager owns the SignalR hubs built from AsyncAPI documents and the raw
// ws protocol adapter, exposing connection lookup and payload delivery behind
// the ConsumerBus contract. It depends only on the MessageRenderer surface.
type hubManager struct {
	hubs []*signalRHub
	ws   *wsProtocolAdapter
}

// hubForAddress finds the SignalR hub owning a channel address.
func (s *Server) hubForAddress(address string) *signalRHub {
	return s.hubMgr.hubForAddress(address)
}

// asyncAddressWithPrefix applies a schema prefix to a channel address.
func asyncAddressWithPrefix(prefix, address string) string {
	addr := "/" + strings.Trim(address, "/")
	if prefix == "" {
		return addr
	}
	return "/" + strings.Trim(prefix, "/") + addr
}

// newHubManager builds a hub per AsyncAPI document declaring root x-signalr and
// keeps the ws adapter used for raw consumer broadcast.
func newHubManager(renderer MessageRenderer, ws *wsProtocolAdapter, schemas []SchemaInfo) *hubManager {
	return &hubManager{
		hubs: buildSignalRHubs(renderer, schemas),
		ws:   ws,
	}
}

// hubForAddress finds the SignalR hub owning a channel address.
func (m *hubManager) hubForAddress(address string) *signalRHub {
	for _, hub := range m.hubs {
		for _, ch := range hub.channels {
			if asyncAddressWithPrefix(hub.prefix, ch.Address) == address {
				return hub
			}
		}
	}
	return nil
}

// hasConnection reports whether a connection id is active on any hub.
func (m *hubManager) hasConnection(id string) bool {
	for _, hub := range m.hubs {
		hub.mu.Lock()
		_, ok := hub.conns[id]
		hub.mu.Unlock()
		if ok {
			return true
		}
	}
	return false
}

// SignalRPush emits a payload into a SignalR hub channel's open streams or as
// a server invocation when none are open (ConsumerBus, RS.SHR.18-19).
func (m *hubManager) SignalRPush(address string, payload []byte) {
	hub := m.hubForAddress(address)
	if hub == nil {
		return
	}
	for channelID, ch := range hub.channels {
		if asyncAddressWithPrefix(hub.prefix, ch.Address) == address {
			hub.pushToStreams(channelID, payload, channelID)
		}
	}
}

// WSBroadcast sends a payload to every connected raw ws consumer of a channel
// address (ConsumerBus).
func (m *hubManager) WSBroadcast(address string, payload []byte) {
	if m.ws == nil {
		return
	}
	m.ws.broadcast(address, payload)
}

// Candidates returns every consumer of a channel address (raw ws and SignalR)
// with the connection context captured at upgrade.
func (m *hubManager) Candidates(address string) []ConsumerInfo {
	var out []ConsumerInfo
	if m.ws != nil {
		for _, ws := range m.ws.registry.connections(address) {
			out = append(out, ConsumerInfo{
				ConnectionID: ws.id,
				Channel:      ws.channel,
				Query:        ws.query,
				Headers:      ws.headers,
			})
		}
	}
	if hub := m.hubForAddress(address); hub != nil {
		for channelID, ch := range hub.channels {
			if asyncAddressWithPrefix(hub.prefix, ch.Address) != address {
				continue
			}
			for _, st := range hub.openStreamsForChannel(channelID) {
				out = append(out, ConsumerInfo{
					ConnectionID: st["connectionId"],
					Channel:      address,
					Query:        hub.connectionMetadata(st["connectionId"]),
					Headers:      hub.connectionHeaders(st["connectionId"]),
					Streams:      []map[string]string{st},
				})
			}
		}
	}
	return out
}

// connectionMetadata returns the upgrade-time query metadata of a connection
// (for {$connection.query.*} evaluation); nil when unknown.
func (h *signalRHub) connectionMetadata(connID string) map[string][]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sc, ok := h.conns[connID]; ok {
		return sc.query
	}
	return nil
}

// connectionHeaders returns the upgrade-time header metadata of a connection;
// nil when unknown. Header keys are lower-cased at capture time.
func (h *signalRHub) connectionHeaders(connID string) map[string][]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sc, ok := h.conns[connID]; ok {
		return sc.headers
	}
	return nil
}

// PushTo delivers a payload to one candidate consumer: its raw ws connection,
// or its SignalR stream network (falling back to a server invocation).
func (m *hubManager) PushTo(consumer ConsumerInfo, address string, payload []byte) {
	if m.ws != nil {
		if ws, ok := m.ws.registry.connection(consumer.ConnectionID); ok {
			ws.writer.write(payload)
			return
		}
	}
	hub := m.hubForAddress(address)
	if hub == nil {
		return
	}
	for channelID, ch := range hub.channels {
		if asyncAddressWithPrefix(hub.prefix, ch.Address) != address {
			continue
		}
		hub.pushToConnection(consumer.ConnectionID, channelID, payload, channelID)
		return
	}
}
