package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Deprecated push alias answers 404 after teardown
Given a server with the canonical async management routes
When a management push is sent to the removed /_mock/ws/push path
Then the server responds with HTTP 404 Not Found

Related spec scenarios: RS.AMG.1, RS.AMG.6
*/
func TestLegacyRouteTeardown_PushAlias404(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","payload":{"msg":"alias"}}`
	resp, err := http.Post(ts.URL+"/_mock/ws/push", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

/*
Scenario: Deprecated consumers alias answers 404 after teardown
Given a server with the canonical async management routes
When consumers are listed via the removed /_mock/ws/consumers path
Then the server responds with HTTP 404 Not Found

Related spec scenarios: RS.AMG.8
*/
func TestLegacyRouteTeardown_ConsumersAlias404(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	resp, err := http.Get(ts.URL + "/_mock/ws/consumers?channel=/alerts")
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

/*
Scenario: Deprecated disconnect alias answers 404 after teardown
Given a server with the canonical async management routes
When a consumer is disconnected via the removed /_mock/ws/disconnect path
Then the server responds with HTTP 404 Not Found

Related spec scenarios: RS.AMG.14
*/
func TestLegacyRouteTeardown_DisconnectAlias404(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"connectionId":"nope","reason":"alias"}`
	resp, err := http.Post(ts.URL+"/_mock/ws/disconnect", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

/*
Scenario: Deprecated events/fire alias answers 404 after teardown
Given a server with the canonical events endpoint
When an event is fired via the removed /_mock/events/fire path
Then the server responds with HTTP 404 Not Found

Related spec scenarios: RS.MAPI.22, RS.MAPI.32
*/
func TestLegacyRouteTeardown_EventsFireAlias404(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"type":"fire","event":"levelUp","payload":{"level":"warn"}}`
	resp, err := http.Post(ts.URL+"/_mock/events/fire", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

/*
Scenario: Removed schedule push answers 404 after teardown
Given a server with the canonical examples endpoint
When a recurring push is scheduled via the removed /_mock/ws/schedule path
Then the server responds with HTTP 404 Not Found

Related spec scenarios: RS.AMG.12
*/
func TestLegacyRouteTeardown_SchedulePush404(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	body := `{"channel":"/alerts","interval":50,"payload":{"tick":true}}`
	resp, err := http.Post(ts.URL+"/_mock/ws/schedule", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

/*
Scenario: Removed schedule stop answers 404 after teardown
Given a server with the canonical examples endpoint
When a schedule is stopped via the removed /_mock/ws/schedule/{pushId} path
Then the server responds with HTTP 404 Not Found

Related spec scenarios: RS.AMG.13
*/
func TestLegacyRouteTeardown_ScheduleStop404(t *testing.T) {
	t.Parallel()

	srv := newPushServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_mock/ws/schedule/push-123", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
