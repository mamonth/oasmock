package server

import (
	"encoding/json"
	"strconv"
)

type streamDelivery struct {
	writer *wsWriter
	env    signalREnvelope
}

// pushToStreams emits a templated payload into all open streams of a channel;
// when no stream is open it sends a server Invocation (RS.SHR.18-19).
func (h *signalRHub) pushToStreams(channelID string, payload []byte, target string) {
	h.mu.Lock()
	deliveries := h.buildStreamDeliveries(channelID, payload, target, nil)
	h.mu.Unlock()
	for _, d := range deliveries {
		d.writer.write(encodeSignalRMessage(d.env))
	}
}

// pushToConnection pushes a payload to one connection's open streams for the
// channel, falling back to a server Invocation on that connection when no
// stream is open (RS.AMG.5, RS.SHR.18-19).
func (h *signalRHub) pushToConnection(connectionID, channelID string, payload []byte, target string) {
	h.mu.Lock()
	sc, ok := h.conns[connectionID]
	var deliveries []streamDelivery
	if ok {
		deliveries = h.buildStreamDeliveries(channelID, payload, target, sc)
	}
	h.mu.Unlock()
	for _, d := range deliveries {
		d.writer.write(encodeSignalRMessage(d.env))
	}
}

// buildStreamDeliveries snapshots the writes needed to deliver a payload to a
// channel's open streams, falling back to per-connection server Invocations
// (each with a distinct server-assigned id) when no stream matches. The caller
// must hold h.mu; when conn is non-nil delivery is restricted to that single
// connection.
func (h *signalRHub) buildStreamDeliveries(channelID string, payload []byte, target string, conn *signalRConnection) []streamDelivery {
	var out []streamDelivery
	emitInvocation := func(sc *signalRConnection) {
		h.idSeq++
		out = append(out, streamDelivery{writer: sc.writer, env: signalREnvelope{
			Type:         signalRTypeInvocation,
			InvocationID: "srv-" + target + "-" + strconv.Itoa(h.idSeq),
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
	for _, sc := range h.conns {
		writeStreamItems(sc)
	}
	if !matched {
		for _, sc := range h.conns {
			emitInvocation(sc)
		}
	}
	return out
}
