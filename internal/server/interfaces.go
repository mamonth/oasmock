package server

//go:generate mockgen -destination=interfaces_mock_test.go -package=server . RouteProvider,StateStore,HistoryStore,RpcProtocol

import (
	"github.com/mamonth/oasmock/internal/history"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
)

// RouteProvider builds route mappings from OpenAPI schemas.
type RouteProvider interface {
	// BuildRouteMappings creates route mappings from loaded schemas.
	BuildRouteMappings(schemas []SchemaInfo) ([]RouteMapping, error)
}

// RouteMapping is a route mapping for a single OpenAPI operation or AsyncAPI
// channel/operation. It aliases loader.RouteMapping so the server never owns a
// mirror copy of the loader's routing model (single source of truth).
type RouteMapping = loader.RouteMapping

// SchemaInfo holds a loaded spec (OpenAPI or AsyncAPI) and its path prefix. It
// aliases loader.SchemaInfo; the server consumes the loader's model directly.
type SchemaInfo = loader.SchemaInfo

// StateStore manages state per namespace.
type StateStore interface {
	// Get returns the value for the given key in the namespace.
	Get(namespace, key string) (any, bool)
	// Set sets a key-value pair in the namespace.
	Set(namespace, key string, value any)
	// Increment increments a numeric value in the namespace.
	// If the key does not exist, it is initialized to delta.
	// Returns the new value.
	Increment(namespace, key string, delta float64) (float64, error)
	// Delete removes a key from the namespace.
	Delete(namespace, key string)
	// GetNamespace returns all key-value pairs in the namespace.
	GetNamespace(namespace string) map[string]any
	// GetAll returns all state across all namespaces.
	GetAll() map[string]map[string]any
}

// HistoryStore stores request history records.
type HistoryStore interface {
	// Add adds a request record to the store.
	Add(record RequestRecord)
	// GetAll returns all request records.
	GetAll() []RequestRecord
	// Count returns the number of records in the store.
	Count() int
	// Capacity returns the maximum capacity of the store.
	Capacity() int
	// Clear removes all records from the store.
	Clear()
}

// RequestRecord captures details of an HTTP request served by the mock. It
// aliases history.RequestRecord so the server and store share one record type.
type RequestRecord = history.RequestRecord

// ResponseRecord captures details of the HTTP response. It aliases
// history.ResponseRecord.
type ResponseRecord = history.ResponseRecord

// RpcProtocol parses RPC request bodies and formats error responses.
type RpcProtocol interface {
	// ParseBody parses a request body into an ordered sequence of entries. A
	// valid call yields an entry with Call set; a malformed batch element
	// yields an entry with Error set (code -32600) and does not abort the
	// other elements. It returns a fatal error only when the body itself is
	// unparseable JSON or is neither an object nor an array; the error carries
	// a JSON-RPC code (see rpcErrorCode).
	ParseBody(body []byte) ([]RpcEntry, error)
	ErrorResponse(code int, message string, id any) []byte
	ContentType() string
}

// RpcEntry is one slot of a parsed JSON-RPC body: either a valid call or a
// per-element protocol error, preserving the request order for batch responses.
type RpcEntry struct {
	Call  *RpcCall        // non-nil for a valid call
	Error *RpcParsedError // non-nil for a malformed element
}

// RpcParsedError is a per-element JSON-RPC error captured during batch
// parsing (for example a batch element missing the jsonrpc or method field).
// Per the JSON-RPC 2.0 spec, a malformed element is answered with -32600 and
// does not abort the other elements.
type RpcParsedError struct {
	Code int
	ID   any
}

// RpcCall represents a single parsed RPC call.
type RpcCall struct {
	Procedure string
	Raw       any
	ID        any
	HasID     bool
}

// Dependencies holds all dependencies for the Server. Only the stores and the
// route provider are injected; the example engine owns runtime-expression
// evaluation, data-source construction and extension processing directly.
type Dependencies struct {
	RouteProvider RouteProvider
	StateStore    StateStore
	HistoryStore  HistoryStore
}

// MessageRenderer is the message-rendering surface consumed by the SignalR
// hub, the event bus and the async protocol adapters. It narrows the
// dependencies of those subsystems to the example-selection/templating core
// instead of the whole Server. exampleEngine implements it.
type MessageRenderer interface {
	// SelectAsyncExample selects a message example using the shared x-mock-*
	// semantics.
	SelectAsyncExample(message *loader.MessageSpec, evaluator runtime.Evaluator, opID string) (*MessageExampleView, string)
	// RenderMessageSpecs renders the first selectable example across the given
	// message specs.
	RenderMessageSpecs(messages []*loader.MessageSpec, prefix, opID string, in InboundMessage) (int, []byte, error)
	// RenderAsyncPayload evaluates runtime expressions in an example payload.
	RenderAsyncPayload(example *MessageExampleView, evaluator runtime.Evaluator) ([]byte, error)
	// ApplySetState applies x-mock-set-state against a schema namespace.
	ApplySetState(stateMap map[string]any, eval runtime.Evaluator, prefix string)
	// NewStateSource builds the runtime state source for a schema namespace.
	NewStateSource(prefix string) *runtime.StateSource
	// NewEnvSource builds the runtime environment-variable source.
	NewEnvSource() *runtime.EnvSource
}

// ConsumerInfo is a delivery candidate: a raw ws consumer or a SignalR
// connection with open streams, carrying the connection context captured at
// upgrade for {$connection.*} evaluation.
type ConsumerInfo struct {
	ConnectionID string
	Channel      string
	Query        map[string][]string
	Headers      map[string][]string
	Streams      []map[string]string
}

// ConsumerBus emits rendered payloads to channel consumers (SignalR open
// streams and/or raw ws broadcast) and enumerates delivery candidates for
// per-connection recipient partitioning. hubManager implements it.
type ConsumerBus interface {
	// SignalRPush emits a payload into a SignalR hub channel's open streams or
	// as a server invocation when none are open (RS.SHR.18-19).
	SignalRPush(address string, payload []byte)
	// WSBroadcast sends a payload to every connected raw ws consumer of a
	// channel address.
	WSBroadcast(address string, payload []byte)
	// Candidates returns every consumer of a channel address (raw ws and
	// SignalR) with the connection context captured at upgrade.
	Candidates(address string) []ConsumerInfo
	// PushTo delivers a payload to one candidate consumer (its open streams,
	// falling back to a server invocation on the same connection).
	PushTo(consumer ConsumerInfo, address string, payload []byte)
}
