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

const streamLifecycleDoc = `asyncapi: 3.0.0
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
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

// readManageEnvelopes reads up to n envelopes, collecting matches for predicate.
func readManageEnvelopes(t *testing.T, conn *websocket.Conn, n int, match func(manageEnvelope) bool) []manageEnvelope {
	t.Helper()
	var out []manageEnvelope
	deadline := time.Now().Add(3 * time.Second)
	for len(out) < n && time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		_, raw, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		var env manageEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}
		if match(env) {
			out = append(out, env)
		}
	}
	return out
}

/*
Scenario: Consumer lifecycle envelopes are emitted on connect/disconnect
Given a stream subscriber and a raw ws consumer on a channel
When the consumer connects and disconnects
Then the subscriber receives consumer envelopes for both lifecycle events

Related spec scenarios: RS.AMG.26
*/
func TestManageStream_ConsumerLifecycleEnvelopes(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(streamLifecycleDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	stream := dialManageStream(t, ts.URL, "channels=/alerts")
	defer stream.Close() //nolint:errcheck

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	envs := readManageEnvelopes(t, stream, 1, func(e manageEnvelope) bool {
		return e.Type == "consumer" && e.Consumer != nil && e.Consumer.Action == "connected"
	})
	require.Len(t, envs, 1)
	assert.Equal(t, "/alerts", envs[0].Consumer.Channel)
	assert.NotEmpty(t, envs[0].Consumer.ConnectionID)

	// Disconnect the consumer and expect a disconnected envelope.
	conn.Close() //nolint:errcheck
	disc := readManageEnvelopes(t, stream, 1, func(e manageEnvelope) bool {
		return e.Type == "consumer" && e.Consumer != nil && e.Consumer.Action == "disconnected"
	})
	assert.Len(t, disc, 1)
}

/*
Scenario: Schedule envelopes are emitted on interval start and stop
Given a stream subscriber and a POST to /_mock/examples with an interval
When the interval example is added then removed
Then the subscriber receives schedule started/stopped envelopes

Related spec scenarios: RS.AMG.27
*/
func TestManageStream_ScheduleEnvelopes(t *testing.T) {
	t.Parallel()

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	stream := dialManageStream(t, ts.URL, "channels=/alerts")
	defer stream.Close() //nolint:errcheck

	body := `{"channel":"/alerts","interval":50,"response":{"code":200,"body":{"tick":true}}}`
	resp := postExample(t, ts.URL, body)
	var addResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&addResp))
	resp.Body.Close() //nolint:errcheck
	exampleID, _ := addResp["id"].(string)
	require.NotEmpty(t, exampleID)

	started := readManageEnvelopes(t, stream, 1, func(e manageEnvelope) bool {
		return e.Type == "schedule" && e.Schedule != nil && e.Schedule.Action == "started"
	})
	require.Len(t, started, 1)
	assert.Equal(t, exampleID, started[0].Schedule.ExampleID)
	assert.Equal(t, "/alerts", started[0].Schedule.Channel)
	assert.Equal(t, 50, started[0].Schedule.Interval)

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_mock/examples/"+exampleID, nil)
	require.NoError(t, err)
	delResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = delResp.Body.Close()
	assert.Equal(t, http.StatusOK, delResp.StatusCode)

	stopped := readManageEnvelopes(t, stream, 1, func(e manageEnvelope) bool {
		return e.Type == "schedule" && e.Schedule != nil && e.Schedule.Action == "stopped"
	})
	require.Len(t, stopped, 1)
	// The stopped envelope carries the same example identity, channel and
	// interval as the started one so stream clients can correlate the pair.
	assert.Equal(t, started[0].Schedule.ExampleID, stopped[0].Schedule.ExampleID)
	assert.Equal(t, "/alerts", stopped[0].Schedule.Channel)
	assert.Equal(t, 50, stopped[0].Schedule.Interval)
}

/*
Scenario: A silently-dead management stream subscriber is reaped
Given a running server with a short management-stream read idle and a connected
/mock/stream subscriber that stops sending frames
When the idle interval elapses
Then the subscriber is removed from the management stream registry and its
handler goroutine returns

Related spec scenarios: RS.AMG.29
*/
func TestManageStream_ReapsIdleSubscriber(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(streamLifecycleDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)
	// Force a short idle so the reaping path is exercised without waiting on
	// the 60s production bound.
	srv.manageStream.mu.Lock()
	srv.manageStream.readIdle = 60 * time.Millisecond
	srv.manageStream.mu.Unlock()

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	stream := dialManageStream(t, ts.URL, "")
	// Do not read: a silently-dead peer.
	time.Sleep(300 * time.Millisecond)

	srv.manageStream.mu.RLock()
	subCount := len(srv.manageStream.subs)
	srv.manageStream.mu.RUnlock()
	_ = stream.Close() //nolint:errcheck
	assert.Zero(t, subCount, "idle subscriber must be reaped from the registry")
}
