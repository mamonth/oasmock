package managapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
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

// streamCollector collects envelopes read from a _mock/stream connection so
// tests can assert on them asynchronously as the server pushes them.
type streamCollector struct {
	mu   sync.Mutex
	envs []map[string]any
}

// collectStream starts a deadline-free reader collecting envelopes from a
// _mock/stream connection. The reader exits when the connection is closed
// (any read error ends the loop); the test closes the connection via defer.
// No read deadlines are used because a gorilla ReadMessage error is permanent
// on a connection.
func collectStream(conn *websocket.Conn) *streamCollector {
	c := &streamCollector{}
	go func() {
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env map[string]any
			if json.Unmarshal(raw, &env) == nil {
				c.mu.Lock()
				c.envs = append(c.envs, env)
				c.mu.Unlock()
			}
		}
	}()
	return c
}

// find returns the first envelope matching the predicate.
func (c *streamCollector) find(match func(map[string]any) bool) (map[string]any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, env := range c.envs {
		if match(env) {
			return env, true
		}
	}
	return nil, false
}

// waitForEnv polls until an envelope matching the predicate arrives or the
// test times out.
func waitForEnv(t *testing.T, c *streamCollector, what string, match func(map[string]any) bool) map[string]any {
	t.Helper()
	var got map[string]any
	require.Eventuallyf(t, func() bool {
		env, ok := c.find(match)
		got = env
		return ok
	}, 3*time.Second, 50*time.Millisecond, "timed out waiting for envelope: %s", what)
	return got
}

// waitQuietEnv asserts no matching envelope arrives within a window.
func waitQuietEnv(t *testing.T, c *streamCollector, what string, dur time.Duration, match func(map[string]any) bool) {
	t.Helper()
	end := time.Now().Add(dur)
	for time.Now().Before(end) {
		time.Sleep(30 * time.Millisecond)
		if _, ok := c.find(match); ok {
			t.Fatalf("expected no envelope during quiet window: %s", what)
		}
	}
}

// envOfType returns the per-type payload of an envelope of the given type, or
// nil when the envelope is of a different type.
func envOfType(env map[string]any, typ string) map[string]any {
	if env["type"] != typ {
		return nil
	}
	sub, _ := env[typ].(map[string]any)
	return sub
}

// readUntilPayload reads frames from a consumer connection until a frame
// containing the given substring is seen, or the deadline passes.
func readUntilPayload(t *testing.T, conn *websocket.Conn, want string) string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	deadline := time.Now().Add(3 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		_, raw, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		got = string(raw)
		if strings.Contains(got, want) {
			break
		}
	}
	return got
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

/*
Scenario: A _mock/stream subscriber receives filtered event/push/consumer/schedule envelopes
Given a stream subscriber filtering events=levelUp and channels=/alerts, and an unfiltered subscriber
When an interval example starts and stops, a consumer connects, and the levelUp event fires
Then the filtered subscriber receives schedule/consumer/event/push envelopes but the connect event stays filtered out, while the unfiltered subscriber sees everything

Related spec scenarios: RS.AMG.23, RS.AMG.24, RS.AMG.25, RS.AMG.26, RS.AMG.27
*/
func TestIntegration_ManageStream_Envelopes(t *testing.T) {
	t.Parallel()
	port, stop := startManagementServer(t)
	defer stop()

	filtered := wsConnect(t, port, "/_mock/stream?events=levelUp&channels=/alerts")
	defer filtered.Close() //nolint:errcheck
	fCol := collectStream(filtered)

	// An omitted filter matches everything (RS.AMG.23).
	all := wsConnect(t, port, "/_mock/stream")
	defer all.Close() //nolint:errcheck
	allCol := collectStream(all)

	// Register an interval example -> schedule started (RS.AMG.27).
	addBody := `{"channel":"/alerts","interval":300,"response":{"code":200,"body":{"mytick":true}}}`
	addResp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/examples", port), "application/json", strings.NewReader(addBody))
	require.NoError(t, err)
	var add map[string]any
	require.NoError(t, json.NewDecoder(addResp.Body).Decode(&add))
	addResp.Body.Close() //nolint:errcheck
	require.Equal(t, 200, addResp.StatusCode)
	exampleID, _ := add["id"].(string)
	require.NotEmpty(t, exampleID)

	started := waitForEnv(t, fCol, "schedule started", func(env map[string]any) bool {
		sub := envOfType(env, "schedule")
		return sub != nil && sub["action"] == "started" && sub["channel"] == "/alerts" && sub["exampleId"] == exampleID
	})
	assert.EqualValues(t, 300, started["schedule"].(map[string]any)["interval"])

	// Connect a consumer -> consumer lifecycle envelope (RS.AMG.26); the
	// connect built-in event (name "connect") is filtered out on the events
	// filter but reaches the unfiltered subscriber (RS.AMG.23).
	conn := wsConnect(t, port, "/alerts")
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage() // connect built-in welcome

	waitForEnv(t, fCol, "consumer connected", func(env map[string]any) bool {
		sub := envOfType(env, "consumer")
		return sub != nil && sub["action"] == "connected" && sub["channel"] == "/alerts"
	})
	// The connect event violates the events=levelUp filter, so it can never
	// appear on the filtered subscriber even though the brand-new consumer
	// fired it (the broadcast is synchronous with the connect).
	waitQuietEnv(t, fCol, "filtered connect event", 400*time.Millisecond, func(env map[string]any) bool {
		sub := envOfType(env, "event")
		return sub != nil && sub["name"] == "connect"
	})
	// The unfiltered subscriber sees it.
	waitForEnv(t, allCol, "unfiltered connect event", func(env map[string]any) bool {
		sub := envOfType(env, "event")
		return sub != nil && sub["name"] == "connect"
	})

	// Fire levelUp -> event and push envelopes (RS.AMG.24, RS.AMG.25).
	_, err = http.Post(fmt.Sprintf("http://localhost:%d/_mock/events", port), "application/json",
		strings.NewReader(`{"type":"fire","event":"levelUp","payload":{"level":"warn","msg":"boom"}}`))
	require.NoError(t, err)

	waitForEnv(t, fCol, "levelUp event", func(env map[string]any) bool {
		sub := envOfType(env, "event")
		return sub != nil && sub["name"] == "levelUp"
	})
	waitForEnv(t, fCol, "levelUp push", func(env map[string]any) bool {
		sub := envOfType(env, "push")
		if sub == nil || sub["channel"] != "/alerts" {
			return false
		}
		payload, _ := sub["payload"].(map[string]any)
		return payload != nil && payload["msg"] == "boom"
	})

	// DELETE the interval example -> schedule stopped (RS.AMG.27).
	delReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://localhost:%d/_mock/examples/%s", port, exampleID), nil)
	require.NoError(t, err)
	delResp, err := http.DefaultClient.Do(delReq)
	require.NoError(t, err)
	_ = delResp.Body.Close()
	assert.Equal(t, 200, delResp.StatusCode)

	waitForEnv(t, fCol, "schedule stopped", func(env map[string]any) bool {
		sub := envOfType(env, "schedule")
		return sub != nil && sub["action"] == "stopped" && sub["exampleId"] == exampleID
	})
}

