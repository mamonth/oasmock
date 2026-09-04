// Package asyncapi provides a protocol-neutral view of AsyncAPI 3.x documents.
//
// It isolates the backing parser (currently a vendored copy of
// github.com/benelser/go-asyncapi) behind a small document model so the rest
// of the codebase never depends on a third-party AsyncAPI type. Swapping the
// parser later only changes this package and the go.mod replace directive.
package asyncapi

// Action is the operation action type.
type Action string

const (
	// ActionSend indicates the application sends messages to the channel.
	ActionSend Action = "send"
	// ActionReceive indicates the application receives messages from the channel.
	ActionReceive Action = "receive"
)

// Supported protocols for the MVP mock server.
const (
	ProtocolHTTP = "http"
	ProtocolWS   = "ws"
)

// Document is the root of a parsed AsyncAPI document.
type Document struct {
	// Version is the AsyncAPI spec version (e.g. "3.0.0", "3.1.0").
	Version string
	// Channels indexed by channel ID, in stable order.
	Channels []*Channel
	// Operations indexed by operation ID, in stable order.
	Operations []*Operation
	// SignalR is the root-level x-signalr hub overlay, if declared.
	SignalR *SignalRConfig
}

// SignalRConfig is the root-level x-signalr hub overlay for a document.
type SignalRConfig struct {
	// Path is the hub path the SignalR hub is served at.
	Path string
	// Raw holds the full x-signalr extension for later options.
	Raw map[string]any
}

// Channel returns the channel with the given ID, or nil when absent.
func (d *Document) Channel(id string) *Channel {
	for _, ch := range d.Channels {
		if ch.ID == id {
			return ch
		}
	}
	return nil
}

// Operation returns the operation with the given ID, or nil when absent.
func (d *Document) Operation(id string) *Operation {
	for _, op := range d.Operations {
		if op.ID == id {
			return op
		}
	}
	return nil
}

// Channel is a parsed AsyncAPI channel.
type Channel struct {
	ID    string
	Title string
	// Address is the raw channel address (may contain {param} placeholders).
	Address string
	// Parameters indexed by name.
	Parameters []*Parameter
	// Messages indexed by message ID.
	Messages []*Message
	// Bindings carries the channel-level protocol bindings.
	Bindings Bindings
}

// Parameter is a channel address parameter.
type Parameter struct {
	Name string
}

// Operation is a parsed AsyncAPI operation with its channel resolved.
type Operation struct {
	ID       string
	Action   Action
	Channel  *Channel
	Messages []*Message
	Bindings Bindings
}

// Message is a parsed AsyncAPI message with its examples.
type Message struct {
	ID          string
	Name        string
	ContentType string
	// Examples in asyncapi order.
	Examples []*Example
}

// Example is a single message example including spec extensions (x-mock-*).
type Example struct {
	Name       string
	Headers    map[string]any
	Payload    any
	Extensions map[string]any
}

// Bindings carries protocol binding information used for routing.
// Protocols lists every protocol binding name declared on the object
// (http, ws, or unsupported ones such as amqp/kafka/mqtt/nats).
type Bindings struct {
	Protocols []string
	HTTP      *HTTPBinding
	WS        *WSBinding
}

// HTTPBinding is the http channel/operation binding.
type HTTPBinding struct {
	Method string
}

// WSBinding is the ws channel binding.
type WSBinding struct {
	Method string
}
