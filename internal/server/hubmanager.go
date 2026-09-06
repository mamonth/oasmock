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

// hubChannelForAddress finds the SignalR hub channel whose fully-prefixed
// address matches, returning the hub and its channel id. It is the single
// address-resolution point used by every delivery and discovery path.
func (m *hubManager) hubChannelForAddress(address string) (*signalRHub, string) {
	hub := m.hubForAddress(address)
	if hub == nil {
		return nil, ""
	}
	for channelID, ch := range hub.channels {
		if asyncAddressWithPrefix(hub.prefix, ch.Address) == address {
			return hub, channelID
		}
	}
	return nil, ""
}

// hasConnection reports whether a connection id is active on any hub.
func (m *hubManager) hasConnection(id string) bool {
	for _, hub := range m.hubs {
		if hub.conns.hasConnection(id) {
			return true
		}
	}
	return false
}

// SignalRPush emits a payload into a SignalR hub channel's open streams or as
// a server invocation when none are open (ConsumerBus, RS.SHR.18-19).
func (m *hubManager) SignalRPush(address string, payload []byte) {
	hub, channelID := m.hubChannelForAddress(address)
	if hub != nil {
		hub.conns.pushToStreams(channelID, payload, channelID)
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
// with the connection context captured at upgrade. SignalR candidates are
// deduplicated per connection: a connection with several open streams on the
// channel appears once, carrying all of its streams, so a per-connection
// PushTo (which writes to every open stream) cannot duplicate delivery
// quadratically (RS.SHR.22).
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
	if hub, channelID := m.hubChannelForAddress(address); hub != nil {
		seen := make(map[string]int) // connectionID -> index in out
		for _, st := range hub.conns.openStreamsForChannel(channelID) {
			connID := st["connectionId"]
			if idx, ok := seen[connID]; ok {
				out[idx].Streams = append(out[idx].Streams, st)
				continue
			}
			seen[connID] = len(out)
			out = append(out, ConsumerInfo{
				ConnectionID: connID,
				Channel:      address,
				Query:        hub.conns.connectionMetadata(connID),
				Headers:      hub.conns.connectionHeaders(connID),
				Streams:      []map[string]string{st},
			})
		}
	}
	return out
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
	hub, channelID := m.hubChannelForAddress(address)
	if hub == nil {
		return
	}
	hub.conns.pushToConnection(consumer.ConnectionID, channelID, payload, channelID)
}
