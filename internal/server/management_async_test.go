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
Scenario: Decoding an async message request accepts an array payload
Given a request body whose payload is a JSON array
When decodeAsyncMessage is called
Then it decodes successfully without rejecting the array

Related spec scenarios: RS.AMG.31
*/
func TestDecodeAsyncMessage_ArrayPayload(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/_mock/async/messages",
		strings.NewReader(`{"channel":"/alerts","payload":[{"orderId":"grid-1"}]}`))
	decoded, err := decodeAsyncMessage(req)
	require.NoError(t, err)
	assert.Equal(t, "/alerts", decoded.Channel)
	raw, err := json.Marshal(decoded.Payload)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"orderId":"grid-1"}]`, string(raw))
}

/*
Scenario: Array push payloads are not runtime-evaluated
Given an array payload whose elements carry expression-like strings
When evaluatePushPayload is called
Then the array is passed through verbatim (expressions address object fields)

Related spec scenarios: RS.AMG.31, RS.SHR.24
*/
func TestEvaluatePushPayload_ArrayVerbatim(t *testing.T) {
	// Not parallel: t.Setenv mutates the process environment, which is unsafe
	// to share across parallel tests.
	t.Setenv("OASMOCK_TEST_VAL", "resolved")
	srv := newAsyncMgmtServer(t)

	got, err := srv.evaluatePushPayload([]any{map[string]any{"msg": "{$env.OASMOCK_TEST_VAL}"}}, "/alerts")
	require.NoError(t, err)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"msg":"{$env.OASMOCK_TEST_VAL}"}]`, string(raw))
}

/*
Scenario: Scalar push payloads are not runtime-evaluated
Given a scalar payload that is a single expression
When evaluatePushPayload is called
Then the scalar is passed through verbatim

Related spec scenarios: RS.AMG.31, RS.SHR.24
*/
func TestEvaluatePushPayload_ScalarVerbatim(t *testing.T) {
	// Not parallel: t.Setenv mutates the process environment, which is unsafe
	// to share across parallel tests.
	t.Setenv("OASMOCK_TEST_VAL", "resolved")
	srv := newAsyncMgmtServer(t)

	got, err := srv.evaluatePushPayload("{$env.OASMOCK_TEST_VAL}", "/alerts")
	require.NoError(t, err)
	assert.Equal(t, "{$env.OASMOCK_TEST_VAL}", got)
}

/*
Scenario: Object push payloads are runtime-evaluated
Given an object payload whose fields carry runtime expressions
When evaluatePushPayload is called
Then the object fields are evaluated against the environment

Related spec scenarios: RS.AMG.10
*/
func TestEvaluatePushPayload_ObjectEvaluated(t *testing.T) {
	// Not parallel: t.Setenv mutates the process environment, which is unsafe
	// to share across parallel tests.
	t.Setenv("OASMOCK_TEST_VAL", "resolved")
	srv := newAsyncMgmtServer(t)

	got, err := srv.evaluatePushPayload(map[string]any{"msg": "{$env.OASMOCK_TEST_VAL}"}, "/alerts")
	require.NoError(t, err)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{"msg":"resolved"}`, string(raw))
}

/*
Scenario: Pushing a message to channel consumers immediately
Given a management push request without a delay and a connected consumer
When the push endpoint is invoked
Then the consumer receives the message

Related spec scenarios: RS.AMG.1, RS.AMG.2, RS.AMG.6
*/
func TestPushEndpoint_Immediate(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	// Consume the receive-operation snapshot emitted on connect.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)

	body := `{"channel":"/alerts","payload":{"msg":"hello"}}`
	resp, err := http.Post(ts.URL+"/_mock/async/messages", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.JSONEq(t, `{"msg":"hello"}`, string(msg))
}

/*
Scenario: Pushing with a negative delay is rejected
Given a management push request with a negative delay
When the push endpoint is invoked
Then the server responds with HTTP 400

Related spec scenarios: RS.AMG.3
*/
func TestPushEndpoint_NegativeDelay(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	body := `{"channel":"/alerts","payload":{},"delay":-10}`
	resp, err := http.Post(ts.URL+"/_mock/async/messages", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

/*
Scenario: Pushing to a channel with no consumers is accepted
Given a management push request to a valid channel with no consumers
When the push endpoint is invoked
Then the server accepts the request without error

Related spec scenarios: RS.AMG.4
*/
func TestPushEndpoint_NoConsumers(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	body := `{"channel":"/alerts","payload":{"msg":"none"}}`
	resp, err := http.Post(ts.URL+"/_mock/async/messages", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

/*
Scenario: Unknown consumer reference returns 404
Given a management push request targeting an unknown connection id
When the push endpoint is invoked
Then the server responds with HTTP 404

Related spec scenarios: RS.AMG.7
*/
func TestPushEndpoint_UnknownConsumer(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	body := `{"channel":"/alerts","payload":{},"connectionId":"missing"}`
	resp, err := http.Post(ts.URL+"/_mock/async/messages", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

/*
Scenario: Listing connected consumers per channel
Given a connected ws consumer
When the consumers endpoint is queried
Then the consumer list includes the connection id

Related spec scenarios: RS.AMG.8, RS.AMG.9
*/
func TestConsumersEndpoint(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	resp, err := http.Get(ts.URL + "/_mock/async/consumers?channel=/alerts")
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	items, ok := payload["consumers"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, items)
}