/*
Scenario: The schema's connect built-in fires on consumer connection
Given a channel whose spec declares an x-mock-match on {$event.name}: connect
When a ws consumer connects
Then the templated welcome example is delivered

Related spec scenarios: RS.EVT.24, RS.EXT.21, RS.EXT.26
*/
func TestIntegration_ConnectBuiltIn_Spec(t *testing.T) {
	t.Parallel()
	port, stop := startManagementServer(t)
	defer stop()

	conn := wsConnect(t, port, "/alerts")
	defer conn.Close() //nolint:errcheck

	got := readUntilPayload(t, conn, `"msg":"welcome"`)
	assert.Contains(t, got, `"msg":"welcome"`)
}

/*
Scenario: A runtime receive built-in example echoes inbound traffic
Given a POST /_mock/examples example matching {$event.name}: receive
When a consumer sends a client message on the channel
Then the templated message is emitted with the inbound payload in the context

Related spec scenarios: RS.EVT.25, RS.EVT.23, RS.MAPI.26, RS.EXT.21
*/
func TestIntegration_ReceiveBuiltIn_Runtime(t *testing.T) {
	t.Parallel()
	port, stop := startManagementServer(t)
	defer stop()

	addBody := `{"channel":"/alerts","match":{"{$event.name}":"receive"},"response":{"code":200,"body":{"echoed":"{$event.text}"}}}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/examples", port), "application/json", strings.NewReader(addBody))
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	conn := wsConnect(t, port, "/alerts")
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage() // connect built-in welcome

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"text":"hi"}`)))

	got := readUntilPayload(t, conn, `"echoed":"hi"`)
	assert.Contains(t, got, `"echoed":"hi"`)
}

/*
Scenario: The legacy x-send-events shim still emits on a deprecated subscription
Given a spec example declared with x-send-events: [{on: legacyAlert}]
When the legacyAlert event fires via POST /_mock/events
Then the shim-mapped example is emitted with the event payload

Related spec scenarios: RS.EVT.18
*/
func TestIntegration_XSendEventsShim_LegacyEmission(t *testing.T) {
	t.Parallel()
	port, stop := startManagementServer(t)
	defer stop()

	conn := wsConnect(t, port, "/alerts")
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage() // connect built-in welcome

	_, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/events", port), "application/json",
		strings.NewReader(`{"type":"fire","event":"legacyAlert","payload":{"level":"warn"}}`))
	require.NoError(t, err)

	got := readUntilPayload(t, conn, `"legacy":"warn"`)
	assert.Contains(t, got, `"legacy":"warn"`)
}

/*
Scenario: A non-upgrade request to the management stream is rejected
Given a plain HTTP GET to /_mock/stream
When the request is made
Then the server responds with HTTP 405

Related spec scenarios: RS.AMG.28
*/
func TestIntegration_ManageStream_NonUpgrade405(t *testing.T) {
	t.Parallel()
	port, stop := startManagementServer(t)
	defer stop()

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/_mock/stream", port))
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, 405, resp.StatusCode)
}
