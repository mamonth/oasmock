package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
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

// wsConnection is a single registered WebSocket consumer connection.
type wsConnection struct {
	id      string
	channel string
	writer  *wsWriter
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
func (r *connectionRegistry) register(channel string, writer *wsWriter) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.autoID++
	id := "conn-" + strconv.Itoa(r.autoID)
	conn := &wsConnection{id: id, channel: channel, writer: writer}
	r.byID[id] = conn
	if r.byChan[channel] == nil {
		r.byChan[channel] = make(map[string]*wsConnection)
	}
	r.byChan[channel][id] = conn
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

// wsProtocolAdapter serves AsyncAPI ws channels as raw WebSockets (RS.ASP.2,
// RS.ASP.6-7, RS.ASP.9). When the document declares root x-signalr the session
// is handed to the SignalR overlay instead (design D7).
type wsProtocolAdapter struct {
	registry *connectionRegistry
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
		id := a.registry.register(channel, wr)
		defer a.registry.unregister(id)

		slog.Debug("WebSocket consumer connected", "connectionId", id, "channel", channel)

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
			out, herr := handler.HandleMessage(r.Context(), InboundMessage{
				Payload:      payload,
				ConnectionID: id,
				PathParams:   addressParams(r),
			})
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
	}
}
