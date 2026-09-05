package extensions

import (
	"github.com/getkin/kin-openapi/openapi3"
)

// The Extract* functions below are the OpenAPI-typed surface used by the
// server's OpenAPI selection pipeline. They delegate to the source-agnostic
// Value* family over an OpenAPIExampleValue so both OpenAPI and AsyncAPI
// examples share one extraction implementation (design D5).

// ExtractParamsMatch extracts the x-mock-params-match extension from an example.
// If both x-mock-match and x-mock-params-match are present, uses x-mock-match
// and writes a warning to stderr (as per spec).
func ExtractParamsMatch(ex *openapi3.Example) (ParamsMatch, bool) {
	m, ok := ValueMatch(OpenAPIExampleValue(ex))
	return ParamsMatch(m), ok
}

// ExtractSkip extracts x-mock-skip extension.
func ExtractSkip(ex *openapi3.Example) bool {
	return ValueSkip(OpenAPIExampleValue(ex))
}

// ExtractOnce extracts x-mock-once extension.
func ExtractOnce(ex *openapi3.Example) bool {
	return ValueOnce(OpenAPIExampleValue(ex))
}

// ExtractSetState extracts x-mock-set-state extension.
func ExtractSetState(ex *openapi3.Example) (map[string]any, bool) {
	return ValueSetState(OpenAPIExampleValue(ex))
}

// ExtractHeaders extracts x-mock-headers extension.
func ExtractHeaders(ex *openapi3.Example) (map[string]any, bool) {
	return ValueHeaders(OpenAPIExampleValue(ex))
}

// EventTrigger is a single x-event-trigger entry (design D8).
type EventTrigger struct {
	// Name is the named event fired when the example is selected.
	Name string
	// Payload is the event payload exposed via {$event.*}.
	Payload map[string]any
	// Delay is the delivery delay in milliseconds.
	Delay int
	// Global makes the event server-wide instead of schema-local.
	Global bool
}

// ExtractEventTriggers parses the x-event-trigger list extension (RS.EVT.1-4).
// It returns false when the extension is absent or not a list.
func ExtractEventTriggers(ex *openapi3.Example) ([]EventTrigger, bool) {
	if ex == nil || ex.Extensions == nil {
		return nil, false
	}
	raw, ok := ex.Extensions["x-event-trigger"]
	if !ok {
		return nil, false
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	var out []EventTrigger
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t := EventTrigger{}
		if name, ok := m["name"].(string); ok {
			t.Name = name
		}
		if payload, ok := m["payload"].(map[string]any); ok {
			t.Payload = payload
		}
		if delay, ok := AsMilliseconds(m["delay"]); ok {
			t.Delay = delay
		}
		if global, ok := m["global"].(bool); ok {
			t.Global = global
		}
		if t.Name == "" {
			continue
		}
		out = append(out, t)
	}
	return out, len(out) > 0
}
