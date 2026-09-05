package server

import (
	"encoding/json"
	"strconv"
	"sync"
)

// streamDelivery is one snapshotted write to perform after the registry lock
// is released, so network I/O never blocks other hub operations.
type streamDelivery struct {
	writer *wsWriter
	env    signalREnvelope
}

// signalRConnRegistry owns the connection, token and open-stream state of a
// SignalR hub, plus the payload-delivery helpers that address those
// connections. It is the single owner of the hub's mu so transport handling
// (negotiate/upgrade/read loop) never reaches into connection state directly.
type signalRConnRegistry struct {
	mu     sync.Mutex
	tokens map[string]string // connection token -> connection id
	conns  map[string]*signalRConnection
	idSeq  int
}

func newSignalRConnRegistry() *signalRConnRegistry {
	return &signalRConnRegistry{
		tokens: make(map[string]string),
		conns:  make(map[string]*signalRConnection),
	}
}

// issueToken creates and records a connection token.
func (r *signalRConnRegistry) issueToken() (token, connID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idSeq++
	connID = "signalr-" + strconv.Itoa(r.idSeq)
	token = connID + "-t"
	r.tokens[token] = connID
	return token, connID
}

// consumeToken validates and consumes a token, binding the connection.
func (r *signalRConnRegistry) consumeToken(token string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	connID, ok := r.tokens[token]
	if ok {
		delete(r.tokens, token)
	}
	return connID, ok
}

// freshToken returns a token for a connection id (no pre-correlation).
func (r *signalRConnRegistry) freshToken() (token, connID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idSeq++
	connID = "signalr-fresh-" + strconv.Itoa(r.idSeq)
	return connID + "-t", connID
}

// register adds a connection to the registry.
func (r *signalRConnRegistry) register(sc *signalRConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[sc.id] = sc
}

// unregister removes a connection; its open streams are discarded with the
// connection object, so no separate stream cleanup is needed (they become
// undiscoverable through openStreamsForChannel which iterates r.conns).
func (r *signalRConnRegistry) unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.conns, id)
}

// hasConnection reports whether a connection id is active.
func (r *signalRConnRegistry) hasConnection(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.conns[id]
	return ok
}

// connection returns a registered connection, or nil.
func (r *signalRConnRegistry) connection(id string) (*signalRConnection, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sc, ok := r.conns[id]
	return sc, ok
}

// connections returns a snapshot of all registered connections keyed by id.
func (r *signalRConnRegistry) connections() map[string]*signalRConnection {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*signalRConnection, len(r.conns))
	for id, sc := range r.conns {
		out[id] = sc
	}
	return out
}

// connectionMetadata returns the upgrade-time query metadata of a connection
// (for {$connection.query.*} evaluation); nil when unknown.
func (r *signalRConnRegistry) connectionMetadata(connID string) map[string][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sc, ok := r.conns[connID]; ok {
		return sc.query
	}
	return nil
}

// connectionHeaders returns the upgrade-time header metadata of a connection;
// nil when unknown. Header keys are lower-cased at capture time.
func (r *signalRConnRegistry) connectionHeaders(connID string) map[string][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sc, ok := r.conns[connID]; ok {
		return sc.headers
	}
	return nil
}

// openStreamsForChannel returns open-stream descriptions for a channel.
func (r *signalRConnRegistry) openStreamsForChannel(channelID string) []map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []map[string]string
	for _, sc := range r.conns {
		for invocationID, st := range sc.streams {
			if st.channelID == channelID {
				out = append(out, map[string]string{
					"connectionId": sc.id,
					"invocationId": invocationID,
					"streamId":     st.channelID,
				})
			}
		}
	}
	return out
}

// unregisterStream removes a stream from its connection registry. The caller
// must hold r.mu.
func (r *signalRConnRegistry) unregisterStream(st *signalRStream) {
	if sc, ok := r.conns[st.connID]; ok {
		delete(sc.streams, st.invocationID)
	}
}

// pushToStreams emits a templated payload into all open streams of a channel;
// when no stream is open it sends a server Invocation (RS.SHR.18-19).
func (r *signalRConnRegistry) pushToStreams(channelID string, payload []byte, target string) {
	r.mu.Lock()
	deliveries := r.buildStreamDeliveries(channelID, payload, target, nil)
	r.mu.Unlock()
	for _, d := range deliveries {
		d.writer.write(encodeSignalRMessage(d.env))
	}
}

// pushToConnection pushes a payload to one connection's open streams for the
// channel, falling back to a server Invocation on that connection when no
// stream is open (RS.AMG.5, RS.SHR.18-19).
func (r *signalRConnRegistry) pushToConnection(connectionID, channelID string, payload []byte, target string) {
	r.mu.Lock()
	sc, ok := r.conns[connectionID]
	var deliveries []streamDelivery
	if ok {
		deliveries = r.buildStreamDeliveries(channelID, payload, target, sc)
	}
	r.mu.Unlock()
	for _, d := range deliveries {
		d.writer.write(encodeSignalRMessage(d.env))
	}
}

// buildStreamDeliveries snapshots the writes needed to deliver a payload to a
// channel's open streams, falling back to per-connection server Invocations
// (each with a distinct server-assigned id) when no stream matches. The caller
// must hold r.mu; when conn is non-nil delivery is restricted to that single
// connection.
func (r *signalRConnRegistry) buildStreamDeliveries(channelID string, payload []byte, target string, conn *signalRConnection) []streamDelivery {
	var out []streamDelivery
	emitInvocation := func(sc *signalRConnection) {
		r.idSeq++
		out = append(out, streamDelivery{writer: sc.writer, env: signalREnvelope{
			Type:         signalRTypeInvocation,
			InvocationID: "srv-" + target + "-" + strconv.Itoa(r.idSeq),
			Target:       target,
			Arguments:    []any{json.RawMessage(payload)},
		}})
	}
	matched := false
	writeStreamItems := func(sc *signalRConnection) {
		for invocationID, st := range sc.streams {
			if st.channelID != channelID {
				continue
			}
			matched = true
			out = append(out, streamDelivery{writer: sc.writer, env: signalREnvelope{
				Type:         signalRTypeStreamItem,
				InvocationID: invocationID,
				Item:         json.RawMessage(payload),
			}})
		}
	}
	if conn != nil {
		writeStreamItems(conn)
		if !matched {
			emitInvocation(conn)
		}
		return out
	}
	for _, sc := range r.conns {
		writeStreamItems(sc)
	}
	if !matched {
		for _, sc := range r.conns {
			emitInvocation(sc)
		}
	}
	return out
}
