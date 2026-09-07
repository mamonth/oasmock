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

const fireEventWsDoc = `asyncapi: 3.0.0
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
              level: "{$event.level}"
              msg: "{$event.message}"
            x-mock-match:
              '{$event.name}': "levelUp"
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

/*
Scenario: Firing an event via the management endpoint delivers to consumers
Given a management request firing a named event with a payload
When the fire-event endpoint is invoked with a connected consumer
Then the consumer receives the templated message

Related spec scenarios: RS.EVT.16, RS.EVT.17, RS.AMG.20, RS.AMG.21, RS.MAPI.22
*/
func TestFireEventEndpoint(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(fireEventWsDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	body := `{"type":"fire","event":"levelUp","payload":{"level":"warn","message":"high load"}}`
	resp, err := http.Post(ts.URL+"/_mock/events", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(msg, &payload))
	assert.Equal(t, "warn", payload["level"])
	assert.Equal(t, "high load", payload["msg"])
}

/*
Scenario: Fire-event endpoint accepts a delay
Given a management request firing an event with a positive delay
When the fire-event endpoint is invoked
Then the request is accepted with 200 (delivery scheduled asynchronously)

Related spec scenarios: RS.EVT.16, RS.AMG.20, RS.MAPI.22
*/
func TestFireEventEndpoint_Delayed(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(fireEventWsDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	body := `{"type":"fire","event":"levelUp","payload":{"level":"warn"},"delay":10}`
	resp, err := http.Post(ts.URL+"/_mock/events", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

/*
Scenario: Fire-event endpoint rejects a negative delay
Given a management request firing an event with a negative delay
When the fire-event endpoint is invoked
Then the server responds with HTTP 400

Related spec scenarios: RS.AMG.3
*/
func TestFireEventEndpoint_NegativeDelay(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(fireEventWsDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	body := `{"type":"fire","event":"levelUp","payload":{},"delay":-5}`
	resp, err := http.Post(ts.URL+"/_mock/events", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

/*
Scenario: Firing an event with a subscription but no connected consumers
Given a server with a subscribed channel and no connected consumers
When the fire-event endpoint is invoked
Then the request is accepted with 200 and no message is delivered

Related spec scenarios: RS.EVT.15, RS.MAPI.22
*/
func TestFireEventEndpoint_NoConsumers(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(fireEventWsDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	body := `{"type":"fire","event":"levelUp","payload":{"level":"warn","message":"high load"}}`
	resp, err := http.Post(ts.URL+"/_mock/events", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
