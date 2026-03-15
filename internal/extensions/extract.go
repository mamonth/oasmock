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
		slog.Warn("Example has both x-mock-match and x-mock-params-match. Using x-mock-match (deprecated).")
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
