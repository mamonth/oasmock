package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Registering a named-event runtime example via /_mock/examples
Given a POST to /_mock/examples with a channel and an event match
When the event fires via /_mock/events
Then the registered message is delivered to the channel's consumers

Related spec scenarios: RS.MAPI.24, RS.EXT.18
*/
func TestAddExample_RuntimeEventMatch(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","match":{"{$event.name}":"levelUp"},"response":{"code":200,"body":{"msg":"{$event.msg}"}}}`
	resp := postExample(t, ts.URL, body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var addResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&addResp))
	resp.Body.Close() //nolint:errcheck
	require.NotEmpty(t, addResp["id"])

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	waitForConnections(srv, "/alerts", 1)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _ = conn.ReadMessage() // consume the connect snapshot

	fire, err := http.Post(ts.URL+"/_mock/events", "application/json",
		strings.NewReader(`{"type":"fire","event":"levelUp","payload":{"msg":"boom"}}`))
	require.NoError(t, err)
	_ = fire.Body.Close()
	assert.Equal(t, http.StatusOK, fire.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	deadline := time.Now().Add(3 * time.Second)
	got := ""
	for time.Now().Before(deadline) {
		_, msg, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		got = string(msg)
		if strings.Contains(got, `"boom"`) {
			break
		}
	}
	assert.Contains(t, got, `"msg":"boom"`)
}

/*
Scenario: Registering an interval runtime example via /_mock/examples
Given a POST to /_mock/examples with a channel and an interval
When the server runs
Then the message is delivered repeatedly at the interval until removed

Related spec scenarios: RS.MAPI.25, RS.EVT.26
*/
func TestAddExample_RuntimeInterval(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","interval":40,"response":{"code":200,"body":{"tick":"{$state.counter}"}}}`
	resp := postExample(t, ts.URL, body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var addResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&addResp))
	resp.Body.Close() //nolint:errcheck
	exampleID, _ := addResp["id"].(string)
	require.NotEmpty(t, exampleID)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	waitForConnections(srv, "/alerts", 1)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _ = conn.ReadMessage() // consume the connect snapshot

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"tick"`)

	// Stop by removing the example.
	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_mock/examples/"+exampleID, nil)
	require.NoError(t, err)
	delResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer delResp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, delResp.StatusCode)

	// The interval job must be cancelled: no further deliveries after a quiet
	// window (5x the cadence), proving DELETE stops recurring delivery.
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	require.Error(t, err, "expected no interval deliveries after DELETE")
}

/*
Scenario: A runtime connect match delivers to the connecting consumer
Given a POST to /_mock/examples with a connect match
When a consumer connects
Then the registered message is delivered to that consumer

Related spec scenarios: RS.MAPI.26
*/
func TestAddExample_RuntimeConnectMatch(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","match":{"{$event.name}":"connect"},"response":{"code":200,"body":{"msg":"welcome"}}}`
	resp := postExample(t, ts.URL, body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close() //nolint:errcheck

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	deadline := time.Now().Add(3 * time.Second)
	got := ""
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		_, msg, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		got = string(msg)
		if strings.Contains(got, `"msg":"welcome"`) {
			break
		}
	}
	assert.Contains(t, got, `"msg":"welcome"`)
}

/*
Scenario: Example ids are namespaced by registry kind
Given a valid sync and async add-example request
When /_mock/examples is invoked
Then the sync example id carries the "dynex-" prefix and the async runtime
example id carries the "rtex-" prefix, keeping the two registries disjoint

Related spec scenarios: RS.MAPI.30, RS.MAPI.31
*/
func TestAddExample_IdsAreNamespaced(t *testing.T) {
	t.Parallel()

	asyncSrv := newPushServer(t)
	asyncTS := httptest.NewServer(asyncSrv.router)
	defer asyncTS.Close() //nolint:errcheck

	body := `{"channel":"/alerts","match":{"{$event.name}":"levelUp"},"response":{"code":200,"body":{"a":1}}}`
	asyncResp := postExample(t, asyncTS.URL, body)
	require.Equal(t, http.StatusOK, asyncResp.StatusCode)
	var asyncPayload map[string]any
	require.NoError(t, json.NewDecoder(asyncResp.Body).Decode(&asyncPayload))
	asyncResp.Body.Close() //nolint:errcheck
	asyncID, _ := asyncPayload["id"].(string)
	assert.True(t, strings.HasPrefix(asyncID, "rtex-"), "async runtime id prefix: %s", asyncID)

	spec, err := openapi3.NewLoader().LoadFromData([]byte(syncDeleteOpenAPI))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindOpenAPI, Spec: spec, Prefix: ""}}
	syncSrv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)
	syncTS := httptest.NewServer(syncSrv.router)
	defer syncTS.Close() //nolint:errcheck

	syncResp := postExample(t, syncTS.URL, `{"path":"/ping","method":"GET","response":{"code":200,"body":{"a":1}}}`)
	require.Equal(t, http.StatusOK, syncResp.StatusCode)
	var syncPayload map[string]any
	require.NoError(t, json.NewDecoder(syncResp.Body).Decode(&syncPayload))
	syncResp.Body.Close() //nolint:errcheck
	syncID, _ := syncPayload["id"].(string)
	assert.True(t, strings.HasPrefix(syncID, "dynex-"), "sync dynamic id prefix: %s", syncID)

	assert.NotEqual(t, asyncID, syncID)
}
