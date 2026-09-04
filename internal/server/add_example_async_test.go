package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDynamicServer(t *testing.T) *Server {
	t.Helper()
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: parsePushDoc(t), Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)
	return srv
}

/*
Scenario: Adding a dynamic example for an AsyncAPI channel
Given an AsyncAPI ws channel and a POST to /_mock/examples with a channel identifier
When a matching message arrives
Then the dynamic example is selected by the shared pipeline

Related spec scenarios: RS.MAPI.19, RS.MAPI.20
*/
func TestAddExample_AsyncChannel(t *testing.T) {
	t.Parallel()

	srv := newDynamicServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	addBody := `{
		"protocol": "ws",
		"channel": "/alerts",
		"response": {"code": 200, "body": {"level": "dynamic", "msg": "injected"}}
	}`
	resp, err := http.Post(ts.URL+"/_mock/examples", "application/json", strings.NewReader(addBody))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"injected"`)
}

/*
Scenario: Adding a dynamic example for an unmatched AsyncAPI route is rejected
Given a POST to /_mock/examples with an unknown channel
When it does not match any loaded channel
Then the server responds with HTTP 400

Related spec scenarios: RS.MAPI.21
*/
func TestAddExample_UnmatchedAsyncRoute(t *testing.T) {
	t.Parallel()

	srv := newDynamicServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	addBody := `{
		"protocol": "ws",
		"channel": "/missing",
		"response": {"code": 200, "body": {"a": 1}}
	}`
	resp, err := http.Post(ts.URL+"/_mock/examples", "application/json", strings.NewReader(addBody))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
