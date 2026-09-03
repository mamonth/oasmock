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

const pushChannelDoc = `asyncapi: 3.0.0
info:
  title: Alerts
  version: 1.0.0
channels:
  alerts:
    address: /alerts
    bindings:
      ws:
        method: GET
    messages:
      alertMsg:
        examples:
          - name: ex1
            payload:
              level: info
              msg: default
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

func newPushServer(t *testing.T) *Server {
	t.Helper()
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: parsePushDoc(t), Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)
	return srv
}

func parsePushDoc(t *testing.T) *asyncapi.Document {
	t.Helper()
	doc, err := asyncapi.Parse([]byte(pushChannelDoc))
	require.NoError(t, err)
	return doc
}

/*
Scenario: Pushing a message to channel consumers immediately
Given a management push request without a delay and a connected consumer
When the push endpoint is invoked
Then the consumer receives the message

Related spec scenarios: RS.AMG.1, RS.AMG.2, RS.AMG.6
*/
func TestPushEndpoint_Immediate(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	// Consume the receive-operation snapshot emitted on connect.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)

	body := `{"channel":"/alerts","payload":{"msg":"hello"}}`
	resp, err := http.Post(ts.URL+"/_mock/ws/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.JSONEq(t, `{"msg":"hello"}`, string(msg))
}

/*
Scenario: Pushing with a negative delay is rejected
Given a management push request with a negative delay
When the push endpoint is invoked
Then the server responds with HTTP 400

Related spec scenarios: RS.AMG.3
*/
func TestPushEndpoint_NegativeDelay(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	body := `{"channel":"/alerts","payload":{},"delay":-10}`
	resp, err := http.Post(ts.URL+"/_mock/ws/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

/*
Scenario: Pushing to a channel with no consumers is accepted
Given a management push request to a valid channel with no consumers
When the push endpoint is invoked
Then the server accepts the request without error

Related spec scenarios: RS.AMG.4
*/
func TestPushEndpoint_NoConsumers(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	body := `{"channel":"/alerts","payload":{"msg":"none"}}`
	resp, err := http.Post(ts.URL+"/_mock/ws/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

/*
Scenario: Unknown consumer reference returns 404
Given a management push request targeting an unknown connection id
When the push endpoint is invoked
Then the server responds with HTTP 404

Related spec scenarios: RS.AMG.7
*/
func TestPushEndpoint_UnknownConsumer(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	body := `{"channel":"/alerts","payload":{},"connectionId":"missing"}`
	resp, err := http.Post(ts.URL+"/_mock/ws/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

/*
Scenario: Listing connected consumers per channel
Given a connected ws consumer
When the consumers endpoint is queried
Then the consumer list includes the connection id

Related spec scenarios: RS.AMG.8, RS.AMG.9
*/
func TestConsumersEndpoint(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	resp, err := http.Get(ts.URL + "/_mock/ws/consumers?channel=/alerts")
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	items, ok := payload["consumers"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, items)
}
