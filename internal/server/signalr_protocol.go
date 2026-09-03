package server

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Async SignalR message type codes per the protocol spec.
const (
	signalRTypeInvocation       = 1
	signalRTypeStreamItem       = 2
	signalRTypeCompletion       = 3
	signalRTypeStreamInvocation = 4
	signalRTypeCancelInvocation = 5
	signalRTypePing             = 6
)

// recordSeparator terminates every SignalR JSON message.
const recordSeparator = byte('\x1e')

// signalREnvelope is a single SignalR protocol message.
type signalREnvelope struct {
	Protocol     string         `json:"protocol,omitempty"`
	Version      int            `json:"version,omitempty"`
	Type         int            `json:"type"`
	InvocationID string         `json:"invocationId,omitempty"`
	Target       string         `json:"target,omitempty"`
	Arguments    []any          `json:"arguments,omitempty"`
	Error        string         `json:"error,omitempty"`
	Result       any            `json:"result,omitempty"`
	Item         any            `json:"item,omitempty"`
	StreamIDs    []string       `json:"streamIds,omitempty"`
	Headers      map[string]any `json:"headers,omitempty"`
}

// splitSignalRFrames splits one WebSocket text frame into SignalR messages on
// the 0x1E record separator (RS.SHR.16).
func splitSignalRFrames(frame []byte) [][]byte {
	var out [][]byte
	for _, chunk := range bytes.Split(frame, []byte{recordSeparator}) {
		chunk = bytes.TrimSpace(chunk)
		if len(chunk) == 0 {
			continue
		}
		out = append(out, chunk)
	}
	return out
}

// encodeSignalRMessage JSON-encodes a message and terminates it with 0x1E.
func encodeSignalRMessage(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		data = []byte(`{"error":"encoding failure"}`)
	}
	return append(data, recordSeparator)
}

// parseSignalRHandshake validates the first-frame handshake. Only the JSON
// protocol with version 1 is accepted (RS.SHR.14, RS.SHR.15).
func parseSignalRHandshake(data []byte) (string, int, error) {
	var hs struct {
		Protocol string `json:"protocol"`
		Version  int    `json:"version"`
	}
	if err := json.Unmarshal(data, &hs); err != nil {
		return "", 0, fmt.Errorf("invalid handshake: %w", err)
	}
	if hs.Protocol != "json" {
		return "", 0, fmt.Errorf("unsupported protocol %q (only json is supported)", hs.Protocol)
	}
	if hs.Version != 1 {
		return "", 0, fmt.Errorf("unsupported protocol version %d", hs.Version)
	}
	return hs.Protocol, hs.Version, nil
}

// parseSignalREnvelope decodes a SignalR message envelope.
func parseSignalREnvelope(data []byte) (signalREnvelope, error) {
	var env signalREnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return env, fmt.Errorf("invalid SignalR message: %w", err)
	}
	return env, nil
}

// String renders a debugging representation of the envelope.
func (e signalREnvelope) String() string {
	return fmt.Sprintf("type=%d invocationId=%s target=%s", e.Type, e.InvocationID, e.Target)
}
