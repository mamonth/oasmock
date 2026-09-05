package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// wsUpgrader upgrades incoming HTTP requests to WebSocket connections.
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// The mock accepts connections from any origin.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsWriter serializes writes to a WebSocket connection. gorilla/websocket
// permits a single writer goroutine; management pushes, event delivery and
// the adapter's own replies all write through this lock.
type wsWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

// newWSWriter wraps a connection so every write goes through its mutex.
func newWSWriter(conn *websocket.Conn) *wsWriter { return &wsWriter{conn: conn} }

// write sends a text frame, locking the connection's write mutex.
func (w *wsWriter) write(data []byte) {
	if w == nil || w.conn == nil || len(data) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = w.conn.WriteMessage(websocket.TextMessage, data)
}

// writeMessage sends a raw frame (used for pong replies).
func (w *wsWriter) writeMessage(messageType int, data []byte) {
	if w == nil || w.conn == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = w.conn.WriteMessage(messageType, data)
}

// writeError sends a JSON-encoded error object on the connection.
func (w *wsWriter) writeError(err error) {
	data, _ := json.Marshal(map[string]string{"error": err.Error()})
	w.write(data)
}

// writeClose sends a normal close frame with a reason.
func (w *wsWriter) writeClose(code int, reason string) {
	if w == nil || w.conn == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	msg := websocket.FormatCloseMessage(code, reason)
	_ = w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = w.conn.WriteMessage(websocket.CloseMessage, msg)
}

// abort closes the connection without a close frame (simulated abrupt drop, RS.AMG.17).
func (w *wsWriter) abort() {
	if w == nil || w.conn == nil {
		return
	}
	_ = w.conn.Close()
}

// close closes the connection.
func (w *wsWriter) close() {
	if w == nil || w.conn == nil {
		return
	}
	_ = w.conn.Close()
}

// wsConnection is a single registered WebSocket consumer connection. Metadata
// (query, headers) is captured at upgrade for {$connection.*} evaluation.
type wsConnection struct {
	id      string
	channel string
	writer  *wsWriter
	query   map[string][]string
	headers map[string][]string
}

// connectionRegistry tracks active WebSocket consumer connections per channel,
// enabling broadcast push (D9), discovery (RS.AMG.8-9) and lifecycle control
// (RS.AMG.14-17).
type connectionRegistry struct {
	mu     sync.RWMutex
	byID   map[string]*wsConnection
	byChan map[string]map[string]*wsConnection
	autoID int
}

func newConnectionRegistry() *connectionRegistry {
	return &connectionRegistry{
		byID:   make(map[string]*wsConnection),
		byChan: make(map[string]map[string]*wsConnection),
	}
}

// register adds a connection and returns a fresh connection id.
func (r *connectionRegistry) register(channel, id string, wr *wsWriter, query, headers map[string][]string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id == "" {
		r.autoID++
		id = "conn-" + strconv.Itoa(r.autoID)
	}
	c := &wsConnection{id: id, channel: channel, writer: wr, query: query, headers: headers}
	r.byID[id] = c
	if r.byChan[channel] == nil {
		r.byChan[channel] = make(map[string]*wsConnection)
	}
	r.byChan[channel][id] = c
	return id
}

// unregister removes a connection by id.
func (r *connectionRegistry) unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ws, ok := r.byID[id]
	if !ok {
		return
	}
	delete(r.byChan[ws.channel], id)
	if len(r.byChan[ws.channel]) == 0 {
		delete(r.byChan, ws.channel)
	}
	delete(r.byID, id)
}

// connections returns all connections for a channel.
func (r *connectionRegistry) connections(channel string) []*wsConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*wsConnection, 0, len(r.byChan[channel]))
	for _, ws := range r.byChan[channel] {
		out = append(out, ws)
	}
	return out
}

// allConnections returns every registered connection across all channels.
func (r *connectionRegistry) allConnections() []*wsConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*wsConnection, 0, len(r.byID))
	for _, ws := range r.byID {
		out = append(out, ws)
	}
	return out
}

