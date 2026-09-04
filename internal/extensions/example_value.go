package extensions

import (
	"log/slog"

	"github.com/getkin/kin-openapi/openapi3"
)

// ExampleValue abstracts a message example (OpenAPI or AsyncAPI) so extension
// extraction and selection behave identically across both sources (design D5).
type ExampleValue interface {
	// Get retrieves a spec extension by name.
	Get(key string) (any, bool)
	// Payload returns the example's payload value.
	Payload() any
	// Headers returns the example's declared headers.
	Headers() map[string]any
}

// OpenAPIExampleValue adapts an *openapi3.Example to the ExampleValue contract.
func OpenAPIExampleValue(ex *openapi3.Example) ExampleValue {
	if ex == nil {
		return nil
	}
	return openAPIExampleValue{ex: ex}
}

type openAPIExampleValue struct {
	ex *openapi3.Example
}

func (v openAPIExampleValue) Get(key string) (any, bool) {
	if v.ex.Extensions == nil {
		return nil, false
	}
	val, ok := v.ex.Extensions[key]
	return val, ok
}

func (v openAPIExampleValue) Payload() any { return v.ex.Value }

func (v openAPIExampleValue) Headers() map[string]any {
	if v.ex.Extensions == nil {
		return nil
	}
	h, _ := v.ex.Extensions["x-mock-headers"].(map[string]any)
	return h
}

// NewExampleValue builds an ExampleValue from a payload, headers map and
// pre-captured extensions (used for AsyncAPI message examples).
func NewExampleValue(payload any, headers map[string]any, extensions map[string]any) ExampleValue {
	return mapExampleValue{
		payload: payload,
		hdrs:    headers,
		ext:     extensions,
	}
}

// mapExampleValue is a map-backed ExampleValue for AsyncAPI message examples.
type mapExampleValue struct {
	payload any
	hdrs    map[string]any
	ext     map[string]any
}

func (m mapExampleValue) Get(key string) (any, bool) {
	if m.ext == nil {
		return nil, false
	}
	v, ok := m.ext[key]
	return v, ok
}

func (m mapExampleValue) Payload() any { return m.payload }

func (m mapExampleValue) Headers() map[string]any {
	if m.hdrs != nil {
		return m.hdrs
	}
	h, _ := m.ext["x-mock-headers"].(map[string]any)
	return h
}

// ValueMatch returns the active params-match condition for an example value.
// When both x-mock-match and x-mock-params-match are present, x-mock-match
// wins and the legacy x-mock-params-match alias is deprecated; a warning is
// written to stderr mirroring ExtractParamsMatch.
func ValueMatch(ev ExampleValue) (map[string]any, bool) {
	if ev == nil {
		return nil, false
	}
	_, hasMatch := ev.Get("x-mock-match")
	_, hasParamsMatch := ev.Get("x-mock-params-match")
	var key string
	switch {
	case hasParamsMatch && hasMatch:
		slog.Warn("Example has both x-mock-match and x-mock-params-match. Ignoring deprecated x-mock-params-match; using x-mock-match.")
		key = "x-mock-match"
	case hasParamsMatch:
		key = "x-mock-params-match"
	case hasMatch:
		key = "x-mock-match"
	default:
		return nil, false
	}
	return asMap(ev, key)
}

// ValueSkip reports whether the example is marked x-mock-skip.
func ValueSkip(ev ExampleValue) bool {
	if ev == nil {
		return false
	}
	v, ok := ev.Get("x-mock-skip")
	return ok && v == true
}

// ValueOnce reports whether the example is marked x-mock-once.
func ValueOnce(ev ExampleValue) bool {
	if ev == nil {
		return false
	}
	v, ok := ev.Get("x-mock-once")
	return ok && v == true
}

// ValueSetState extracts x-mock-set-state from an example value.
func ValueSetState(ev ExampleValue) (map[string]any, bool) {
	if ev == nil {
		return nil, false
	}
	return asMap(ev, "x-mock-set-state")
}

// ValueHeaders extracts x-mock-headers from an example value.
func ValueHeaders(ev ExampleValue) (map[string]any, bool) {
	if ev == nil {
		return nil, false
	}
	h := ev.Headers()
	return h, h != nil
}

func asMap(ev ExampleValue, key string) (map[string]any, bool) {
	v, ok := ev.Get(key)
	if !ok {
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
}
