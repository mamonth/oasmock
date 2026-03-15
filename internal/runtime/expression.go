package runtime

//go:generate mockgen -destination=../../mock/runtime/expression_mock.go -package=mock_runtime . DataSource,Evaluator
//go:generate mockgen -destination=expression_mock_test.go -package=runtime . DataSource,Evaluator

import (
	"fmt"
	"strings"
)

// splitEscapedPath splits a path by dots, respecting escaped dots (\.).
// Returns the parts with escapes removed.
func splitEscapedPath(path string) []string {
	parts := make([]string, 0)
	var current strings.Builder
	escape := false

	for i := 0; i < len(path); i++ {
		ch := path[i]

		if escape {
			current.WriteByte(ch)
			escape = false
			continue
		}

		if ch == '\\' {
			// Check if next char is dot
			if i+1 < len(path) && path[i+1] == '.' {
				escape = true
				continue
			}
			current.WriteByte(ch)
		} else if ch == '.' {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// getNested retrieves a value from a nested map using path parts.
func getNested(obj any, parts []string) (any, bool) {
	current := obj
	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = v[part]
			if !ok {
				return nil, false
			}
		default:
			// Not a map, cannot traverse further
			return nil, false
		}
	}
	return current, true
}

// DataSource represents a source of data for runtime expressions.
type DataSource interface {
	// Get retrieves a value from the data source by path.
	// Path is a dot-separated string (e.g., "path.id", "query.page").
	// Returns the value and true if found, nil and false otherwise.
	Get(path string) (any, bool)
}

// RequestSource provides access to request data.
type RequestSource struct {
	PathParams  map[string]string
	QueryParams map[string][]string
	Headers     map[string][]string
	Body        any
	Cookies     map[string]string
}

func (r *RequestSource) Get(path string) (any, bool) {
	parts := splitEscapedPath(path)
	if len(parts) < 2 {
		return nil, false
	}

	category := parts[0]
	// Re-join remaining parts with dots for the key (if there were escaped dots)
	key := strings.Join(parts[1:], ".")

	switch category {
	case "path":
		if val, ok := r.PathParams[key]; ok {
			return val, true
		}
	case "query":
		if vals, ok := r.QueryParams[key]; ok && len(vals) > 0 {
			if len(vals) == 1 {
				return vals[0], true
			}
			return vals, true
		}
	case "header":
		if vals, ok := r.Headers[strings.ToLower(key)]; ok && len(vals) > 0 {
			if len(vals) == 1 {
				return vals[0], true
			}
			return vals, true
		}
	case "body":
		if r.Body == nil {
			return nil, false
		}
		// Use getNested for nested path traversal
		remainingParts := parts[1:]
		if len(remainingParts) == 0 {
			return nil, false
		}
		return getNested(r.Body, remainingParts)
	case "cookie":
		if val, ok := r.Cookies[key]; ok {
			return val, true
		}
	}

	return nil, false
}

// StateSource provides access to server state.
type StateSource struct {
	Data map[string]any
}

func (s *StateSource) Get(path string) (any, bool) {
	parts := splitEscapedPath(path)
	if len(parts) == 0 {
		return nil, false
	}
	// If we have a flat key (single part), look up directly
	if len(parts) == 1 {
		if val, ok := s.Data[parts[0]]; ok {
			return val, true
		}
		return nil, false
	}
	// For nested path, try to traverse
	// First part is the top-level key
	topKey := parts[0]
	val, ok := s.Data[topKey]
	if !ok {
		return nil, false
	}
	// Traverse remaining parts
	return getNested(val, parts[1:])
}

// EnvSource provides access to environment variables.
type EnvSource struct {
	Env map[string]string
}

func (e *EnvSource) Get(path string) (any, bool) {
	// Environment variables are flat
	parts := splitEscapedPath(path)
	if len(parts) != 1 {
		return nil, false
	}
	key := parts[0]
	if val, ok := e.Env[key]; ok {
		return val, true
	}
	return nil, false
}

// Evaluator evaluates runtime expressions using available data sources.
type Evaluator interface {
	AddSource(name string, source DataSource)
	Evaluate(expr string) (any, error)
}

// evaluator implements the Evaluator interface.
type evaluator struct {
	sources map[string]DataSource
}

func NewEvaluator() Evaluator {
	return &evaluator{
		sources: make(map[string]DataSource),
	}
}

// AddSource registers a data source with a name (e.g., "request", "state", "env").
func (e *evaluator) AddSource(name string, source DataSource) {
	e.sources[name] = source
}

// Evaluate evaluates a runtime expression like "{$request.path.id}".
func (e *evaluator) Evaluate(expr string) (any, error) {
	if !strings.HasPrefix(expr, "{$") || !strings.HasSuffix(expr, "}") {
		return nil, fmt.Errorf("invalid expression format: %s", expr)
	}

	// Extract inner expression: {$request.path.id} -> request.path.id
	inner := strings.TrimSuffix(strings.TrimPrefix(expr, "{$"), "}")

	// Check for modifier
	var modifier string
	if before, after, found := strings.Cut(inner, "|"); found {
		modifier = strings.TrimSpace(after)
		inner = strings.TrimSpace(before)
	}

	// Parse source and path
	parts := strings.SplitN(inner, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid expression syntax: %s", inner)
	}

	sourceName := parts[0]
	path := parts[1]

	source, ok := e.sources[sourceName]
	if !ok {
		return nil, fmt.Errorf("unknown data source: %s", sourceName)
	}

	value, found := source.Get(path)
	if !found {
		// Apply default modifier if present and value not found
		if modifier != "" && strings.HasPrefix(modifier, "default:") {
			defaultVal := strings.TrimPrefix(modifier, "default:")
			return defaultVal, nil
		}
		// Path not found, but we have a modifier (e.g., toJWT) - apply with nil value
		if modifier != "" {
			return e.applyModifier(nil, modifier)
		}
		// No modifier - return nil (not an error)
		return nil, nil
	}

	// Apply modifier if any
	if modifier != "" {
		return e.applyModifier(value, modifier)
	}

	return value, nil
}

func (e *evaluator) applyModifier(value any, modifier string) (any, error) {
	// Check for modifier with argument (e.g., "default:value", "getByPath:path")
	if name, arg, found := strings.Cut(modifier, ":"); found {

		switch name {
		case "default":
			// Should have been handled earlier when value not found
			// If we reach here, value exists, so return it
			return value, nil
		case "getByPath":
			// value should be an object (map), arg is a dot-separated path
			pathParts := splitEscapedPath(arg)
			if len(pathParts) == 0 {
				return nil, fmt.Errorf("empty path for getByPath modifier")
			}
			result, ok := getNested(value, pathParts)
			if !ok {
				return nil, fmt.Errorf("path %s not found in object", arg)
			}
			return result, nil
		default:
			return nil, fmt.Errorf("unknown modifier: %s", name)
		}
	}

	// Modifier without argument
	switch modifier {
	case "toJWT":
		// Stub implementation
		return "[JWT stub]", nil
	default:
		return nil, fmt.Errorf("unknown modifier: %s", modifier)
	}
}
