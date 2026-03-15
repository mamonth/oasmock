package extensions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/xeipuuv/gojsonschema"
)

// ParamsMatch represents a parsed x-mock-params-match extension.
type ParamsMatch map[string]any

var (
	schemaCache sync.Map // key: schema hash -> *gojsonschema.Schema
)

// getCachedSchema returns a cached compiled schema or compiles and caches it.
func getCachedSchema(schema map[string]any) (*gojsonschema.Schema, error) {
	// Create a hash of the schema for caching
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(schemaJSON)
	key := hex.EncodeToString(hash[:])

	// Check cache
	if cached, ok := schemaCache.Load(key); ok {
		return cached.(*gojsonschema.Schema), nil
	}

	// Compile and cache
	compiled, err := gojsonschema.NewSchema(gojsonschema.NewGoLoader(schema))
	if err != nil {
		return nil, err
	}
	schemaCache.Store(key, compiled)
	return compiled, nil
}

// EvaluateParamsMatch evaluates whether the given params match the conditions.
func EvaluateParamsMatch(pm ParamsMatch, eval runtime.Evaluator) (bool, error) {
	for expr, condition := range pm {
		// Evaluate the runtime expression
		value, err := eval.Evaluate(expr)
		if err != nil {
			// If expression cannot be evaluated, treat as mismatch
			slog.Debug("EvaluateParamsMatch: expression evaluation failed", "expr", expr, "err", err)
			return false, nil
		}
		slog.Debug("EvaluateParamsMatch: expression evaluated", "expr", expr, "value", value, "condition", condition)

		// Check condition based on its type
		switch cond := condition.(type) {
		case map[string]any:
			// JSON schema condition
			matched, err := matchesJSONSchema(value, cond)
			if err != nil {
				return false, fmt.Errorf("schema validation error for %q: %w", expr, err)
			}
			if !matched {
				return false, nil
			}
		default:
			// Literal equality (JSON types)
			if !equalJSON(value, cond) {
				return false, nil
			}
		}
	}
	return true, nil
}

// equalJSON compares two JSON values for equality.
func equalJSON(a, b any) bool {
	// Handle numeric string vs number equality
	if equal, ok := numericEqual(a, b); ok {
		return equal
	}
	// Fallback to JSON equality
	aj, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bj, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aj) == string(bj)
}

// numericEqual returns true if both values are numerically equal.
// Second return indicates if comparison was numeric (i.e., at least one is a numeric string).
func numericEqual(a, b any) (bool, bool) {
	// Convert both to float64 if possible
	var fa, fb float64
	var okA, okB bool

	if sa, ok := a.(string); ok {
		if f, err := strconv.ParseFloat(sa, 64); err == nil {
			fa = f
			okA = true
		}
	} else {
		fa, okA = asFloat64(a)
	}
	if sb, ok := b.(string); ok {
		if f, err := strconv.ParseFloat(sb, 64); err == nil {
			fb = f
			okB = true
		}
	} else {
		fb, okB = asFloat64(b)
	}
	if okA && okB {
		return fa == fb, true
	}
	return false, false
}

// asFloat64 converts numeric types to float64.
func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	default:
		return 0, false
	}
}

// matchesJSONSchema validates a value against a JSON schema.
func matchesJSONSchema(value any, schema map[string]any) (bool, error) {
	compiledSchema, err := getCachedSchema(schema)
	if err != nil {
		return false, err
	}

	valueLoader := gojsonschema.NewGoLoader(value)
	result, err := compiledSchema.Validate(valueLoader)
	if err != nil {
		return false, err
	}
	return result.Valid(), nil
}
