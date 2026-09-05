package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: One-shot push on the canonical async path is unchanged
Given a connected ws consumer and a management push targeting the canonical path
When the push is invoked (immediate, delayed, targeted, broadcast)
Then the consumer receives the message exactly as before the rename

Related spec scenarios: RS.AMG.1, RS.AMG.5, RS.AMG.6
*/
func TestPushCanonicalPath_Unchanged(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _ = conn.ReadMessage() // snapshot

	body := `{"channel":"/alerts","payload":{"seq":1}}`
	resp, err := http.Post(ts.URL+"/_mock/async/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"seq":1`)
}

/*
Scenario: Delayed push on the canonical path is unchanged
Given a push with a positive delay to the canonical path
When the response returns and the consumer reads
Then the message arrives after the delay

Related spec scenarios: RS.AMG.6
*/
func TestPushCanonicalPath_Delayed(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _ = conn.ReadMessage() // snapshot

	body := `{"channel":"/alerts","payload":{"delayed":true},"delay":10}`
	resp, err := http.Post(ts.URL+"/_mock/async/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"delayed":true`)
}

/*
Scenario: Targeted push on the canonical path is unchanged
Given two consumers and a push carrying one connectionId
When the push targets the canonical path
Then only the targeted consumer receives the message

Related spec scenarios: RS.AMG.5
*/
func TestPushCanonicalPath_Targeted(t *testing.T) {
	t.Parallel()

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"

	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn1.Close() //nolint:errcheck
	_ = conn1.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _ = conn1.ReadMessage() // snapshot

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn2.Close() //nolint:errcheck
	_ = conn2.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _ = conn2.ReadMessage() // snapshot

	body := `{"channel":"/alerts","connectionId":"conn-1","payload":{"targeted":true}}`
	post, err := http.Post(ts.URL+"/_mock/async/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer post.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, post.StatusCode)

	_ = conn1.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg1, err := conn1.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg1), `"targeted":true`)

	_ = conn2.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err2 := conn2.ReadMessage()
	require.Error(t, err2)
}

/*
Scenario: Broadcast push on the canonical path reaches both consumers
Given two consumers on the same channel
When a broadcast push targets the canonical path
Then both consumers receive the message

Related spec scenarios: RS.AMG.1
*/
func TestPushCanonicalPath_Broadcast(t *testing.T) {
	t.Parallel()

	srv := newAsyncMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"

	conns := make([]*websocket.Conn, 0, 2)
	for i := 0; i < 2; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close() //nolint:errcheck
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, _, _ = conn.ReadMessage() // snapshot
		conns = append(conns, conn)
	}

	body := `{"channel":"/alerts","payload":{"broadcast":true}}`
	post, err := http.Post(ts.URL+"/_mock/async/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer post.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, post.StatusCode)

	for i, conn := range conns {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, msg, rerr := conn.ReadMessage()
		require.NoError(t, rerr, "conn %d", i)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(msg, &payload))
		assert.True(t, payload["broadcast"].(bool))
	}
}