// connection returns a single connection by id.
func (r *connectionRegistry) connection(id string) (*wsConnection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ws, ok := r.byID[id]
	return ws, ok
}

// lowerHeaderKeys lowercases header keys so {$connection.header.<key>} lookups
// are case-insensitive.
func lowerHeaderKeys(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		out[strings.ToLower(k)] = v
	}
	return out
}

// broadcast sends a payload to every connected consumer of a channel address.
func (a *wsProtocolAdapter) broadcast(address string, payload []byte) {
	if a == nil || a.registry == nil {
		return
	}
	for _, ws := range a.registry.connections(address) {
		ws.writer.write(payload)
	}
}

// builtInHooks carries the optional built-in trigger and lifecycle callbacks
// the Server wires in so the adapter never reaches into Server (D5, RS.EVT.24-25).
type builtInHooks struct {
	// Connect fires the connect built-in schema-local for a just-connected
	// consumer with its connection context as the recipient.
	Connect func(channel string, connID string, info ConsumerInfo)
	// Receive fires the receive built-in schema-local with the inbound message
	// exposed in the event context.
	Receive func(channel string, in InboundMessage)
	// OnConnect/OnDisconnect notify consumer lifecycle observers.
	OnConnect    func(channel string, connID string, info ConsumerInfo)
	OnDisconnect func(channel string, connID string)
}

// wsProtocolAdapter serves AsyncAPI ws channels as raw WebSockets (RS.ASP.2,
// RS.ASP.6-7, RS.ASP.9). When the document declares root x-signalr the session
// is handed to the SignalR overlay instead (design D7).
type wsProtocolAdapter struct {
	registry *connectionRegistry
	hooks    builtInHooks
}

func newWSProtocolAdapter() *wsProtocolAdapter {
	return &wsProtocolAdapter{registry: newConnectionRegistry()}
}

// Protocol implements ProtocolAdapter.
func (a *wsProtocolAdapter) Protocol() string { return asyncWSProtocol }

// Handler builds the WebSocket upgrade handler for an AsyncAPI ws channel.
func (a *wsProtocolAdapter) Handler(mapping *RouteMapping, handler MessageHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		wr := newWSWriter(conn)
		channel := mapping.Path
		id := a.registry.register(channel, "", wr, r.URL.Query(), lowerHeaderKeys(r.Header))
		defer a.registry.unregister(id)

		slog.Debug("WebSocket consumer connected", "connectionId", id, "channel", channel)

		info := ConsumerInfo{
			ConnectionID: id,
			Channel:      channel,
			Query:        r.URL.Query(),
			Headers:      lowerHeaderKeys(r.Header),
		}
		if a.hooks.OnConnect != nil {
			a.hooks.OnConnect(channel, id, info)
		}
		if a.hooks.Connect != nil {
			a.hooks.Connect(channel, id, info)
		}

		// Receive-operation emission on connect (RS.ASP.7).
		if mapping.Action == "receive" {
			out, herr := handler.HandleMessage(r.Context(), InboundMessage{ConnectionID: id, PathParams: addressParams(r)})
			if herr == nil && out != nil {
				wr.write(out)
			}
		}

		// Read loop: send-operation acceptance + reply (RS.ASP.6, RS.ASP.9).
		for {
			messageType, payload, rerr := conn.ReadMessage()
			if rerr != nil {
				break
			}
			if messageType == websocket.PingMessage {
				wr.writeMessage(websocket.PongMessage, payload)
				continue
			}
			in := InboundMessage{
				Payload:      payload,
				ConnectionID: id,
				PathParams:   addressParams(r),
			}
			if a.hooks.Receive != nil {
				a.hooks.Receive(channel, in)
			}
			out, herr := handler.HandleMessage(r.Context(), in)
			if herr != nil {
				wr.writeError(herr)
				continue
			}
			// A send with no reply messages back an ack frame (RS.ASP.9).
			if out == nil {
				out = []byte("{}")
			}
			wr.write(out)
		}

		if a.hooks.OnDisconnect != nil {
			a.hooks.OnDisconnect(channel, id)
		}
	}
}
