package server

import (
	"encoding/json"
	"io"
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
Scenario: Deprecated ws alias for push still works
Given a server with the canonical async management routes
When a management push is sent to the deprecated /_mock/ws/push path
Then the request is accepted (200) and delivered to consumers

Related spec scenarios: RS.AMG.1, RS.AMG.6
*/
func TestDeprecatedAlias_Push(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()           //nolint:errcheck
	_, _, _ = conn.ReadMessage() // consume snapshot

	body := `{"channel":"/alerts","payload":{"msg":"alias"}}`
	resp, err := http.Post(ts.URL+"/_mock/ws/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"alias"`)
}

/*
Scenario: Deprecated ws alias for consumers still works
Given a server with the canonical async management routes and a connected consumer
When consumers are listed via the deprecated /_mock/ws/consumers path
Then the consumer list includes the connection id

Related spec scenarios: RS.AMG.8
*/
func TestDeprecatedAlias_Consumers(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

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

/*
Scenario: Deprecated ws alias for disconnect still works
Given a server with the canonical async management routes and an active consumer
When the consumer is disconnected via the deprecated /_mock/ws/disconnect path
Then the connection is closed and the request is accepted

Related spec scenarios: RS.AMG.14
*/
func TestDeprecatedAlias_Disconnect(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _ = conn.ReadMessage() // consume snapshot

	resp, err := http.Get(ts.URL + "/_mock/ws/consumers?channel=/alerts")
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

	disc := `{"connectionId":"` + connID + `","reason":"alias"}`
	discResp, err := http.Post(ts.URL+"/_mock/ws/disconnect", "application/json", strings.NewReader(disc))
	require.NoError(t, err)
	defer discResp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, discResp.StatusCode)

	// The disconnect must actually close the socket: a read on the consumer
	// should observe the close (EOF) rather than keep waiting for frames.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err = conn.ReadMessage()
	require.Error(t, err, "expected the connection to close after the alias disconnect")
}

/*
Scenario: Deprecated events/fire alias still works
Given a server with the canonical events endpoint and a fired-event subscription
When an event is fired via the deprecated /_mock/events/fire path
Then the templated message reaches a connected consumer

Related spec scenarios: RS.MAPI.22, RS.AMG.20
*/
func TestDeprecatedAlias_EventsFire(t *testing.T) {
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
	resp, err := http.Post(ts.URL+"/_mock/events/fire", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"high load"`)
}

/*
Scenario: The deprecated events/fire alias accepts a legacy type-less body
Given a server with the canonical events endpoint and a fired-event subscription
When an event is fired via the deprecated /_mock/events/fire path without a
'type' field
Then the legacy body is still honored and the message reaches a consumer

Related spec scenarios: RS.MAPI.22, RS.AMG.20
*/
func TestDeprecatedAlias_EventsFireLegacyBody(t *testing.T) {
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

	body := `{"event":"levelUp","payload":{"level":"warn","message":"legacy high load"}}`
	resp, err := http.Post(ts.URL+"/_mock/events/fire", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"legacy high load"`)
}

/*
Scenario: Removed schedule push answers 410 Gone
Given a server with the canonical examples endpoint
When a recurring push is scheduled via the removed /_mock/ws/schedule path
Then the server responds with HTTP 410 Gone pointing at POST /_mock/examples

Related spec scenarios: RS.AMG.12
*/
func TestRemovedScheduleRet_410(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","interval":50,"payload":{"tick":true}}`
	resp, err := http.Post(ts.URL+"/_mock/ws/schedule", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusGone, resp.StatusCode)

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(string(bodyBytes)), "/_mock/examples")
}

/*
Scenario: Removed schedule stop answers 410 Gone
Given a server with the canonical examples endpoint
When a schedule is stopped via the removed /_mock/ws/schedule/{pushId} path
Then the server responds with HTTP 410 Gone pointing at POST /_mock/examples

Related spec scenarios: RS.AMG.13
*/
func TestRemovedScheduleStop_410(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_mock/ws/schedule/push-123", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusGone, resp.StatusCode)
}
