package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Mixing sync and async targeting is rejected
Given a POST with both path and channel
When /_mock/examples is invoked
Then the server responds with HTTP 400

Related spec scenarios: RS.MAPI.27
*/
func TestAddExampleValidation_PathAndChannel(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"path":"/users","channel":"/alerts","response":{"code":200,"body":{"a":1}}}`
	resp := postExample(t, ts.URL, body)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

/*
Scenario: match or interval on an OpenAPI target is rejected
Given a POST with path and match but no AsyncAPI target
When /_mock/examples is invoked
Then the server responds with HTTP 400

Related spec scenarios: RS.MAPI.28
*/
func TestAddExampleValidation_AsyncFieldsOnSyncedPath(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"path":"/users","match":{"{$event.name}":"x"},"response":{"code":200,"body":{"a":1}}}`
	resp := postExample(t, ts.URL, body)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

/*
Scenario: Dual triggers are rejected
Given a POST with interval alongside an event-based match
When /_mock/examples is invoked
Then the server responds with HTTP 400

Related spec scenarios: RS.MAPI.29, RS.EXT.28
*/
func TestAddExampleValidation_DualTriggers(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","interval":100,"match":{"{$event.name}":"x"},"response":{"code":200,"body":{"a":1}}}`
	resp := postExample(t, ts.URL, body)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

/*
Scenario: A non-positive interval is rejected
Given a POST with a non-positive interval on an AsyncAPI target
When /_mock/examples is invoked
Then the server responds with HTTP 400

Related spec scenarios: RS.MAPI.29
*/
func TestAddExampleValidation_NonPositiveInterval(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	for _, interval := range []string{"0", "-5"} {
		body := `{"channel":"/alerts","interval":` + interval + `,"response":{"code":200,"body":{"a":1}}}`
		resp := postExample(t, ts.URL, body)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		resp.Body.Close() //nolint:errcheck
	}
}

/*
Scenario: A non-event match on an AsyncAPI target is rejected
Given a POST with an async target and a connection-only match whose values do
not reference the event context
When /_mock/examples is invoked
Then the server responds with HTTP 400 and registers nothing

Related spec scenarios: RS.MAPI.34
*/
func TestAddExampleValidation_NonEventMatchRejected(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","match":{"{$connection.channel}":"/alerts"},"response":{"code":200,"body":{"a":1}}}`
	resp := postExample(t, ts.URL, body)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	assertNoRuntimeExampleRegistered(t, srv, "a connection-only match must not register a runtime example")
}

/*
Scenario: A connection match whose value references an event is accepted
Given a POST with an async target and '{$connection.id}': '{$event.connectionId}'
When /_mock/examples is invoked
Then the server accepts the event-driven example with HTTP 200

Related spec scenarios: RS.MAPI.33, RS.EVT.19
*/
func TestAddExampleValidation_ConnectionMatchWithEventValueAccepted(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","match":{"{$connection.id}":"{$event.connectionId}"},"response":{"code":200,"body":{"a":1}}}`
	resp := postExample(t, ts.URL, body)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

/*
Scenario: A match with no event or connection references is rejected
Given a POST with an async target and a literal-only match
When /_mock/examples is invoked
Then the server responds with HTTP 400 and registers nothing

Related spec scenarios: RS.MAPI.34
*/
func TestAddExampleValidation_LiteralOnlyMatchRejected(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","match":{"kind":"tick"},"response":{"code":200,"body":{"a":1}}}`
	resp := postExample(t, ts.URL, body)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	assertNoRuntimeExampleRegistered(t, srv, "a literal-only match must not register a runtime example")
}

// assertNoRuntimeExampleRegistered proves a rejected request left no live
// async-driven example behind (no broker subscription, no interval job),
// pinning the "registers nothing" part of the validation scenarios.
func assertNoRuntimeExampleRegistered(t *testing.T, srv *Server, msg string) {
	t.Helper()
	srv.runtimeExamples.mu.RLock()
	defer srv.runtimeExamples.mu.RUnlock()
	assert.Empty(t, srv.runtimeExamples.byID, msg)
}

/*
Scenario: An event match alongside a delay is accepted
Given a POST with an async target, an event match and a delay
When /_mock/examples is invoked
Then the server accepts it with HTTP 200

Related spec scenarios: RS.MAPI.24, RS.EXT.23
*/
func TestAddExampleValidation_EventMatchWithDelayAccepted(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","delay":10,"match":{"{$event.name}":"levelUp"},"response":{"code":200,"body":{"a":1}}}`
	resp := postExample(t, ts.URL, body)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

/*
Scenario: Existing valid request shapes still pass
Given valid sync and async add-example requests
When /_mock/examples is invoked
Then they are accepted with HTTP 200

Related spec scenarios: RS.MAPI.19, RS.MAPI.20
*/
func TestAddExampleValidation_ValidShapesPass(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	tests := []string{
		`{"channel":"/alerts","response":{"code":200,"body":{"a":1}}}`,
		`{"channel":"/alerts","match":{"{$event.name}":"levelUp"},"response":{"code":200,"body":{"a":1}}}`,
		`{"channel":"/alerts","interval":200,"response":{"code":200,"body":{"a":1}}}`,
		`{"channel":"/alerts","delay":10,"response":{"code":200,"body":{"a":1}}}`,
	}
	for _, body := range tests {
		resp := postExample(t, ts.URL, body)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", body)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
		resp.Body.Close() //nolint:errcheck
		require.NotEmpty(t, payload["id"])
	}
}

/*
Scenario: Validation failures return a valid JSON error envelope
Given a POST /_mock/examples body rejected by schema validation or handler rules
When the server responds with HTTP 400
Then the body parses as JSON with an "error" key and a JSON content type

Related spec scenarios: RS.MAPI.19, RS.MAPI.27
*/
func TestAddExampleValidation_ErrorEnvelopeIsValidJSON(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	invalidBodies := []string{
		`{"path":"/users","channel":"/alerts","response":{"code":200,"body":{"a":1}}}`,                              // oneOf violation
		`{"channel":"/alerts","interval":100,"match":{"{$event.name}":"x"},"response":{"code":200,"body":{"a":1}}}`, // dual trigger
		`{"path":"/does-not-exist","response":{"code":200}}`,                                                        // no matching route
		`not-json`, // malformed body
	}
	for _, body := range invalidBodies {
		resp := postExample(t, ts.URL, body)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body=%s", body)

		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"), "body=%s", body)
		var envelope map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope), "invalid JSON error envelope for body=%s", body)
		resp.Body.Close() //nolint:errcheck
		require.NotEmpty(t, envelope["error"])
	}
}
