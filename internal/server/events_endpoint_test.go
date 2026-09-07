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
Scenario: Events endpoint requires the type discriminator
Given a management request to /_mock/events without a type field
When the events endpoint is invoked
Then the server responds with HTTP 400

Related spec scenarios: RS.MAPI.32, RS.MAPI.22
*/
func TestEventsEndpoint_MissingType(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(fireEventWsDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	body := `{"event":"levelUp","payload":{"level":"warn"}}`
	resp, err := http.Post(ts.URL+"/_mock/events", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

/*
Scenario: Events endpoint rejects an unknown type
Given a management request to /_mock/events with an unsupported type
When the events endpoint is invoked
Then the server responds with HTTP 400

Related spec scenarios: RS.MAPI.32, RS.MAPI.22
*/
func TestEventsEndpoint_UnknownType(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(fireEventWsDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	body := `{"type":"explode","event":"levelUp","payload":{"level":"warn"}}`
	resp, err := http.Post(ts.URL+"/_mock/events", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

/*
Scenario: Firing an event with type fire reproduces the fire behavior
Given a management request to /_mock/events with type fire and a payload
When the events endpoint is invoked with a connected consumer
Then the consumer receives the templated message

Related spec scenarios: RS.MAPI.22, RS.AMG.20
*/
func TestEventsEndpoint_TypeFireDelivers(t *testing.T) {
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
Scenario: Fired-event payload expressions are templated against state and env
Given a management fired event whose payload references {$env.*} and {$state.*}
When the events endpoint is invoked with a connected consumer
Then the expressions are resolved before delivery (RS.MAPI.23)

Related spec scenarios: RS.MAPI.23, RS.AMG.20
*/
func TestEventsEndpoint_PayloadTemplating(t *testing.T) {
	t.Setenv("OASMOCK_EVENTS_TEST", "warn")
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

	body := `{"type":"fire","event":"levelUp","payload":{"level":"{$env.OASMOCK_EVENTS_TEST}","message":"high load"}}`
	resp, err := http.Post(ts.URL+"/_mock/events", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	_ = resp.Body.Close()
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
Scenario: The removed /events/fire alias answers 404
Given a server with the canonical events endpoint
When a fire request is posted to the removed /_mock/events/fire path
Then the server responds with HTTP 404 (the legacy alias is gone)

Related spec scenarios: RS.MAPI.22, RS.MAPI.32
*/
func TestEventsEndpoint_LegacyAliasGone(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(fireEventWsDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"type":"fire","event":"levelUp","payload":{"level":"warn"}}`
	resp, err := http.Post(ts.URL+"/_mock/events/fire", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
