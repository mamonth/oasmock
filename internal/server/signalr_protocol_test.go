package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Splitting a SignalR text frame on the record separator
Given bytes containing several JSON messages separated by 0x1E
When splitSignalRFrames is called
Then each chunk is returned as a separate message

Related spec scenarios: RS.SHR.16
*/
func TestSplitSignalRFrames(t *testing.T) {
	t.Parallel()

	frame := []byte("{\"type\":4,\"target\":\"c\"}\x1e{\"type\":6}\x1e")
	chunks := splitSignalRFrames(frame)
	require.Len(t, chunks, 2)
	assert.JSONEq(t, `{"type":4,"target":"c"}`, string(chunks[0]))
	assert.JSONEq(t, `{"type":6}`, string(chunks[1]))
}

/*
Scenario: Encoding a SignalR message with the record separator
Given a message struct
When encodeSignalRMessage is called
Then the JSON payload is terminated by the 0x1E byte

Related spec scenarios: RS.SHR.16
*/
func TestEncodeSignalRMessage(t *testing.T) {
	t.Parallel()

	out := encodeSignalRMessage(map[string]any{"type": 6})
	assert.Equal(t, "{\"type\":6}\x1e", string(out))
	assert.True(t, strings.HasSuffix(string(out), "\x1e"))
}

/*
Scenario: Parsing a valid SignalR handshake
Given the JSON protocol handshake payload
When parseSignalRHandshake is called
Then it returns protocol json and version 1 without error

Related spec scenarios: RS.SHR.14
*/
func TestParseSignalRHandshake_Valid(t *testing.T) {
	t.Parallel()

	proto, version, err := parseSignalRHandshake([]byte(`{"protocol":"json","version":1}`))
	require.NoError(t, err)
	assert.Equal(t, "json", proto)
	assert.Equal(t, 1, version)
}

/*
Scenario: Rejecting an unsupported SignalR handshake protocol
Given a handshake requesting messagepack
When parseSignalRHandshake is called
Then it returns an error

Related spec scenarios: RS.SHR.15
*/
func TestParseSignalRHandshake_UnsupportedProtocol(t *testing.T) {
	t.Parallel()

	_, _, err := parseSignalRHandshake([]byte(`{"protocol":"messagepack","version":1}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "messagepack")
}

/*
Scenario: Parsing a SignalR envelope by type
Given a JSON representation of a type-4 StreamInvocation
When parseSignalREnvelope is called
Then the envelope type and target are decoded

Related spec scenarios: RS.SHR.3, RS.SHR.16
*/
func TestParseSignalREnvelope_StreamInvocation(t *testing.T) {
	t.Parallel()

	env, err := parseSignalREnvelope([]byte(`{"type":4,"invocationId":"1","target":"priceFeed"}`))
	require.NoError(t, err)
	assert.Equal(t, 4, env.Type)
	assert.Equal(t, "1", env.InvocationID)
	assert.Equal(t, "priceFeed", env.Target)
}

/*
Scenario: Marshaling a decoded SignalR envelope back to JSON
Given a parsed envelope of type 2 (StreamItem)
When the envelope is marshaled
Then the JSON round-trips through the record-separator encoding

Related spec scenarios: RS.SHR.3, RS.SHR.16
*/
func TestSignalREnvelope_Marshal_Roundtrip(t *testing.T) {
	t.Parallel()

	env := signalREnvelope{Type: 2, InvocationID: "1", Item: map[string]any{"price": 1}}
	data, err := json.Marshal(env)
	require.NoError(t, err)
	back, err := parseSignalREnvelope(data)
	require.NoError(t, err)
	assert.Equal(t, 2, back.Type)
	assert.Equal(t, "1", back.InvocationID)
}
