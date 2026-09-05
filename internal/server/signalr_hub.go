package server

import (
	"encoding/json"
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

	// hooks is wired by the Server for built-in triggers and consumer
	// lifecycle notifications (D5).
	hooks builtInHooks

	mu     sync.Mutex
	tokens map[string]string // connection token -> connection id
	conns  map[string]*signalRConnection
	idSeq  int
}

// setHooks wires built-in trigger and lifecycle callbacks into the hub.
func (h *signalRHub) setHooks(hooks builtInHooks) {
	h.hooks = hooks
}

// signalRConnection is a single SignalR client connection.
type signalRConnection struct {
	id      string
	token   string
	conn    *websocket.Conn
	writer  *wsWriter
	streams map[string]*signalRStream // invocationId -> open stream
	server  *signalRHub
	// query/headers capture the upgrade-time metadata for {$connection.*}
	// evaluation (RS.EXT.27).
	query   map[string][]string
	headers map[string][]string
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
			// Only the Text transfer format is offered: the handshake rejects
			// binary frames (RS.SHR.8, RS.SHR.15), so advertising Binary would
			// promise a capability the server then disconnects.
			{Transport: "WebSockets", TransferFormats: []string{"Text"}},
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
		query:   r.URL.Query(),
		headers: lowerHeaderKeys(r.Header),
	}
	h.mu.Lock()
	h.conns[connID] = sc
	h.mu.Unlock()

	channel := hubDefaultChannel(h)
	info := ConsumerInfo{
		ConnectionID: connID,
		Channel:      channel,
		Query:        sc.query,
		Headers:      sc.headers,
	}
	if h.hooks.OnConnect != nil {
		h.hooks.OnConnect(channel, connID, info)
	}
	if h.hooks.Connect != nil {
		h.hooks.Connect(channel, connID, info)
	}

	defer func() {
		h.mu.Lock()
		// The connection's open streams are discarded with the connection
		// object: removing connID makes them undiscoverable (openStreamsForChannel
		// iterates h.conns), so no separate stream cleanup is needed.
		delete(h.conns, connID)
		h.mu.Unlock()
		wr.close()
		if h.hooks.OnDisconnect != nil {
			h.hooks.OnDisconnect(channel, connID)
		}
	}()

	h.runConnection(sc)
}

// buildSignalRHubs constructs a SignalR hub for each AsyncAPI document
// declaring root x-signalr (design D7).
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
