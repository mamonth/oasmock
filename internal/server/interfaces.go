package server

//go:generate mockgen -destination=interfaces_mock_test.go -package=server . RouteProvider,StateStore,HistoryStore,DataSource,RequestSourceFactory,StateSourceFactory,EnvSourceFactory,ExpressionEvaluator,ExtensionProcessor

import (
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

// RouteProvider builds route mappings from OpenAPI schemas.
type RouteProvider interface {
	// BuildRouteMappings creates route mappings from loaded schemas.
	BuildRouteMappings(schemas []SchemaInfo) ([]RouteMapping, error)
}

// RouteMapping represents a route mapping for a single OpenAPI operation.
type RouteMapping struct {
	Method     string
	Path       string // The full path pattern with prefix (e.g., "/v1/users/{id}")
	Pattern    string // The path pattern without prefix (e.g., "/users/{id}")
	Prefix     string // The prefix for this route (e.g., "/v1")
	ChiPattern string // Path converted to Chi pattern (e.g., "/v1/users/:id")
	Operation  *openapi3.Operation
	Parameters openapi3.Parameters
	Responses  *openapi3.Responses
}

// SchemaInfo holds a loaded OpenAPI spec and its path prefix.
type SchemaInfo struct {
	Spec   *openapi3.T
	Prefix string
}

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

// RequestRecord captures details of an HTTP request served by the mock.
type RequestRecord struct {
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	Method    string          `json:"method"`
	Path      string          `json:"path"`
	Query     string          `json:"query,omitempty"`
	Headers   http.Header     `json:"headers"`
	Body      []byte          `json:"body,omitempty"`
	Response  *ResponseRecord `json:"response,omitempty"`
}

// ResponseRecord captures details of the HTTP response.
type ResponseRecord struct {
	StatusCode int           `json:"statusCode"`
	Headers    http.Header   `json:"headers"`
	Body       []byte        `json:"body,omitempty"`
	Duration   time.Duration `json:"duration"`
}

// DataSource represents a source of data for runtime expressions.
type DataSource interface {
	// Get retrieves a value from the data source by path.
	// Path is a dot-separated string (e.g., "path.id", "query.page").
	// Returns the value and true if found, nil and false otherwise.
	Get(path string) (any, bool)
}

// RequestSourceFactory creates DataSource instances for HTTP requests.
type RequestSourceFactory interface {
	// NewRequestSource creates a DataSource from an HTTP request and path parameters.
	NewRequestSource(r *http.Request, pathParams map[string]string) DataSource
}

// StateSourceFactory creates DataSource instances for state.
type StateSourceFactory interface {
	// NewStateSource creates a DataSource for the given namespace.
	NewStateSource(namespace string) DataSource
}

// EnvSourceFactory creates DataSource instances for environment variables.
type EnvSourceFactory interface {
	// NewEnvSource creates a DataSource for environment variables.
	NewEnvSource() DataSource
}

// ExpressionEvaluator evaluates runtime expressions.
type ExpressionEvaluator interface {
	// AddSource adds a data source with the given name.
	AddSource(name string, source DataSource)
	// Evaluate evaluates an expression and returns the result.
	Evaluate(expr string) (any, error)
}

// ExtensionProcessor processes OpenAPI extensions.
type ExtensionProcessor interface {
	// ExtractSetState extracts x-mock-set-state extension from an example.
	ExtractSetState(example *openapi3.Example) (map[string]any, bool)
	// ExtractSkip extracts x-mock-skip extension from an example.
	ExtractSkip(example *openapi3.Example) bool
	// ExtractOnce extracts x-mock-once extension from an example.
	ExtractOnce(example *openapi3.Example) bool
	// ExtractParamsMatch extracts x-mock-params-match extension from an example.
	ExtractParamsMatch(example *openapi3.Example) (map[string]any, bool)
	// EvaluateParamsMatch evaluates a params match against an evaluator.
	EvaluateParamsMatch(params map[string]any, eval ExpressionEvaluator) (bool, error)
	// ExtractHeaders extracts x-mock-headers extension from an example.
	ExtractHeaders(example *openapi3.Example) (map[string]any, bool)
}

// Dependencies holds all dependencies for the Server.
type Dependencies struct {
	RouteProvider        RouteProvider
	StateStore           StateStore
	HistoryStore         HistoryStore
	RequestSourceFactory RequestSourceFactory
	StateSourceFactory   StateSourceFactory
	EnvSourceFactory     EnvSourceFactory
	ExpressionEvaluator  ExpressionEvaluator
	ExtensionProcessor   ExtensionProcessor
}
