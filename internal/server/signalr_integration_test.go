package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const signalRIntegrationDoc = `asyncapi: 3.0.0
info:
  title: SignalR Hub
  version: 1.0.0
x-signalr:
  path: /hub
channels:
  priceFeed:
    address: priceFeed
    bindings:
      ws:
        method: GET
    messages:
      priceMsg:
        examples:
          - name: snap
            payload:
              symbol: ETH
              price: 3000
          - name: item2
            payload:
              symbol: BTC
              price: 60000
operations:
  getStatus:
    action: send
    channel:
      $ref: '#/channels/priceFeed'
    messages:
      - $ref: '#/channels/priceFeed/messages/priceMsg'
`

func newSignalRServer(t *testing.T) *Server {
	t.Helper()
	doc, err := asyncapi.Parse([]byte(signalRIntegrationDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)
	return srv
}

// dialSignalR wires a raw-framed SignalR client over the hub's ws upgrade.
func dialSignalR(t *testing.T, srv *Server) *websocket.Conn {
	t.Helper()
	ts := httptest.NewServer(srv.router)
	t.Cleanup(ts.Close)

	httpURL := ts.URL
	wsURL := "ws" + strings.TrimPrefix(httpURL, "http") + "/hub"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	if resp != nil {
		defer resp.Body.Close() //nolint:errcheck
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

/*
Scenario: Full SignalR stream lifecycle with a raw-framed client
Given a connected SignalR client over the hub
When the client handshakes, opens a stream by channel ID, and cancels
Then the server responds to the handshake, streams the snapshot, keeps the
stream open, and completes on cancellation

Related spec scenarios: RS.SHR.14, RS.SHR.16, RS.SHR.3, RS.SHR.4, RS.SHR.17
*/
func TestSignalR_StreamLifecycle(t *testing.T) {
	t.Parallel()

	srv := newSignalRServer(t)
	conn := dialSignalR(t, srv)

	// Handshake (RS.SHR.14).
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`)))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, hs, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "{}\x1e", string(hs))

	// StreamInvocation by channel ID (RS.SHR.3, RS.SHR.16).
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":4,"invocationId":"1","target":"priceFeed"}`+"\x1e")))
	_, snap, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(snap), "\x1e"))
	var item map[string]any
	require.NoError(t, json.Unmarshal(splitSignalRFrames(snap)[0], &item))
	assert.Equal(t, float64(signalRTypeStreamItem), item["type"])
	raw, _ := json.Marshal(item["item"])
	assert.Contains(t, string(raw), `"symbol":"ETH"`)

	// Stream stays open: next message is not a completion (RS.SHR.4). We send
	// a ping and expect a ping back, proving no completion was emitted.
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":6}`+"\x1e")))
	_, pong, err := conn.ReadMessage()
	require.NoError(t, err)
	var env signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(pong)[0], &env))
	assert.Equal(t, signalRTypePing, env.Type)

	// Cancel closes the stream with a completion (RS.SHR.17).
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":5,"invocationId":"1"}`+"\x1e")))
	_, comp, err := conn.ReadMessage()
	require.NoError(t, err)
	var env2 signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(comp)[0], &env2))
	assert.Equal(t, signalRTypeCompletion, env2.Type)
}

/*
Scenario: StreamInvocation with an unknown channel target
Given a SignalR client with a frame targeting a missing channel
When the server processes it
Then it replies with a Completion carrying an error

Related spec scenarios: RS.SHR.5
*/
func TestSignalR_UnknownChannelTarget(t *testing.T) {
	t.Parallel()

	srv := newSignalRServer(t)
	conn := dialSignalR(t, srv)
	handshakeSignalR(t, conn)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":4,"invocationId":"1","target":"nope"}`+"\x1e")))
	_, comp, err := conn.ReadMessage()
	require.NoError(t, err)
	var env signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(comp)[0], &env))
	assert.Equal(t, signalRTypeCompletion, env.Type)
	assert.Contains(t, env.Error, "unknown channel target")
}

/*
Scenario: One-shot Invocation by operation ID returns a Completion result
Given a SignalR client invoking an operation target
When the server processes the Invocation
Then it replies with a Completion carrying the operation's message example

Related spec scenarios: RS.SHR.6, RS.SHR.7
*/
func TestSignalR_OperationInvocation(t *testing.T) {
	t.Parallel()

	srv := newSignalRServer(t)
	conn := dialSignalR(t, srv)
	handshakeSignalR(t, conn)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":1,"invocationId":"2","target":"getStatus"}`+"\x1e")))
	_, comp, err := conn.ReadMessage()
	require.NoError(t, err)
	var env signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(comp)[0], &env))
	assert.Equal(t, signalRTypeCompletion, env.Type)
	raw, _ := json.Marshal(env.Result)
	assert.Contains(t, string(raw), `"symbol":"ETH"`)
}

