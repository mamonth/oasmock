package managapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/test/_shared/clihelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startManagementServer(t *testing.T) (int, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../../_shared/resources/asyncapi-management.yaml", "").Run()
	cleanup := func() { clihelper.StopServer(t, cmd) }
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}
	_ = errCh
	return port, cleanup
}

func wsConnect(t *testing.T, port int, path string) *websocket.Conn {
	t.Helper()
	url := fmt.Sprintf("ws://localhost:%d%s", port, path)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	return conn
}

/*
Scenario: Runtime match on {$event.name} fired via POST /_mock/events
Given a running server with a match on {$event.name} and a connected consumer
When POST /_mock/events fires the named event
Then the templated message reaches the consumer

Related spec scenarios: RS.MAPI.24, RS.EXT.18, RS.MAPI.22
*/
func TestIntegration_EventMatchFired(t *testing.T) {
	t.Parallel()
	port, stop := startManagementServer(t)
	defer stop()

	conn := wsConnect(t, port, "/alerts")
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage() // consume welcome snapshot

	req, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/events", port), "application/json",
		strings.NewReader(`{"type":"fire","event":"levelUp","payload":{"level":"warn","msg":"boom"}}`))
	require.NoError(t, err)
	_ = req.Body.Close()
	assert.Equal(t, 200, req.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	deadline := time.Now().Add(3 * time.Second)
	got := ""
	for time.Now().Before(deadline) {
		_, raw, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		got = string(raw)
		if strings.Contains(got, `"boom"`) {
			break
		}
	}
	assert.Contains(t, got, `"level":"warn"`)
	assert.Contains(t, got, `"msg":"boom"`)
}

/*
Scenario: A runtime interval example recurs then stops via DELETE
Given a registered interval example and a connected consumer
When DELETE /_mock/examples/{id} cancels it
Then the recurring delivery stops

Related spec scenarios: RS.MAPI.25, RS.MAPI.30, RS.EVT.26
*/
func TestIntegration_RuntimeIntervalThenStop(t *testing.T) {
	t.Parallel()
	port, stop := startManagementServer(t)
	defer stop()

	conn := wsConnect(t, port, "/alerts")
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage() // welcome

	addBody := `{"channel":"/alerts","interval":200,"response":{"code":200,"body":{"mytick":true}}}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/examples", port), "application/json", strings.NewReader(addBody))
	require.NoError(t, err)
	var add map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&add))
	resp.Body.Close() //nolint:errcheck
	require.Equal(t, 200, resp.StatusCode)
	exampleID, _ := add["id"].(string)
	require.NotEmpty(t, exampleID)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	deadline := time.Now().Add(3 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		_, raw, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		if strings.Contains(string(raw), `"mytick"`) {
			found = true
			break
		}
	}
	require.True(t, found, "expected a first mytick delivery")

	// Delete the example; assert success and then no further mytick deliveries
	// at the cadence. A single in-flight tick may already be in the pipe, so
	// drain it first, then require a quiet window.
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://localhost:%d/_mock/examples/%s", port, exampleID), nil)
	require.NoError(t, err)
	delResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = delResp.Body.Close()
	assert.Equal(t, 200, delResp.StatusCode)

	wait := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(wait) {
		_ = conn.SetReadDeadline(wait)
		_, raw, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		t.Logf("drained %s", string(raw))
	}
	_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	_, _, rerr := conn.ReadMessage()
	require.Error(t, rerr, "no mytick delivery should occur after DELETE")
}

/*
Scenario: Per-connection targeting via {$connection.id}
Given an event match that also narrows by {$connection.id}
When the event fires with a connectionId payload
Then only the matching consumer receives the message

Related spec scenarios: RS.EVT.19, RS.EXT.24, RS.MAPI.33
*/
func TestIntegration_ConnectionTargeting(t *testing.T) {
	t.Parallel()
	port, stop := startManagementServer(t)
	defer stop()

	addBody := `{"channel":"/alerts","match":{"{$event.name}":"levelUp","{$connection.id}":"{$event.connectionId}"},"response":{"code":200,"body":{"ring":"{$event.data}"}}}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/examples", port), "application/json", strings.NewReader(addBody))
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	conn1 := wsConnect(t, port, "/alerts")
	defer conn1.Close() //nolint:errcheck
	conn2 := wsConnect(t, port, "/alerts")
	defer conn2.Close() //nolint:errcheck
	_ = conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn1.ReadMessage() // welcome
	_ = conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn2.ReadMessage() // welcome

	// Find the first connection's id via the consumers endpoint.
	creq, err := http.Get(fmt.Sprintf("http://localhost:%d/_mock/async/consumers?channel=/alerts", port))
	require.NoError(t, err)
	var cpayload map[string]any
	require.NoError(t, json.NewDecoder(creq.Body).Decode(&cpayload))
	creq.Body.Close() //nolint:errcheck
	items, ok := cpayload["consumers"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, items)
	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	connID, ok := first["connectionId"].(string)
	require.True(t, ok)

	body := `{"type":"fire","event":"levelUp","payload":{"connectionId":"` + connID + `"}}`
	_, err = http.Post(fmt.Sprintf("http://localhost:%d/_mock/events", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)

	// Exactly one of the two connections receives the ring with the targeted
	// connection id; the other receives nothing.
	readWith := func(conn *websocket.Conn) string {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			_, raw, rerr := conn.ReadMessage()
			if rerr != nil {
				return ""
			}
			if strings.Contains(string(raw), `"ring"`) {
				return string(raw)
			}
		}
		return ""
	}
	r1 := readWith(conn1)
	r2 := readWith(conn2)

	ringCount := 0
	if r1 != "" {
		assert.Contains(t, r1, fmt.Sprintf(`"connectionId":"%s"`, connID))
		ringCount++
	}
	if r2 != "" {
		assert.Contains(t, r2, fmt.Sprintf(`"connectionId":"%s"`, connID))
		ringCount++
	}
	assert.Equal(t, 1, ringCount, "exactly one consumer should receive the targeted ring")
}

/*
Scenario: The deprecated /_mock/ws/push alias still works
Given a connected consumer and a push to the deprecated path
When the push is invoked
Then the message reaches the consumer

Related spec scenarios: RS.AMG.1, RS.AMG.6
*/
func TestIntegration_DeprecatedAliasStillWorks(t *testing.T) {
	t.Parallel()
	port, stop := startManagementServer(t)
	defer stop()

	conn := wsConnect(t, port, "/alerts")
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage() // welcome

	body := `{"channel":"/alerts","payload":{"alias":true}}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/ws/push", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	deadline := time.Now().Add(3 * time.Second)
	got := ""
	for time.Now().Before(deadline) {
		_, raw, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		got = string(raw)
		if strings.Contains(got, `"alias":true`) {
			break
		}
	}
	assert.Contains(t, got, `"alias":true`)
}

/*
Scenario: The removed schedule endpoint answers 410
Given a request to the removed /_mock/ws/schedule path
When the schedule endpoint is invoked
Then the server responds with 410 Gone

Related spec scenarios: RS.AMG.12, RS.AMG.13
*/
func TestIntegration_ScheduleGone410(t *testing.T) {
	t.Parallel()
	port, stop := startManagementServer(t)
	defer stop()

	body := `{"channel":"/alerts","interval":50,"payload":{"tick":true}}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/ws/schedule", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, 410, resp.StatusCode)
}
