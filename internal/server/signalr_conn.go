package server

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/loader"
)

// runConnection drives the SignalR message loop for one connection. A read
// deadline bounds the lifetime of silently-dead peers (pings are answered, but
// a peer that stops sending entirely is reaped).
func (h *signalRHub) runConnection(sc *signalRConnection) {
	handshaken := false
	_ = sc.conn.SetReadDeadline(time.Now().Add(wsReadIdleBounds))
	for {
		messageType, payload, err := sc.conn.ReadMessage()
		if err != nil {
			return
		}
		_ = sc.conn.SetReadDeadline(time.Now().Add(wsReadIdleBounds))
		if messageType == websocket.PingMessage {
			sc.writer.writeMessage(websocket.PongMessage, payload)
			continue
		}
		// Handshake: the first frame must be the protocol handshake (RS.SHR.14-15).
		if !handshaken {
			if messageType != websocket.TextMessage {
				h.writeHandshakeError(sc, "binary frames are not supported")
				return
			}
			proto, version, hserr := parseSignalRHandshake(payload)
			if hserr != nil {
				h.writeHandshakeError(sc, hserr.Error())
				return
			}
			_, _ = proto, version
			sc.writer.write([]byte("{}" + string(recordSeparator)))
			handshaken = true
			continue
		}
		if messageType != websocket.TextMessage {
			continue
		}
		for _, chunk := range splitSignalRFrames(payload) {
			env, err := parseSignalREnvelope(chunk)
			if err != nil {
				continue
			}
			h.dispatch(sc, env)
		}
	}
}

// writeHandshakeError sends a handshake response then closes (RS.SHR.15). The
// frame is JSON-encoded so the message is escaped and cannot corrupt the frame.
func (h *signalRHub) writeHandshakeError(sc *signalRConnection, msg string) {
	sc.writer.write(encodeSignalRMessage(signalREnvelope{Error: msg}))
	sc.writer.close()
}

// dispatch routes a single parsed envelope.
func (h *signalRHub) dispatch(sc *signalRConnection, env signalREnvelope) {
	switch env.Type {
	case signalRTypePing:
		sc.writer.write(encodeSignalRMessage(signalREnvelope{Type: signalRTypePing}))
	case signalRTypeStreamInvocation:
		h.handleStreamInvocation(sc, env)
	case signalRTypeInvocation:
		// An inbound client invocation carries a payload; fire the receive
		// built-in (RS.EVT.25) before answering. A single argument is exposed
		// directly as the event payload (the common case for message mocks).
		if h.hooks.Receive != nil {
			payload := json.RawMessage("{}")
			if len(env.Arguments) == 1 {
				payload, _ = json.Marshal(env.Arguments[0])
			} else if len(env.Arguments) > 1 {
				payload, _ = json.Marshal(env.Arguments)
			}
			ch := hubChannelAddress(h)
			if ch != "" {
				h.hooks.Receive(ch, InboundMessage{
					Payload:      payload,
					ConnectionID: sc.id,
				})
			}
		}
		h.handleInvocation(sc, env)
	case signalRTypeCancelInvocation:
		h.handleCancelInvocation(sc, env)
	}
}

// hubDefaultChannel returns the channel address a SignalR hub uses for
// connection-level built-ins (connect/receive) and recipient metadata. A hub
// may serve several channels, so selection is deterministic (the
// lexicographically smallest prefixed address) rather than relying on map
// iteration order. It is empty when the hub has no addressable channel.
func hubDefaultChannel(h *signalRHub) string {
	best := ""
	for _, ch := range h.channels {
		if ch.Address == "" {
			continue
		}
		addr := asyncAddressWithPrefix(h.prefix, ch.Address)
		if best == "" || addr < best {
			best = addr
		}
	}
	return best
}

// hubChannelAddress returns the default channel address of a hub (used by the
// receive built-in dispatch).
func hubChannelAddress(h *signalRHub) string {
	return hubDefaultChannel(h)
}

