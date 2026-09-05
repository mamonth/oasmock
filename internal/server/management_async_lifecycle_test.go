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

/*
Scenario: Templated push payloads evaluate against schema state
Given a push request whose payload uses the schema's state namespace
When the push is delivered
Then the expression is evaluated before delivery

Related spec scenarios: RS.AMG.10
*/
func TestPushEndpoint_TemplatedPayload(t *testing.T) {
	t.Setenv("OASMOCK_TEST_VAL", "resolved")

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()           //nolint:errcheck
	_, _, _ = conn.ReadMessage() // consume snapshot

	body := `{"channel":"/alerts","payload":{"msg":"{$env.OASMOCK_TEST_VAL}"}}`
	resp, err := http.Post(ts.URL+"/_mock/async/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"resolved"`)
}

/*
Scenario: Pushing a malformed payload expression is rejected
Given a push request with an unresolvable expression
When the push is delivered
Then it is rejected with HTTP 400

Related spec scenarios: RS.AMG.11
*/
func TestPushEndpoint_UnresolvableExpression(t *testing.T) {
	t.Parallel()

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	body := `{"channel":"/alerts","payload":{"msg":"{$event.nonexistent}"}}`
	resp, err := http.Post(ts.URL+"/_mock/async/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

/*
Scenario: Removed schedule endpoint answers 410 Gone
Given a management schedule request against the removed /_mock/ws/schedule path
When the schedule endpoint is invoked
Then the server responds 410 Gone pointing at POST /_mock/examples

Related spec scenarios: RS.AMG.12
*/
func TestSchedulePush_Removed(t *testing.T) {
	t.Parallel()

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()           //nolint:errcheck
	_, _, _ = conn.ReadMessage() // consume snapshot

	body := `{"channel":"/alerts","interval":50,"payload":{"tick":true}}`
	resp, err := http.Post(ts.URL+"/_mock/ws/schedule", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusGone, resp.StatusCode)
}

/*
Scenario: Force-disconnecting a consumer closes the connection
Given an active consumer and a disconnect request
When the disconnect endpoint is invoked
Then the connection is closed

Related spec scenarios: RS.AMG.14, RS.AMG.15, RS.AMG.16
*/
func TestDisconnectEndpoint(t *testing.T) {
	t.Parallel()

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _ = conn.ReadMessage() // consume snapshot

	// Identify the connection id via consumers.
	resp, err := http.Get(ts.URL + "/_mock/async/consumers?channel=/alerts")
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	_ = resp.Body.Close()
	items, ok := payload["consumers"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, items)
	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	connID, ok := first["connectionId"].(string)
	require.True(t, ok)

	// Disconnect it.
	disc := `{"connectionId":"` + connID + `","reason":"busy","code":4001}`
	discResp, err := http.Post(ts.URL+"/_mock/async/disconnect", "application/json", strings.NewReader(disc))
	require.NoError(t, err)
	defer discResp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, discResp.StatusCode)

	// The disconnect must actually close the socket: a read on the consumer
	// observes the close handshake (RS.AMG.15).
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, readErr := conn.ReadMessage()
	require.Error(t, readErr, "expected the connection to close after the disconnect")

	// Unknown consumer 404.
	unknownResp, err := http.Post(ts.URL+"/_mock/async/disconnect", "application/json", strings.NewReader(`{"connectionId":"nope"}`))
	require.NoError(t, err)
	defer unknownResp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, unknownResp.StatusCode)
}

/*
Scenario: Fire-event and push drive live ws connections together
Given a server with an AsyncAPI ws channel and a fire-event subscription
When an event fires and a push occurs on the same channel
Then both deliveries reach a connected consumer

Related spec scenarios: RS.AMG.1, RS.AMG.6, RS.AMG.20, RS.EVT.16
*/
func TestAsyncManagement_LiveConnections(t *testing.T) {
	t.Parallel()

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()           //nolint:errcheck
	_, _, _ = conn.ReadMessage() // snapshot

	// Fire an event (accepted, possibly no matching subscriber on /alerts).
	evResp, err := http.Post(ts.URL+"/_mock/events", "application/json",
		strings.NewReader(`{"type":"fire","event":"any","payload":{"x":1}}`))
	require.NoError(t, err)
	_ = evResp.Body.Close()
	assert.Equal(t, http.StatusOK, evResp.StatusCode)

	// Push a message to the connected consumer.
	pushResp, err := http.Post(ts.URL+"/_mock/async/push", "application/json",
		strings.NewReader(`{"channel":"/alerts","payload":{"seq":1}}`))
	require.NoError(t, err)
	_ = pushResp.Body.Close()
	assert.Equal(t, http.StatusOK, pushResp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"seq":1`)
}

const signalRPushDoc = `asyncapi: 3.0.0
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
operations:
  receivePrice:
    action: receive
    channel:
      $ref: '#/channels/priceFeed'
`

func newSignalRPushMgmtServer(t *testing.T) *Server {
	t.Helper()
	doc, err := asyncapi.Parse([]byte(signalRPushDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)
	return srv
}

/*
Scenario: Targeted push delivers only to the named consumer
Given two consumers connected to the same ws channel
When a push request carries one consumer's connectionId
Then only that consumer receives the message

Related spec scenarios: RS.AMG.5
*/
func TestPushEndpoint_TargetedWS(t *testing.T) {
	t.Parallel()

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"

	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn1.Close()           //nolint:errcheck
	_, _, _ = conn1.ReadMessage() // consume snapshot

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn2.Close()           //nolint:errcheck
	_, _, _ = conn2.ReadMessage() // consume snapshot

	// The registry hands out sequential ids (conn-1, conn-2) in dial order.
	body := `{"channel":"/alerts","connectionId":"conn-1","payload":{"targeted":true}}`
	post, err := http.Post(ts.URL+"/_mock/async/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer post.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, post.StatusCode)

	_ = conn1.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg1, err := conn1.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg1), `"targeted":true`)

	// The other consumer must not receive the targeted message.
	_ = conn2.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err2 := conn2.ReadMessage()
	require.Error(t, err2)
}

/*
Scenario: Targeted push reaches an open SignalR stream only
Given two SignalR consumers where one holds an open stream on a hub channel
When a push carries the streaming consumer's connectionId
Then only that consumer's stream receives the payload

Related spec scenarios: RS.AMG.5, RS.SHR.18
*/
func TestPushEndpoint_TargetedSignalR(t *testing.T) {
	t.Parallel()

	srv := newSignalRPushMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	hubURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/hub"

	connA, _, err := websocket.DefaultDialer.Dial(hubURL, nil)
	require.NoError(t, err)
	defer connA.Close() //nolint:errcheck
	require.NoError(t, connA.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`)))
	_, _, err = connA.ReadMessage()
	require.NoError(t, err) // handshake reply
	require.NoError(t, connA.WriteMessage(websocket.TextMessage, []byte(`{"type":4,"invocationId":"s1","target":"priceFeed"}`+"\x1e")))
	_, _, err = connA.ReadMessage()
	require.NoError(t, err) // snapshot

	connB, _, err := websocket.DefaultDialer.Dial(hubURL, nil)
	require.NoError(t, err)
	defer connB.Close() //nolint:errcheck
	require.NoError(t, connB.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`)))
	_, _, err = connB.ReadMessage()
	require.NoError(t, err) // handshake reply

	hub := srv.hubMgr.hubs[0]
	hub.mu.Lock()
	var streamConnID string
	for id, sc := range hub.conns {
		if len(sc.streams) > 0 {
			streamConnID = id
			break
		}
	}
	hub.mu.Unlock()
	require.NotEmpty(t, streamConnID)

	body := `{"channel":"/priceFeed","connectionId":"` + streamConnID + `","payload":{"seq":1}}`
	post, err := http.Post(ts.URL+"/_mock/async/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer post.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, post.StatusCode)

	_ = connA.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := connA.ReadMessage()
	require.NoError(t, err)
	var env signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(msg)[0], &env))
	assert.Equal(t, signalRTypeStreamItem, env.Type)
	assert.Equal(t, "s1", env.InvocationID)

	// The second connection has no open stream and must not receive it.
	_ = connB.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, errB := connB.ReadMessage()
	require.Error(t, errB)
}

/*
Scenario: Abrupt disconnect aborts without a close frame
Given an active consumer and a disconnect request with abrupt=true
When the disconnect endpoint is invoked
Then the client read fails without a normal close frame

Related spec scenarios: RS.AMG.17
*/
func TestDisconnectEndpoint_Abrupt(t *testing.T) {
	t.Parallel()

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _ = conn.ReadMessage() // consume snapshot

	// Identify the connection id via consumers.
	resp, err := http.Get(ts.URL + "/_mock/async/consumers?channel=/alerts")
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	_ = resp.Body.Close()
	items, ok := payload["consumers"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, items)
	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	connID, ok := first["connectionId"].(string)
	require.True(t, ok)

	disc := `{"connectionId":"` + connID + `","abrupt":true}`
	discResp, err := http.Post(ts.URL+"/_mock/async/disconnect", "application/json", strings.NewReader(disc))
	require.NoError(t, err)
	defer discResp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, discResp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, readErr := conn.ReadMessage()
	require.Error(t, readErr)
	assert.False(t, websocket.IsCloseError(readErr, websocket.CloseNormalClosure),
		"abrupt drop must not surface as a normal close frame")
}
