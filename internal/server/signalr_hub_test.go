package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/asyncapi"
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

	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
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

	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
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

	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	token, connID := hub.conns.issueToken()
	require.NotEmpty(t, token)
	require.NotEmpty(t, connID)

	gotConnID, ok := hub.conns.consumeToken(token)
	assert.True(t, ok, "issued token must correlate with its connection id")
	assert.Equal(t, connID, gotConnID)
	// The token is consumed on correlation, so it cannot be reused.
	_, ok = hub.conns.consumeToken(token)
	assert.False(t, ok, "consumed token must not correlate again")
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

	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	token, connID := hub.conns.freshToken()
	require.NotEmpty(t, token)
	require.NotEmpty(t, connID)
	// A fresh token is not correlated until an upgrade presents it.
	_, ok := hub.conns.consumeToken(token)
	assert.False(t, ok, "fresh token is not yet correlated")
}

/*
Scenario: Negotiate advertises only the Text transfer format
Given a negotiate request
When negotiateSignalR is called
Then WebSockets is offered with Text transfer format only, matching the
handshake that rejects binary frames

Related spec scenarios: RS.SHR.8
*/
func TestNegotiateSignalR_TextOnlyTransferFormat(t *testing.T) {
	t.Parallel()

	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hub/negotiate", nil)

	hub.negotiate(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"transferFormats":["Text"]`)
	assert.NotContains(t, body, `"Binary"`)
}

/*
Scenario: Handshake error frames are JSON-escaped
Given a handshake error message containing a quote and backslash
When writeHandshakeError is used
Then the produced frame is valid JSON with the message escaped

Related spec scenarios: RS.SHR.15
*/
func TestSignalRHub_HandshakeErrorIsJSONEscaped(t *testing.T) {
	t.Parallel()

	hub := newSignalRHubAtPath(&Server{}, "/hub", "", nil)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		hub.writeHandshakeError(&signalRConnection{id: "x", writer: newWSWriter(conn)}, `bad "protocol" \ here`)
	}))
	defer ts.Close() //nolint:errcheck

	wsURL := "ws" + ts.URL[len("http"):] + "/hub"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	_, frame, err := conn.ReadMessage()
	require.NoError(t, err)
	// The frame is `{...}\x1e`; validate the JSON message before the separator.
	chunks := splitSignalRFrames(frame)
	require.Len(t, chunks, 1, "handshake error should be a single framed message")
	require.True(t, json.Valid(chunks[0]), "handshake error JSON must be valid")
	frameStr := string(chunks[0])
	assert.Contains(t, frameStr, `bad \"protocol\" \\ here`)
	assert.NotContains(t, frameStr, `bad "protocol"`, "quote must be escaped, not spliced raw")
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

	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
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

	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hub/negotiate?transport=webSockets", nil)

	hub.negotiate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"WebSockets"`)
}

/*
Scenario: Candidates deduplicates per-connection for multi-stream hubs
Given a hub connection holding two open streams on the same channel
When hubManager.Candidates is called for that channel
Then exactly one candidate (the single connection) is returned carrying both
streams, so the per-connection partition cannot emit one write per stream and
duplicate delivery quadratically

Related spec scenarios: RS.SHR.22
*/
func TestHubManager_CandidatesDeduplicatesStreams(t *testing.T) {
	t.Parallel()

	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)
	hub.channels["priceFeed"] = &asyncapi.Channel{ID: "priceFeed", Address: "/price"}
	sc := &signalRConnection{
		id:      "signalr-1",
		writer:  newWSWriter(nil),
		streams: make(map[string]*signalRStream),
	}
	sc.streams["inv-1"] = &signalRStream{invocationID: "inv-1", channelID: "priceFeed", connID: "signalr-1"}
	sc.streams["inv-2"] = &signalRStream{invocationID: "inv-2", channelID: "priceFeed", connID: "signalr-1"}
	hub.conns.register(sc)

	mgr := &hubManager{hubs: []*signalRHub{hub}}
	candidates := mgr.Candidates("/price")

	require.Len(t, candidates, 1, "one connection with two streams must yield one candidate")
	assert.Equal(t, "signalr-1", candidates[0].ConnectionID)
	assert.Len(t, candidates[0].Streams, 2, "the single candidate carries both open streams")
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

	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	hub := newSignalRHubAtPath(srv, "/hub", "", nil)

	sc := &signalRConnection{
		id:      "signalr-1",
		writer:  newWSWriter(nil),
		streams: make(map[string]*signalRStream),
	}
	sc.streams["inv-1"] = &signalRStream{invocationID: "inv-1", channelID: "priceFeed", connID: "signalr-1"}
	hub.conns.register(sc)

	streams := hub.conns.openStreamsForChannel("priceFeed")
	require.Len(t, streams, 1)
	assert.Equal(t, "signalr-1", streams[0]["connectionId"])
	assert.Equal(t, "inv-1", streams[0]["invocationId"])
	assert.Equal(t, "priceFeed", streams[0]["streamId"])

	assert.Empty(t, hub.conns.openStreamsForChannel("otherChannel"))
}
