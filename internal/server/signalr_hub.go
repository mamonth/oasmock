package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
)

// signalRNegotiate is the negotiate endpoint response (RS.SHR.8-9).
type signalRNegotiate struct {
	ConnectionToken     string                      `json:"connectionToken"`
	ConnectionID        string                      `json:"connectionId"`
	NegotiateVersion    int                         `json:"negotiateVersion"`
	AvailableTransports []signalRAvailableTransport `json:"availableTransports"`
}

type signalRAvailableTransport struct {
	Transport       string   `json:"transport"`
	TransferFormats []string `json:"transferFormats"`
}

// signalRHub is the SignalR overlay serving a single AsyncAPI document
// declared with root-level x-signalr (design D7). It depends only on the
// MessageRenderer surface, never on the whole Server.
type signalRHub struct {
	renderer MessageRenderer
	path     string // hub path, e.g. "/hub"
	doc      *asyncapi.Document
	prefix   string
	channels map[string]*asyncapi.Channel
	ops      map[string]*asyncapi.Operation

	mu     sync.Mutex
	tokens map[string]string // connection token -> connection id
	conns  map[string]*signalRConnection
	idSeq  int
}

// signalRConnection is a single SignalR client connection.
type signalRConnection struct {
	id      string
	token   string
	conn    *websocket.Conn
	writer  *wsWriter
	streams map[string]*signalRStream // invocationId -> open stream
	server  *signalRHub
}

// signalRStream is an open client-initiated stream over a channel.
type signalRStream struct {
	invocationID string
	channelID    string
	connID       string
}

// newSignalRHub creates a hub for a document with root x-signalr.
func newSignalRHub(renderer MessageRenderer, doc *asyncapi.Document, prefix string) *signalRHub {
	hub := &signalRHub{
		renderer: renderer,
		doc:      doc,
		prefix:   prefix,
		channels: make(map[string]*asyncapi.Channel),
		ops:      make(map[string]*asyncapi.Operation),
		tokens:   make(map[string]string),
		conns:    make(map[string]*signalRConnection),
	}
	if doc != nil {
		hub.path = signalRPath(doc)
		for _, ch := range doc.Channels {
			hub.channels[ch.ID] = ch
		}
		for _, op := range doc.Operations {
			hub.ops[op.ID] = op
		}
	}
	return hub
}

// newSignalRHubAtPath creates a hub at an explicit path (used by tests and
// when the caller controls the hub path directly).
func newSignalRHubAtPath(s *Server, path, prefix string, doc *asyncapi.Document) *signalRHub {
	hub := newSignalRHub(s.engine, doc, prefix)
	hub.path = normalizeHubPath(path)
	return hub
}

// signalRPath extracts the hub path from the root x-signalr extension.
func signalRPath(doc *asyncapi.Document) string {
	if doc == nil || doc.SignalR == nil {
		return ""
	}
	if p, ok := doc.SignalR.Raw["path"].(string); ok && p != "" {
		return normalizeHubPath(p)
	}
	return ""
}

// normalizeHubPath ensures the hub path starts with "/" and has no trailing "/".
func normalizeHubPath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(path, "/")
}

// prefixPath applies the schema prefix to a hub-relative path.
func (h *signalRHub) prefixPath(rel string) string {
	prefix := strings.TrimSuffix(h.prefix, "/")
	if prefix == "" {
		return rel
	}
	return prefix + rel
}

// negotiatePath is the full negotiate endpoint URL.
func (h *signalRHub) negotiatePath() string {
	return h.prefixPath(h.path + "/negotiate")
}

// upgradePath is the full WebSocket upgrade URL.
func (h *signalRHub) upgradePath() string {
	return h.prefixPath(h.path)
}

