package loader

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mamonth/oasmock/internal/asyncapi"
)

// RouteMapping holds a mapping from HTTP method and path pattern to an OpenAPI
// operation, or from a channel address to an AsyncAPI channel/operation.
type RouteMapping struct {
	Method     string
	Path       string // The full path pattern with prefix (e.g., "/v1/users/{id}")
	Pattern    string // The path pattern without prefix (e.g., "/users/{id}")
	Prefix     string // The prefix for this route (e.g., "/v1")
	ChiPattern string // Path converted to Chi pattern (e.g., "/v1/users/:id")
	Operation  *openapi3.Operation
	Parameters openapi3.Parameters
	Responses  *openapi3.Responses

	// AsyncAPI-specific route data.
	Protocol string         // "http" | "ws"
	Action   string         // "send" | "receive" | "" (OpenAPI default)
	Messages []*MessageSpec // AsyncAPI-backed message specs
}

// MessageSpec is a protocol-neutral message source for AsyncAPI routes,
// carrying examples that drive the shared selection pipeline.
type MessageSpec struct {
	ID      string
	Name    string
	Headers map[string]any
	Payload any
	// Examples in AsyncAPI definition order.
	Examples []*MessageExampleSpec
}

// MessageExampleSpec is a single asyncapi message example with extensions.
type MessageExampleSpec struct {
	Name       string
	Headers    map[string]any
	Payload    any
	Extensions map[string]any
}

// buildAsyncRouteMappings converts an AsyncAPI schema's channels/operations
// into routes keyed by protocol binding. It reports startup errors for
// channels with unknown/missing binding info. When the document declares root
// x-signalr, its ws channels are served by the SignalR hub and are not mapped
// to raw ws routes (design D7).
func buildAsyncRouteMappings(info SchemaInfo) ([]RouteMapping, error) {
	if info.Async == nil {
		return nil, fmt.Errorf("schema %q has no AsyncAPI document", info.Prefix)
	}
	prefix := strings.TrimSuffix(info.Prefix, "/")

	// Group operations by the channel they reference so channel-level and
	// operation-level bindings can be combined per protocol.
	byChannel := make(map[string][]*asyncapi.Operation)
	for _, op := range info.Async.Operations {
		if op.Channel == nil {
			continue
		}
		byChannel[op.Channel.ID] = append(byChannel[op.Channel.ID], op)
	}

	var mappings []RouteMapping
	for _, ch := range info.Async.Channels {
		ops := byChannel[ch.ID]
		if len(ops) == 0 {
			return nil, fmt.Errorf("channel %q has no operations referencing it", ch.ID)
		}

		// A channel's protocol can be declared at channel level or at the
		// operation level (e.g. an http method binding on the operation).
		protocol, err := channelProtocolForOps(ch, ops)
		if err != nil {
			return nil, err
		}

		// SignalR documents serve ws channels via the hub, not as raw routes.
		if info.Async.SignalR != nil && protocol == asyncapi.ProtocolWS {
			continue
		}

		for _, op := range ops {
			rm, err := asyncRoute(ch, prefix, protocol, op, ch.Bindings)
			if err != nil {
				return nil, err
			}
			mappings = append(mappings, rm)
		}
	}
	return mappings, nil
}

// channelProtocolForOps resolves the protocol for a channel by merging its own
// channel-level bindings with the bindings of the operations that reference it.
func channelProtocolForOps(ch *asyncapi.Channel, ops []*asyncapi.Operation) (string, error) {
	prots := append([]string{}, ch.Bindings.Protocols...)
	for _, op := range ops {
		prots = append(prots, op.Bindings.Protocols...)
	}
	if len(prots) == 0 {
		return "", fmt.Errorf("channel %q has no binding information usable to determine a server protocol", ch.ID)
	}
	protocol := routeProtocol(prots)
	if protocol == "" {
		return "", fmt.Errorf("channel %q declares unsupported protocol binding(s): %s", ch.ID, strings.Join(prots, ", "))
	}
	return protocol, nil
}

// routeProtocol picks the first supported protocol from the declared set.
func routeProtocol(prots []string) string {
	for _, p := range prots {
		switch p {
		case asyncapi.ProtocolHTTP, asyncapi.ProtocolWS:
			return p
		}
	}
	return ""
}

// asyncRoute builds a single RouteMapping for an asyncapi channel+operation.
func asyncRoute(ch *asyncapi.Channel, prefix, protocol string, op *asyncapi.Operation, bindings asyncapi.Bindings) (RouteMapping, error) {
	address := ch.Address
	if address == "" {
		return RouteMapping{}, fmt.Errorf("channel %q has no address", ch.ID)
	}
	fullAddress := applyAddressPrefix(prefix, address)

	rm := RouteMapping{
		Prefix:   prefix,
		Protocol: protocol,
		Action:   string(op.Action),
		Messages: messageSpecs(op),
	}

	switch protocol {
	case asyncapi.ProtocolHTTP:
		method := "GET"
		if op.Bindings.HTTP != nil && op.Bindings.HTTP.Method != "" {
			method = op.Bindings.HTTP.Method
		}
		rm.Method = method
		rm.Path = fullAddress
		rm.Pattern = address
		rm.ChiPattern = fullAddress
	case asyncapi.ProtocolWS:
		rm.Method = http.MethodGet
		rm.Path = fullAddress
		rm.Pattern = address
		rm.ChiPattern = fullAddress
	default:
		return RouteMapping{}, fmt.Errorf("channel %q: unsupported protocol %q", ch.ID, protocol)
	}

	return rm, nil
}

