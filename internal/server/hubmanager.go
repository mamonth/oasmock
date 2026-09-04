package server

// hubManager owns the SignalR hubs built from AsyncAPI documents and the raw
// ws protocol adapter, exposing connection lookup and payload delivery behind
// the ConsumerBus contract. It depends only on the MessageRenderer surface.
type hubManager struct {
	hubs []*signalRHub
	ws   *wsProtocolAdapter
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
