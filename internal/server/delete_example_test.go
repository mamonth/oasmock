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
Scenario: Removing a dynamic async example
Given a registered interval example and a connected consumer
When DELETE /_mock/examples/{exampleId} cancels it
Then the example is removed and no further deliveries occur

Related spec scenarios: RS.MAPI.30, RS.MAPI.25
*/
func TestDeleteExample_RemovesAsync(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	exampleID := addExample(t, ts.URL, `{"channel":"/alerts","interval":20,"response":{"code":200,"body":{"tick":true}}}`)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	waitForConnections(srv, "/alerts", 1)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, _ = conn.ReadMessage() // consume snapshot

	// First interval delivery.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"tick":true`)

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_mock/examples/"+exampleID, nil)
	require.NoError(t, err)
	delResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer delResp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, delResp.StatusCode)

	// No further deliveries after a quiet window.
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
}

/*
Scenario: Removing an unknown example returns 404
Given a DELETE for an unknown exampleId
When /_mock/examples/{exampleId} is invoked
Then the server responds with HTTP 404

Related spec scenarios: RS.MAPI.31
*/
func TestDeleteExample_Unknown404(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_mock/examples/nope", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// syncDeleteOpenAPI is a minimal OpenAPI doc with one GET path for the sync
// dynamic-example deletion test.
const syncDeleteOpenAPI = `openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths:
  /ping:
    get:
      responses:
        '200':
          description: OK
          content:
            application/json:
              examples:
                default:
                  value:
                    message: pong
`

/*
Scenario: Removing a sync dynamic example
Given a registered sync example on an OpenAPI path
When DELETE /_mock/examples/{exampleId} removes it
Then the delete succeeds and the dynamic example no longer matches the path

Related spec scenarios: RS.MAPI.30, RS.MAPI.27
*/
func TestDeleteExample_RemovesSync(t *testing.T) {
	t.Parallel()

	spec, err := openapi3.NewLoader().LoadFromData([]byte(syncDeleteOpenAPI))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindOpenAPI, Spec: spec, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	exampleID := addExample(t, ts.URL, `{"path":"/ping","method":"GET","response":{"code":200,"body":{"injected":true}}}`)

	resp, err := http.Get(ts.URL + "/ping")
	require.NoError(t, err)
	var injected map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&injected))
	resp.Body.Close() //nolint:errcheck
	assert.Equal(t, true, injected["injected"])

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_mock/examples/"+exampleID, nil)
	require.NoError(t, err)
	delResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer delResp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, delResp.StatusCode)

	resp2, err := http.Get(ts.URL + "/ping")
	require.NoError(t, err)
	defer resp2.Body.Close() //nolint:errcheck
	var after map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&after))
	assert.NotEqual(t, true, after["injected"], "the deleted dynamic example must no longer match")
}
