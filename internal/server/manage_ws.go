package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// manageEnvelope is a single notification pushed to _mock/stream subscribers
// (RS.AMG.24-27). Type is one of event|push|consumer|schedule.
type manageEnvelope struct {
	Type string `json:"type"`
	TS   int64  `json:"ts"`
	// per-type payloads (omitempty; present only for the matching type)
	Event    *manageEventEnvelope    `json:"event,omitempty"`
	Push     *managePushEnvelope     `json:"push,omitempty"`
	Consumer *manageConsumerEnvelope `json:"consumer,omitempty"`
	Schedule *manageScheduleEnvelope `json:"schedule,omitempty"`
}

type manageEventEnvelope struct {
	Name    string         `json:"name"`
	Schema  string         `json:"schema,omitempty"`
	Global  bool           `json:"global,omitempty"`
	Payload map[string]any `json:"payload"`
}

type managePushEnvelope struct {
	Channel      string         `json:"channel"`
	ConnectionID string         `json:"connectionId,omitempty"`
	Payload      map[string]any `json:"payload"`
}

type manageConsumerEnvelope struct {
	Action       string              `json:"action"`
	ConnectionID string              `json:"connectionId"`
	Channel      string              `json:"channel"`
	Streams      []map[string]string `json:"streams,omitempty"`
}

type manageScheduleEnvelope struct {
	Action    string `json:"action"`
	ExampleID string `json:"exampleId"`
	Channel   string `json:"channel"`
	Interval  int    `json:"interval"`
}

// streamFilter holds a subscriber's connect-time filters (RS.AMG.23).
type streamFilter struct {
	events   []string
	channels []string
}

// manageStreamSub is a single connected _mock/stream subscriber.
type manageStreamSub struct {
	writer *wsWriter
	filter streamFilter
}

// manageStream is the management WebSocket stream (GET /_mock/stream). The
// registry is deliberately separate from the channel connectionRegistry so
// management sockets never appear under /_mock/async/consumers (D7).
type manageStream struct {
	mu      sync.RWMutex
	subs    map[*wsWriter]*manageStreamSub
	verbose bool
}

// newManageStream creates the management stream registry.
func newManageStream(verbose bool) *manageStream {
	return &manageStream{
		subs:    make(map[*wsWriter]*manageStreamSub),
		verbose: verbose,
	}
}

// add registers a subscriber connection with its filters.
func (ms *manageStream) add(w *wsWriter, filter streamFilter) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.subs[w] = &manageStreamSub{writer: w, filter: filter}
}

// remove unregisters a subscriber connection.
func (ms *manageStream) remove(w *wsWriter) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.subs, w)
}

// broadcast encodes and sends an envelope to matching subscribers.
func (ms *manageStream) broadcast(env manageEnvelope) {
	ms.mu.RLock()
	subs := make([]*manageStreamSub, 0, len(ms.subs))
	for _, sub := range ms.subs {
		subs = append(subs, sub)
	}
	ms.mu.RUnlock()

	env.TS = time.Now().UnixMilli()
	payload, err := json.Marshal(env)
	if err != nil {
		return
	}
	for _, sub := range subs {
		if !filterMatches(sub.filter, env) {
			continue
		}
		sub.writer.write(payload)
	}
}

// filterMatches applies a subscriber's event/channel filters to an envelope.
// Filtering is done per type; the envelope passes when it matches all active
// filters (an omitted filter matches everything, RS.AMG.23).
func filterMatches(f streamFilter, env manageEnvelope) bool {
	if len(f.events) > 0 && env.Type == "event" && env.Event != nil && !globMatchAny(f.events, env.Event.Name) {
		return false
	}
	if len(f.channels) > 0 {
		channel := ""
		switch env.Type {
		case "push":
			if env.Push != nil {
				channel = env.Push.Channel
			}
		case "consumer":
			if env.Consumer != nil {
				channel = env.Consumer.Channel
			}
		case "schedule":
			if env.Schedule != nil {
				channel = env.Schedule.Channel
			}
		case "event":
			// Event envelopes carry a schema scope, not a channel; the channels
			// filter does not apply to them.
		}
		if channel != "" && !globMatchAny(f.channels, channel) {
			return false
		}
	}
	return true
}