// negotiate handles POST {hubPath}/negotiate (RS.SHR.8-10).
func (h *signalRHub) negotiate(w http.ResponseWriter, r *http.Request) {
	// Reject negotiate requests for a transport this server cannot serve
	// (RS.SHR.10): only WebSockets is offered, anything else is HTTP 400.
	if transport := r.URL.Query().Get("transport"); transport != "" && !isSignalRWebSockets(transport) {
		writeJSONError(w, http.StatusBadRequest, "unsupported transport "+transport)
		return
	}
	token, connID := h.issueToken()
	resp := signalRNegotiate{
		ConnectionToken:  token,
		ConnectionID:     connID,
		NegotiateVersion: 1,
		AvailableTransports: []signalRAvailableTransport{
			{Transport: "WebSockets", TransferFormats: []string{"Text", "Binary"}},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// isSignalRWebSockets reports whether the transport name is the WebSockets
// transport (case-insensitive), which is the only one this server offers.
func isSignalRWebSockets(transport string) bool {
	return strings.EqualFold(transport, "webSockets") || strings.EqualFold(transport, "websockets")
}

// issueToken creates and records a connection token.
func (h *signalRHub) issueToken() (token, connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.idSeq++
	connID = "signalr-" + strconv.Itoa(h.idSeq)
	token = connID + "-t"
	h.tokens[token] = connID
	return token, connID
}

// checkToken validates a connection token.
func (h *signalRHub) checkToken(token string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.tokens[token]
	return ok
}

// consumeToken validates and consumes a token, binding the connection.
func (h *signalRHub) consumeToken(token string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	connID, ok := h.tokens[token]
	if ok {
		delete(h.tokens, token)
	}
	return connID, ok
}

// freshToken returns a token for a connection id (no pre-correlation).
func (h *signalRHub) freshToken() (token, connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.idSeq++
	connID = "signalr-fresh-" + strconv.Itoa(h.idSeq)
	return connID + "-t", connID
}

// serveUpgrade handles a WebSocket upgrade to the hub path (RS.SHR.11-13).
func (h *signalRHub) serveUpgrade(w http.ResponseWriter, r *http.Request) {
	// Only the WebSockets transport can upgrade (RS.SHR.10).
	if transport := r.URL.Query().Get("transport"); transport != "" && !isSignalRWebSockets(transport) {
		writeJSONError(w, http.StatusBadRequest, "unsupported transport "+transport)
		return
	}
	idParam := r.URL.Query().Get("id")
	connID := ""
	token := idParam
	if idParam != "" {
		var ok bool
		connID, ok = h.consumeToken(idParam)
		if !ok {
			writeJSONError(w, http.StatusNotFound, "unknown connection token")
			return
		}
	} else {
		// Fresh internally generated connection id (RS.SHR.13).
		_, connID = h.freshToken()
		token = connID + "-t"
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	wr := newWSWriter(conn)

	sc := &signalRConnection{
		id:      connID,
		token:   token,
		conn:    conn,
		writer:  wr,
		streams: make(map[string]*signalRStream),
		server:  h,
	}
	h.mu.Lock()
	h.conns[connID] = sc
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.conns, connID)
		h.mu.Unlock()
		h.removeConnectionStreams(connID)
		wr.close()
	}()

	h.runConnection(sc)
}

// removeConnectionStreams drops all open streams for a connection.
func (h *signalRHub) removeConnectionStreams(connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sc, ok := h.conns[connID]; ok {
		for _, st := range sc.streams {
			h.unregisterStream(st)
		}
	}
}

// runConnection drives the SignalR message loop for one connection.
func (h *signalRHub) runConnection(sc *signalRConnection) {
	handshaken := false
	for {
		messageType, payload, err := sc.conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType == websocket.PingMessage {
			sc.writer.writeMessage(websocket.PongMessage, payload)
			continue
		}
		// Handshake: the first frame must be the protocol handshake (RS.SHR.14-15).
		if !handshaken {
			if messageType != websocket.TextMessage {
				h.writeHandshakeError(sc, "binary frames are not supported")
				return
			}
			proto, version, hserr := parseSignalRHandshake(payload)
			if hserr != nil {
				h.writeHandshakeError(sc, hserr.Error())
				return
			}
			_, _ = proto, version
			sc.writer.write([]byte("{}" + string(recordSeparator)))
			handshaken = true
			continue
		}
		if messageType != websocket.TextMessage {
			continue
		}
		for _, chunk := range splitSignalRFrames(payload) {
			env, err := parseSignalREnvelope(chunk)
			if err != nil {
				continue
			}
			h.dispatch(sc, env)
		}
	}
}

// writeHandshakeError sends a handshake response then closes (RS.SHR.15).
func (h *signalRHub) writeHandshakeError(sc *signalRConnection, msg string) {
	sc.writer.write([]byte(`{"error":"` + msg + `"}` + string(recordSeparator)))
	sc.writer.close()
}

// dispatch routes a single parsed envelope.
func (h *signalRHub) dispatch(sc *signalRConnection, env signalREnvelope) {
	switch env.Type {
	case signalRTypePing:
		sc.writer.write(encodeSignalRMessage(signalREnvelope{Type: signalRTypePing}))
	case signalRTypeStreamInvocation:
		h.handleStreamInvocation(sc, env)
	case signalRTypeInvocation:
		h.handleInvocation(sc, env)
	case signalRTypeCancelInvocation:
		h.handleCancelInvocation(sc, env)
	}
}

// handleStreamInvocation answers a StreamInvocation by channel ID with the
// channel's snapshot message and holds the stream open (RS.SHR.3-5, RS.SHR.17).
func (h *signalRHub) handleStreamInvocation(sc *signalRConnection, env signalREnvelope) {
	channelID := env.Target
	ch := h.channels[channelID]
	if ch == nil {
		h.writeCompletion(sc, env.InvocationID, fmt.Sprintf("unknown channel target %q", channelID))
		return
	}

	// Snapshot stream item(s).
	count, body, err := h.renderChannel(channelID)
	if err != nil {
		h.writeCompletion(sc, env.InvocationID, fmt.Sprintf("failed to render channel: %v", err))
		return
	}
	if count == 0 {
		h.writeCompletion(sc, env.InvocationID, "channel has no message examples")
		return
	}
	sc.writer.write(encodeSignalRMessage(signalREnvelope{
		Type:         signalRTypeStreamItem,
		InvocationID: env.InvocationID,
		Item:         json.RawMessage(body),
	}))

	// Hold the stream open and register it (RS.SHR.4, RS.SHR.21).
	h.mu.Lock()
	sc.streams[env.InvocationID] = &signalRStream{
		invocationID: env.InvocationID,
		channelID:    channelID,
		connID:       sc.id,
	}
	h.mu.Unlock()
}

// handleInvocation answers a one-shot Invocation by operation ID with a
// Completion carrying the operation's message example (RS.SHR.6-7).
func (h *signalRHub) handleInvocation(sc *signalRConnection, env signalREnvelope) {
	opID := env.Target
	op := h.ops[opID]
	if op == nil {
		h.writeCompletion(sc, env.InvocationID, fmt.Sprintf("unknown operation target %q", opID))
		return
	}
	count, body, err := h.renderOperation(opID)
	if err != nil {
		h.writeCompletion(sc, env.InvocationID, fmt.Sprintf("failed to render operation: %v", err))
		return
	}
	if count == 0 {
		h.writeCompletion(sc, env.InvocationID, "operation has no message examples")
		return
	}
	sc.writer.write(encodeSignalRMessage(signalREnvelope{
		Type:         signalRTypeCompletion,
		InvocationID: env.InvocationID,
		Result:       json.RawMessage(body),
	}))
}

// handleCancelInvocation closes an open stream (RS.SHR.17).
func (h *signalRHub) handleCancelInvocation(sc *signalRConnection, env signalREnvelope) {
	h.mu.Lock()
	if st, ok := sc.streams[env.InvocationID]; ok {
		h.unregisterStream(st)
	}
	h.mu.Unlock()
	h.writeCompletion(sc, env.InvocationID, "")
}

// unregisterStream removes a stream from its connection registry.
// The caller must hold h.mu.
func (h *signalRHub) unregisterStream(st *signalRStream) {
	if sc, ok := h.conns[st.connID]; ok {
		delete(sc.streams, st.invocationID)
	}
}

// writeCompletion sends a completion envelope.
func (h *signalRHub) writeCompletion(sc *signalRConnection, invocationID, errMsg string) {
	env := signalREnvelope{Type: signalRTypeCompletion, InvocationID: invocationID}
	if errMsg != "" {
		env.Error = errMsg
	}
	sc.writer.write(encodeSignalRMessage(env))
}

// renderChannel renders the snapshot message for a channel as JSON bytes.
func (h *signalRHub) renderChannel(channelID string) (int, []byte, error) {
	ch := h.channels[channelID]
	if ch == nil {
		return 0, nil, nil
	}
	specs := loader.MessageSpecsFromAsync(ch.Messages)
	opID := "signalr:channel:" + channelID
	return h.renderer.RenderMessageSpecs(specs, h.prefix, opID, InboundMessage{})
}

// renderOperation renders the result message for a one-shot operation.
func (h *signalRHub) renderOperation(opID string) (int, []byte, error) {
	op := h.ops[opID]
	if op == nil {
		return 0, nil, nil
	}
	specs := loader.MessageSpecsFromAsync(op.Messages)
	opKey := "signalr:operation:" + opID
	return h.renderer.RenderMessageSpecs(specs, h.prefix, opKey, InboundMessage{})
}

// pushToStreams emits a templated payload into all open streams of a channel;
// when no stream is open it sends a server Invocation (RS.SHR.18-19).
func (h *signalRHub) pushToStreams(channelID string, payload []byte, target string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var matched bool
	for _, sc := range h.conns {
		for invocationID, st := range sc.streams {
			if st.channelID != channelID {
				continue
			}
			matched = true
			sc.writer.write(encodeSignalRMessage(signalREnvelope{
				Type:         signalRTypeStreamItem,
				InvocationID: invocationID,
				Item:         json.RawMessage(payload),
			}))
		}
	}
	if !matched {
		for _, sc := range h.conns {
			// Server-to-client Invocation with a server-assigned id (RS.SHR.19).
			sc.writer.write(encodeSignalRMessage(signalREnvelope{
				Type:         signalRTypeInvocation,
				InvocationID: "srv-" + target + "-" + strconv.Itoa(h.idSeq),
				Target:       target,
				Arguments:    []any{json.RawMessage(payload)},
			}))
		}
	}
}

// pushToConnection pushes a payload to one connection's open streams for the
// channel, falling back to a server Invocation on that connection when no
// stream is open (RS.AMG.5, RS.SHR.18-19).
func (h *signalRHub) pushToConnection(connectionID, channelID string, payload []byte, target string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sc, ok := h.conns[connectionID]
	if !ok {
		return
	}
	var matched bool
	for invocationID, st := range sc.streams {
		if st.channelID != channelID {
			continue
		}
		matched = true
		sc.writer.write(encodeSignalRMessage(signalREnvelope{
			Type:         signalRTypeStreamItem,
			InvocationID: invocationID,
			Item:         json.RawMessage(payload),
		}))
	}
	if !matched {
		sc.writer.write(encodeSignalRMessage(signalREnvelope{
			Type:         signalRTypeInvocation,
			InvocationID: "srv-" + target + "-" + strconv.Itoa(h.idSeq),
			Target:       target,
			Arguments:    []any{json.RawMessage(payload)},
		}))
	}
}

// buildSignalRHubs constructs a SignalR hub for each AsyncAPI document that
// declares root x-signalr (design D7).
func buildSignalRHubs(renderer MessageRenderer, schemas []SchemaInfo) []*signalRHub {
	var hubs []*signalRHub
	for _, schema := range schemas {
		if schema.Kind != loader.KindAsyncAPI || schema.Async == nil || schema.Async.SignalR == nil {
			continue
		}
		hub := newSignalRHub(renderer, schema.Async, schema.Prefix)
		if hub.path == "" {
			continue
		}
		hubs = append(hubs, hub)
	}
	return hubs
}

// registerSignalRHubs registers negotiate + upgrade endpoints for all hubs.
func (s *Server) registerSignalRHubs(r interface {
	Post(string, http.HandlerFunc)
	Get(string, http.HandlerFunc)
}) {
	for _, hub := range s.hubMgr.hubs {
		r.Post(hub.negotiatePath(), hub.negotiate)
		r.Get(hub.upgradePath(), hub.serveUpgrade)
	}
}

// openStreamsForChannel returns open-stream descriptions for a channel.
func (h *signalRHub) openStreamsForChannel(channelID string) []map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []map[string]string
	for _, sc := range h.conns {
		for invocationID, st := range sc.streams {
			if st.channelID == channelID {
				out = append(out, map[string]string{
					"connectionId": sc.id,
					"invocationId": invocationID,
					"streamId":     st.channelID,
				})
			}
		}
	}
	return out
}
