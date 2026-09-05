package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dialManageStream connects to /_mock/stream with optional filters.
func dialManageStream(t *testing.T, tsURL, query string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(tsURL, "http") + "/_mock/stream"
	if query != "" {
		url += "?" + query
	}
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	return conn
}

/*
Scenario: A plain HTTP GET to the stream endpoint is rejected
Given a non-upgrade request to /_mock/stream
When the request is served
Then the server responds with HTTP 405

Related spec scenarios: RS.AMG.28
*/
func TestManageStream_PlainHTTPRejected(t *testing.T) {
	t.Parallel()

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	resp, err := http.Get(ts.URL + "/_mock/stream")
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

/*
Scenario: A ws client connects and receives envelopes
Given a management stream subscriber and a fired event
When an event fires with a connected consumer
Then the subscriber receives an event envelope

Related spec scenarios: RS.AMG.23, RS.AMG.24
*/
func TestManageStream_ReceivesEventEnvelope(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(fireEventWsDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	stream := dialManageStream(t, ts.URL, "events=levelUp")
	defer stream.Close() //nolint:errcheck

	// Connect a consumer on the /alerts channel.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	resp, err := http.Post(ts.URL+"/_mock/events", "application/json",
		strings.NewReader(`{"type":"fire","event":"levelUp","payload":{"level":"warn"}}`))
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	deadline := time.Now().Add(3 * time.Second)
	var gotEnv manageEnvelope
	for time.Now().Before(deadline) {
		_ = stream.SetReadDeadline(deadline)
		_, raw, rerr := stream.ReadMessage()
		if rerr != nil {
			break
		}
		var env manageEnvelope
		require.NoError(t, json.Unmarshal(raw, &env))
		if env.Type == "event" && env.Event != nil && env.Event.Name == "levelUp" {
			gotEnv = env
			break
		}
	}
	assert.Equal(t, "event", gotEnv.Type)
	require.NotNil(t, gotEnv.Event)
	assert.Equal(t, "levelUp", gotEnv.Event.Name)
}

/*
Scenario: Parsing connect-time event and channel filters
Given a stream URL with comma-separated events and channels globs
When parseStreamFilters runs
Then the filters are parsed into the subscriber's filter

Related spec scenarios: RS.AMG.23
*/
func TestManageStream_FiltersParsed(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/_mock/stream?events=orderCreated,levelUp&channels=/*,x", nil)
	f := parseStreamFilters(req)
	assert.ElementsMatch(t, []string{"orderCreated", "levelUp"}, f.events)
	assert.ElementsMatch(t, []string{"/*", "x"}, f.channels)
}

/*
Scenario: Ping keepalive stops after the stream connection closes
Given a management stream subscriber with a ping loop
When the connection drops and the stop signal closes
Then the ping goroutine exits and issues no further pings

Related spec scenarios: RS.AMG.23
*/
func TestServePings_StopsAfterStop(t *testing.T) {
	t.Parallel()

	rec := &pingRecorder{}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		servePings(rec, 5*time.Millisecond, stop)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for rec.count() < 3 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	require.GreaterOrEqual(t, rec.count(), 3)

	close(stop)
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("servePings did not exit after the stop signal")
	}

	after := rec.count()
	time.Sleep(15 * time.Millisecond)
	assert.Equal(t, after, rec.count(), "no ping may be issued after the stop signal")
}

// pingRecorder counts ping frames written through a pingWriter.
type pingRecorder struct {
	mu    sync.Mutex
	sends int
}

func (p *pingRecorder) writeMessage(_ int, _ []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sends++
}

func (p *pingRecorder) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sends
}

/*
Scenario: Glob matching covers leading, middle and trailing wildcards
Given a glob pattern with '*' in any position
When globMatch compares it to a value
Then the value matches when the literal segments appear in order

Related spec scenarios: RS.AMG.23
*/
func TestGlobMatch_WildcardPositions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{name: "leading star", pattern: "*lerts", value: "alerts", want: true},
		{name: "leading segment", pattern: "*level", value: "orderCreated, levelUp.level", want: true},
		{name: "mid star", pattern: "alert*up", value: "alerts-up", want: true},
		{name: "trailing star", pattern: "/alerts/*", value: "/alerts/x", want: true},
		{name: "prefix literal", pattern: "orderCr*", value: "orderCreated", want: true},
		{name: "bare star", pattern: "*", value: "anything", want: true},
		{name: "exact", pattern: "orderCreated", value: "orderCreated", want: true},
		{name: "miss", pattern: "orderOther", value: "orderCreated", want: false},
		{name: "multi segment order", pattern: "*a*b", value: "xa-b", want: true},
		{name: "out of order segments", pattern: "*b*a", value: "a-b", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, globMatch(tc.pattern, tc.value))
		})
	}
}
