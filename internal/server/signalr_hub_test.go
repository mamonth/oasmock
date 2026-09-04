package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Successful negotiation returns connection token and transports
Given a negotiate request with negotiateVersion=1
When negotiateSignalR is called
Then it returns a 200 with connectionToken, connectionId, negotiateVersion 1 and WebSockets transport

Related spec scenarios: RS.SHR.8
*/
func TestNegotiateSignalR_Success(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hub/negotiate?negotiateVersion=1", nil)

	hub.negotiate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"negotiateVersion":1`)
	assert.Contains(t, body, `"connectionToken"`)
	assert.Contains(t, body, `"connectionId"`)
	assert.Contains(t, body, `"WebSockets"`)
}

/*
Scenario: Negotiate without negotiateVersion is treated as version 0 request but answers 1
Given a negotiate request without negotiateVersion
When negotiateSignalR is called
Then the response reports negotiateVersion 1 and includes connectionToken and connectionId

Related spec scenarios: RS.SHR.9
*/
func TestNegotiateSignalR_DefaultVersion(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hub/negotiate", nil)

	hub.negotiate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"negotiateVersion":1`)
	assert.Contains(t, body, `"connectionToken"`)
	assert.Contains(t, body, `"connectionId"`)
}

/*
Scenario: Issued connection tokens correlate with upgrades
Given a negotiated token
When checkToken is called with it and with an unknown token
Then the issued token is valid and the unknown token is not

Related spec scenarios: RS.SHR.11, RS.SHR.12
*/
func TestSignalRHub_TokenCorrelation(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	token, connID := hub.issueToken()
	require.NotEmpty(t, token)
	require.NotEmpty(t, connID)

	assert.True(t, hub.checkToken(token))
	assert.False(t, hub.checkToken("unknown-token"))
	_ = fmt.Sprintf("%s-%s", token, connID)
}

/*
Scenario: Fresh token issued when upgrade has no id
Given a hub with no issued tokens
When a fresh token is requested
Then a new token and connection id are returned

Related spec scenarios: RS.SHR.13
*/
func TestSignalRHub_FreshToken(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	token, connID := hub.freshToken()
	require.NotEmpty(t, token)
	require.NotEmpty(t, connID)
	assert.False(t, hub.checkToken(token)) // fresh token not yet correlated until upgrade
}

/*
Scenario: Negotiate for an unsupported transport is rejected
Given a negotiate request requesting a non-WebSockets transport
When negotiateSignalR is called
Then it responds with HTTP 400

Related spec scenarios: RS.SHR.10
*/
func TestNegotiateSignalR_UnsupportedTransport(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hub/negotiate?transport=ServerSentEvents", nil)

	hub.negotiate(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "unsupported transport")
}

/*
Scenario: Negotiate for the WebSockets transport is accepted
Given a negotiate request explicitly requesting the WebSockets transport
When negotiateSignalR is called
Then it responds with 200 listing only WebSockets

Related spec scenarios: RS.SHR.8, RS.SHR.10
*/
func TestNegotiateSignalR_WebSocketsTransport(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hub/negotiate?transport=webSockets", nil)

	hub.negotiate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"WebSockets"`)
}

/*
Scenario: Open-stream registry tracks per-channel streams
Given a hub connection holding an open stream on a channel
When openStreamsForChannel is called
Then the stream is returned with connection id, invocation id and channel id

Related spec scenarios: RS.SHR.21
*/
func TestSignalRHub_OpenStreamsForChannel(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	sc := &signalRConnection{
		id:      "signalr-1",
		writer:  newWSWriter(nil),
		streams: make(map[string]*signalRStream),
	}
	sc.streams["inv-1"] = &signalRStream{invocationID: "inv-1", channelID: "priceFeed", connID: "signalr-1"}
	hub.mu.Lock()
	hub.conns["signalr-1"] = sc
	hub.mu.Unlock()

	streams := hub.openStreamsForChannel("priceFeed")
	require.Len(t, streams, 1)
	assert.Equal(t, "signalr-1", streams[0]["connectionId"])
	assert.Equal(t, "inv-1", streams[0]["invocationId"])
	assert.Equal(t, "priceFeed", streams[0]["streamId"])

	assert.Empty(t, hub.openStreamsForChannel("otherChannel"))
}
