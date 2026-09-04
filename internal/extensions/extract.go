package extensions

import (
	"log/slog"

	"github.com/getkin/kin-openapi/openapi3"
)

// extractExtension extracts an extension value by key and attempts to convert it to type T.
// Returns zero value and false if the extension is not present or type conversion fails.
func extractExtension[T any](ex *openapi3.Example, key string) (T, bool) {
	var zero T
	if ex == nil || ex.Extensions == nil {
		return zero, false
	}
	raw, ok := ex.Extensions[key]
	if !ok {
		return zero, false
	}
	val, ok := raw.(T)
	if !ok {
		return zero, false
	}
	return val, true
}

// ExtractParamsMatch extracts the x-mock-params-match extension from an example.
// If both x-mock-match and x-mock-params-match are present, uses x-mock-match
// and writes a warning to stderr (as per spec).
func ExtractParamsMatch(ex *openapi3.Example) (ParamsMatch, bool) {
	if ex == nil || ex.Extensions == nil {
		return nil, false
	}

	_, hasParamsMatch := ex.Extensions["x-mock-params-match"]
	_, hasMatch := ex.Extensions["x-mock-match"]

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

	m, ok := extractExtension[map[string]any](ex, key)
	return ParamsMatch(m), ok
}

// ExtractSkip extracts x-mock-skip extension.
func ExtractSkip(ex *openapi3.Example) bool {
	skip, _ := extractExtension[bool](ex, "x-mock-skip")
	return skip
}

// ExtractOnce extracts x-mock-once extension.
func ExtractOnce(ex *openapi3.Example) bool {
	once, _ := extractExtension[bool](ex, "x-mock-once")
	return once
}

// ExtractSetState extracts x-mock-set-state extension.
func ExtractSetState(ex *openapi3.Example) (map[string]any, bool) {
	return extractExtension[map[string]any](ex, "x-mock-set-state")
}

// ExtractHeaders extracts x-mock-headers extension.
func ExtractHeaders(ex *openapi3.Example) (map[string]any, bool) {
	return extractExtension[map[string]any](ex, "x-mock-headers")
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
		if delay, ok := asDelay(m["delay"]); ok {
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

// asDelay converts a JSON number to an int millisecond delay.
func asDelay(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}