// messageSpecs converts an asyncapi operation's messages into MessageSpecs.
func messageSpecs(op *asyncapi.Operation) []*MessageSpec {
	return MessageSpecsFromAsync(op.Messages)
}

// NewMessageSpec creates a MessageSpec from a single neutral AsyncAPI message,
// leaving Examples to the caller. It returns nil for a nil message.
func NewMessageSpec(m *asyncapi.Message) *MessageSpec {
	if m == nil {
		return nil
	}
	return &MessageSpec{ID: m.ID, Name: m.Name}
}

// MessageSpecsFromAsync converts neutral AsyncAPI messages into MessageSpecs
// preserving example order. Nil messages and examples are skipped. It is the
// single conversion point shared by the loader, signalr hub, event driver and
// tests.
func MessageSpecsFromAsync(messages []*asyncapi.Message) []*MessageSpec {
	var specs []*MessageSpec
	for _, m := range messages {
		spec := NewMessageSpec(m)
		if spec == nil {
			continue
		}
		for _, ex := range m.Examples {
			if ex == nil {
				continue
			}
			spec.Examples = append(spec.Examples, &MessageExampleSpec{
				Name:       ex.Name,
				Headers:    ex.Headers,
				Payload:    ex.Payload,
				Extensions: ex.Extensions,
			})
		}
		specs = append(specs, spec)
	}
	return specs
}

// applyAddressPrefix applies the schema prefix to an AsyncAPI channel address.
func applyAddressPrefix(prefix, address string) string {
	if prefix == "" {
		return normalizeAddress(address)
	}
	prefix = "/" + strings.Trim(prefix, "/")
	return prefix + normalizeAddress(address)
}

// normalizeAddress ensures a channel address is absolute (starts with "/").
func normalizeAddress(address string) string {
	addr := "/" + strings.Trim(address, "/")
	if addr == "/" {
		return addr
	}
	return addr
}

// BuildRouteMappings creates route mappings from loaded schemas.
// OpenAPI schemas map HTTP method+path pairs; AsyncAPI schemas map channels
// to routes per their protocol binding. For each schema, paths/addresses are
// combined with the schema's prefix to produce the full routing pattern.
func BuildRouteMappings(infos []SchemaInfo) ([]RouteMapping, error) {
	var mappings []RouteMapping

	for _, info := range infos {
		switch info.Kind {
		case KindAsyncAPI:
			asyncMappings, err := buildAsyncRouteMappings(info)
			if err != nil {
				return nil, err
			}
			mappings = append(mappings, asyncMappings...)
		default:
			mappings = append(mappings, buildOpenAPIRouteMappings(info)...)
		}
	}

	return mappings, nil
}

func buildOpenAPIRouteMappings(info SchemaInfo) []RouteMapping {
	var mappings []RouteMapping
	spec := info.Spec
	prefix := strings.TrimSuffix(info.Prefix, "/")

	// Collect and sort paths for deterministic iteration
	pathMap := spec.Paths.Map()
	paths := make([]string, 0, len(pathMap))
	for path := range pathMap {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		pathItem := pathMap[path]
		if pathItem == nil {
			continue
		}

		// Apply prefix to the path
		fullPath := applyPrefix(prefix, path)

		// Create mappings for each HTTP method defined in the path item
		mappings = append(mappings, createMappingsForPath(path, fullPath, prefix, pathItem)...)
	}
	return mappings
}

func applyPrefix(prefix, path string) string {
	if prefix == "" {
		return path
	}
	// Ensure prefix starts with "/" and path doesn't have double slash
	prefix = "/" + strings.Trim(prefix, "/")
	path = "/" + strings.Trim(path, "/")
	if path == "/" {
		return prefix
	}
	return prefix + path
}

func createMappingsForPath(originalPath, fullPath, prefix string, pathItem *openapi3.PathItem) []RouteMapping {
	var mappings []RouteMapping

	// Helper to add mapping if operation exists
	addMapping := func(method string, op *openapi3.Operation) {
		if op != nil {
			mappings = append(mappings, RouteMapping{
				Method:     method,
				Path:       fullPath,
				Pattern:    originalPath,
				Prefix:     prefix,
				ChiPattern: OpenAPIPatternToChi(fullPath),
				Operation:  op,
				Parameters: pathItem.Parameters,
				Responses:  op.Responses,
			})
		}
	}

	addMapping(http.MethodGet, pathItem.Get)
	addMapping(http.MethodPost, pathItem.Post)
	addMapping(http.MethodPut, pathItem.Put)
	addMapping(http.MethodPatch, pathItem.Patch)
	addMapping(http.MethodDelete, pathItem.Delete)
	addMapping(http.MethodHead, pathItem.Head)
	addMapping(http.MethodOptions, pathItem.Options)
	addMapping(http.MethodTrace, pathItem.Trace) // Note: OpenAPI 3.0 doesn't officially support TRACE

	return mappings
}

// OpenAPIPatternToChi converts an OpenAPI path pattern (with {param}) to a Chi pattern.
// Chi supports both {param} and :param syntax. We keep the OpenAPI braces.
func OpenAPIPatternToChi(pattern string) string {
	return pattern
}

// FindOperation finds the operation that matches the given method and path.
// It returns the route mapping and extracted path parameters.
func FindOperation(mappings []RouteMapping, method, path string) (*RouteMapping, map[string]string, bool) {
	for _, mapping := range mappings {
		if mapping.Method != method {
			continue
		}

		// Simple exact match for now; later we need to handle path parameters
		// For MVP, we'll do exact match on path
		if mapping.Path == path {
			return &mapping, nil, true
		}
	}
	return nil, nil, false
}