// globMatchAny reports whether a value matches any comma-separated glob in the
// list ("*" matches everything).
func globMatchAny(patterns []string, value string) bool {
	for _, p := range patterns {
		if globMatch(p, value) {
			return true
		}
	}
	return false
}

// globMatch implements single-* glob matching (RS.AMG.23): each literal
// segment between '*'s must appear in value in order. The first segment is
// anchored to the start when the pattern begins with a literal, and the last
// segment is anchored to the end when the pattern ends with a literal.
func globMatch(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}
	segments := splitGlob(pattern)

	pos := 0
	for i, seg := range segments {
		if i == 0 && !strings.HasPrefix(pattern, "*") {
			if !strings.HasPrefix(value, seg) {
				return false
			}
			pos = len(seg)
			continue
		}
		remaining := value[pos:]
		if i == len(segments)-1 && !strings.HasSuffix(pattern, "*") {
			return len(remaining) >= len(seg) && remaining[len(remaining)-len(seg):] == seg
		}
		idx := strings.Index(remaining, seg)
		if idx < 0 {
			return false
		}
		pos += idx + len(seg)
	}
	return true
}

// splitGlob splits a glob pattern on '*' dropping the empty segments.
func splitGlob(pattern string) []string {
	var parts []string
	for _, seg := range strings.Split(pattern, "*") {
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return parts
}

// parseStreamFilters parses the connect-time events/channels query parameters
// (comma-separated globs).
func parseStreamFilters(r *http.Request) streamFilter {
	return streamFilter{
		events:   splitComma(r.URL.Query().Get("events")),
		channels: splitComma(r.URL.Query().Get("channels")),
	}
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if part := s[start:i]; part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

// pingWriter is the subset of wsWriter the keepalive loop needs, so the loop is
// testable without a live socket.
type pingWriter interface {
	writeMessage(messageType int, data []byte)
}

// manageStreamPingInterval is the management stream keepalive cadence.
const manageStreamPingInterval = 30 * time.Second

// servePings writes WebSocket pings on a management stream connection until the
// stop signal fires. It returns when stop closes, so the goroutine never leaks
// past the connection's lifetime (the handler closes stop in its defer).
func servePings(wr pingWriter, interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		interval = manageStreamPingInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			wr.writeMessage(websocket.PingMessage, nil)
		}
	}
}

// handleManageStream upgrades GET /_mock/stream or rejects a non-upgrade
// request with 405 (RS.AMG.28). A connected subscriber's filters are parsed at
// connect time; V1 is notifications-only (pings/pongs keep the socket alive).
func (s *Server) handleManageStream(w http.ResponseWriter, r *http.Request) {
	if s.manageStream == nil {
		writeJSONError(w, http.StatusInternalServerError, "management stream not initialized")
		return
	}
	// Non-WebSocket requests must be rejected (RS.AMG.28).
	if !strings.EqualFold(r.Header.Get("Connection"), "Upgrade") ||
		!strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		writeJSONError(w, http.StatusMethodNotAllowed, "management stream requires a WebSocket upgrade")
		return
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	wr := newWSWriter(conn)
	s.manageStream.add(wr, parseStreamFilters(r))
	defer s.manageStream.remove(wr)

	// Ping/pong keepalive; V1 has no client→server commands. The pinger is
	// bound to this connection's lifetime so it cannot outlive the socket.
	stopPings := make(chan struct{})
	defer close(stopPings)
	go servePings(wr, manageStreamPingInterval, stopPings)
	for {
		messageType, payload, rerr := conn.ReadMessage()
		if rerr != nil {
			return
		}
		if messageType == websocket.PingMessage {
			wr.writeMessage(websocket.PongMessage, payload)
			continue
		}
		if messageType == websocket.PongMessage {
			continue
		}
		// Notifications-only: client frames are ignored.
	}
}

// notifyConsumer pushes a consumer lifecycle envelope (RS.AMG.26).
func (ms *manageStream) notifyConsumer(action, channel string, info ConsumerInfo) {
	if ms == nil {
		return
	}
	ms.broadcast(manageEnvelope{
		Type: "consumer",
		Consumer: &manageConsumerEnvelope{
			Action:       action,
			ConnectionID: info.ConnectionID,
			Channel:      channel,
			Streams:      info.Streams,
		},
	})
}