/*
Scenario: Invocation with an unknown operation target returns an error completion
Given a SignalR client invoking a target matching no operation
When the server processes the Invocation
Then it replies with a Completion carrying an error

Related spec scenarios: RS.SHR.7
*/
func TestSignalR_UnknownOperationTarget(t *testing.T) {
	t.Parallel()

	srv := newSignalRServer(t)
	conn := dialSignalR(t, srv)
	handshakeSignalR(t, conn)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":1,"invocationId":"3","target":"missingOp"}`+"\x1e")))
	_, comp, err := conn.ReadMessage()
	require.NoError(t, err)
	var env signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(comp)[0], &env))
	assert.Equal(t, signalRTypeCompletion, env.Type)
	assert.Contains(t, env.Error, "unknown operation target")
}

func handshakeSignalR(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`)))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := conn.ReadMessage()
	require.NoError(t, err)
	_ = conn.SetReadDeadline(time.Time{})
}

/*
Scenario: Upgrade with an unknown token is rejected
Given a negotiated token
When a ws client upgrades with an unknown id token
Then the server rejects the upgrade with HTTP 404

Related spec scenarios: RS.SHR.12
*/
func TestSignalR_UpgradeUnknownToken(t *testing.T) {
	t.Parallel()

	srv := newSignalRServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/hub?id=unknown-token"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	}
}

/*
Scenario: Upgrade for an unsupported transport is rejected with 400
Given a ws client upgrading to the hub path with a non-WebSockets transport
When the upgrade is attempted
Then the server rejects it with HTTP 400

Related spec scenarios: RS.SHR.10
*/
func TestSignalR_UpgradeUnsupportedTransport(t *testing.T) {
	t.Parallel()

	srv := newSignalRServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/hub?transport=ServerSentEvents"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	}
}

/*
Scenario: Ping is echoed
Given a connected and handshaken SignalR client
When it sends a {type:6} ping frame
Then the server replies {type:6} without affecting streams

Related spec scenarios: RS.SHR.20
*/
func TestSignalR_PingEcho(t *testing.T) {
	t.Parallel()

	srv := newSignalRServer(t)
	conn := dialSignalR(t, srv)
	handshakeSignalR(t, conn)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":6}`+"\x1e")))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, pong, err := conn.ReadMessage()
	require.NoError(t, err)
	var env signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(pong)[0], &env))
	assert.Equal(t, signalRTypePing, env.Type)
}

/*
Scenario: Handshake with an unsupported protocol closes the connection
Given a SignalR client sending a messagepack handshake
When the server processes it
Then it sends a handshake error and closes the connection

Related spec scenarios: RS.SHR.15
*/
func TestSignalR_UnsupportedHandshake(t *testing.T) {
	t.Parallel()

	srv := newSignalRServer(t)
	conn := dialSignalR(t, srv)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"messagepack","version":1}`)))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, hs, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(hs), `"error"`)
	assert.True(t, strings.HasSuffix(string(hs), "\x1e"))
}

/*
Scenario: Event-driven item is appended to an open stream
Given a connected client with an open stream on a channel
When pushToStreams emits a payload for that channel
Then the client receives an additional StreamItem on the open invocationId

Related spec scenarios: RS.SHR.18, RS.EVT.13
*/
func TestSignalR_PushToOpenStream(t *testing.T) {
	t.Parallel()

	srv := newSignalRServer(t)
	conn := dialSignalR(t, srv)
	handshakeSignalR(t, conn)

	// Open a stream on priceFeed.
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":4,"invocationId":"s1","target":"priceFeed"}`+"\x1e")))
	_, _, err := conn.ReadMessage()
	require.NoError(t, err) // snapshot

	hub := srv.hubMgr.hubs[0]
	hub.pushToStreams("priceFeed", []byte(`{"symbol":"BTC","price":60000}`), "priceFeed")

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var env signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(msg)[0], &env))
	assert.Equal(t, signalRTypeStreamItem, env.Type)
	assert.Equal(t, "s1", env.InvocationID)
	raw, _ := json.Marshal(env.Item)
	assert.Contains(t, string(raw), `"symbol":"BTC"`)
}

/*
Scenario: Server Invocation push when no open stream matches
Given a connected SignalR client with no open stream on a channel
When pushToStreams emits a payload for that channel
Then the server sends an Invocation with a server-assigned id

Related spec scenarios: RS.SHR.19, RS.EVT.13
*/
func TestSignalR_PushWithoutOpenStream(t *testing.T) {
	t.Parallel()

	srv := newSignalRServer(t)
	conn := dialSignalR(t, srv)
	handshakeSignalR(t, conn)

	// No stream opened: place a marker by sending a ping after which we expect
	// only the server Invocation for the push.
	hub := srv.hubMgr.hubs[0]
	hub.pushToStreams("priceFeed", []byte(`{"symbol":"BTC","price":60000}`), "priceFeed")

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var env signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(msg)[0], &env))
	assert.Equal(t, signalRTypeInvocation, env.Type)
	assert.True(t, strings.HasPrefix(env.InvocationID, "srv-"))
	require.Len(t, env.Arguments, 1)
	raw, _ := json.Marshal(env.Arguments[0])
	assert.Contains(t, string(raw), `"symbol":"BTC"`)
}