// handleStreamInvocation answers a StreamInvocation by channel ID with the
// channel's snapshot message and holds the stream open (RS.SHR.3-5, RS.SHR.17).
func (h *signalRHub) handleStreamInvocation(sc *signalRConnection, env signalREnvelope) {
	channelID := env.Target
	ch := h.channels[channelID]
	if ch == nil {
		h.writeCompletion(sc, env.InvocationID, fmt.Sprintf("unknown channel target %q", channelID))
		return
	}

	// Snapshot stream item(s).
	count, body, err := h.renderChannel(channelID)
	if err != nil {
		h.writeCompletion(sc, env.InvocationID, fmt.Sprintf("failed to render channel: %v", err))
		return
	}
	if count == 0 {
		h.writeCompletion(sc, env.InvocationID, "channel has no message examples")
		return
	}
	sc.writer.write(encodeSignalRMessage(signalREnvelope{
		Type:         signalRTypeStreamItem,
		InvocationID: env.InvocationID,
		Item:         json.RawMessage(body),
	}))

	// Hold the stream open and register it (RS.SHR.4, RS.SHR.21).
	h.mu.Lock()
	sc.streams[env.InvocationID] = &signalRStream{
		invocationID: env.InvocationID,
		channelID:    channelID,
		connID:       sc.id,
	}
	h.mu.Unlock()
}

// handleInvocation answers a one-shot Invocation by operation ID with a
// Completion carrying the operation's message example (RS.SHR.6-7).
func (h *signalRHub) handleInvocation(sc *signalRConnection, env signalREnvelope) {
	opID := env.Target
	op := h.ops[opID]
	if op == nil {
		h.writeCompletion(sc, env.InvocationID, fmt.Sprintf("unknown operation target %q", opID))
		return
	}
	count, body, err := h.renderOperation(opID)
	if err != nil {
		h.writeCompletion(sc, env.InvocationID, fmt.Sprintf("failed to render operation: %v", err))
		return
	}
	if count == 0 {
		h.writeCompletion(sc, env.InvocationID, "operation has no message examples")
		return
	}
	sc.writer.write(encodeSignalRMessage(signalREnvelope{
		Type:         signalRTypeCompletion,
		InvocationID: env.InvocationID,
		Result:       json.RawMessage(body),
	}))
}

// handleCancelInvocation closes an open stream (RS.SHR.17).
func (h *signalRHub) handleCancelInvocation(sc *signalRConnection, env signalREnvelope) {
	h.mu.Lock()
	if st, ok := sc.streams[env.InvocationID]; ok {
		h.unregisterStream(st)
	}
	h.mu.Unlock()
	h.writeCompletion(sc, env.InvocationID, "")
}

// unregisterStream removes a stream from its connection registry.
// The caller must hold h.mu.
func (h *signalRHub) unregisterStream(st *signalRStream) {
	if sc, ok := h.conns[st.connID]; ok {
		delete(sc.streams, st.invocationID)
	}
}

// writeCompletion sends a completion envelope.
func (h *signalRHub) writeCompletion(sc *signalRConnection, invocationID, errMsg string) {
	env := signalREnvelope{Type: signalRTypeCompletion, InvocationID: invocationID}
	if errMsg != "" {
		env.Error = errMsg
	}
	sc.writer.write(encodeSignalRMessage(env))
}

// renderChannel renders the snapshot message for a channel as JSON bytes.
func (h *signalRHub) renderChannel(channelID string) (int, []byte, error) {
	ch := h.channels[channelID]
	if ch == nil {
		return 0, nil, nil
	}
	specs := loader.MessageSpecsFromAsync(ch.Messages)
	opID := "signalr:channel:" + channelID
	return h.renderer.RenderMessageSpecs(specs, h.prefix, opID, InboundMessage{})
}

// renderOperation renders the result message for a one-shot operation.
func (h *signalRHub) renderOperation(opID string) (int, []byte, error) {
	op := h.ops[opID]
	if op == nil {
		return 0, nil, nil
	}
	specs := loader.MessageSpecsFromAsync(op.Messages)
	opKey := "signalr:operation:" + opID
	return h.renderer.RenderMessageSpecs(specs, h.prefix, opKey, InboundMessage{})
}

// streamDelivery is one snapshotted write to perform after the hub lock is
// released, so network I/O never blocks other hub operations (negotiate,
func (h *signalRHub) openStreamsForChannel(channelID string) []map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []map[string]string
	for _, sc := range h.conns {
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
