package extensions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
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
		// Pre-evaluate the condition value when it is itself a runtime
		// expression AND the key references the new event/connection contexts
		// (design D6): a condition like
		// '{$connection.id}': '{$event.connectionId}' compares resolved values.
		// Reply-path conditions keep literal value semantics so sync matching
		// is unchanged.
		if str, ok := condition.(string); ok && isFullExpression(str) && referencesNewMatchContext(expr) {
			resolved, err := eval.Evaluate(str)
			if err != nil {
				slog.Debug("EvaluateParamsMatch: condition expression evaluation failed", "expr", expr, "condition", str, "err", err)
				return false, nil
			}
			condition = resolved
		}

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

// referencesNewMatchContext reports whether a condition key references the
// event or connection context, the two contexts added by this change. Only
// those conditions pre-resolve full-expression values (design D6); reply-path
// conditions keep literal value semantics.
func referencesNewMatchContext(expr string) bool {
	return eventRefPattern.MatchString(expr) || connectionRefPattern.MatchString(expr)
}

// ReferencesEvent reports whether a condition key or value references the
// event context ({$event.*}).
func ReferencesEvent(expr string, condition any) bool {
	return referencesContext(expr, condition, eventRefPattern)
}

// ReferencesNonEventContext reports whether a condition key or value references
// a reply-path context ({$request.*}, {$message.*}, {$channel.*}).
func ReferencesNonEventContext(expr string, condition any) bool {
	return referencesContext(expr, condition, nonEventContextPattern)
}

// referencesConnection reports whether a condition key or value references the
// connection context ({$connection.*}).
func referencesConnection(expr string, condition any) bool {
	return referencesContext(expr, condition, connectionRefPattern)
}

// referencesContext reports whether a condition key or its string value matches
// any of the given context-reference patterns.
func referencesContext(expr string, condition any, patterns ...*regexp.Regexp) bool {
	s, _ := condition.(string)
	for _, p := range patterns {
		if p.MatchString(expr) || p.MatchString(s) {
			return true
		}
	}
	return false
}

// isFullExpression reports whether a string is a single complete runtime
// expression (e.g. "{$event.connectionId}").
func isFullExpression(s string) bool {
	return strings.HasPrefix(s, "{$") && strings.HasSuffix(s, "}") && strings.Count(s, "{$") == 1
}

// connectionRefPattern matches a {$connection.*} reference anywhere in a
// condition key or value string.
var connectionRefPattern = regexp.MustCompile(`\{\$connection\.`)

// eventRefPattern matches a {$event.*} reference.
var eventRefPattern = regexp.MustCompile(`\{\$event\.`)

// nonEventContextPattern matches the HTTP/message/channel reply contexts
// ({$request.*}, {$message.*}, {$channel.*}).
var nonEventContextPattern = regexp.MustCompile(`\{\$(request|message|channel)\.`)

// MatchReferencesEvent reports whether any condition in a match references the
// event context. A match with no and no event reference is a plain reply match.
func MatchReferencesEvent(match map[string]any) bool {
	for expr, condition := range match {
		if ReferencesEvent(expr, condition) {
			return true
		}
	}
	return false
}

// MatchMixedContext reports whether a match mixes event conditions with
// reply-path ({$request.*}/{$message.*}/{$channel.*}) conditions, which is
// rejected at load (RS.EXT.20).
func MatchMixedContext(match map[string]any) bool {
	hasEvent := false
	hasNonEvent := false
	for expr, condition := range match {
		if ReferencesEvent(expr, condition) {
			hasEvent = true
		}
		if ReferencesNonEventContext(expr, condition) {
			hasNonEvent = true
		}
	}
	return hasEvent && hasNonEvent
}

// EventIdentity extracts the identity condition from a match: the literal value
// of a "{$event.name}" condition. It returns ("", false) when the match does
// not pin an identity.
func EventIdentity(match map[string]any) (string, bool) {
	for expr, condition := range match {
		if !eventRefPattern.MatchString(expr) {
			continue
		}
		if expr != "{$event.name}" {
			continue
		}
		if s, ok := condition.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// PartitionConnectionConditions splits an x-mock-match into the conditions
// that reference {$connection.*} (the per-connection recipient filter) and the
// conditions that do not (evaluated once per emission). An empty connection
// bucket means delivery broadcasts to all consumers of the channel (design D6,
// RS.EXT.24-25).
func PartitionConnectionConditions(pm ParamsMatch) (common, connection ParamsMatch) {
	common = make(ParamsMatch)
	connection = make(ParamsMatch)
	for expr, condition := range pm {
		if referencesConnection(expr, condition) {
			connection[expr] = condition
		} else {
			common[expr] = condition
		}
	}
	return common, connection
}

// MatchReferencesConnection reports whether any condition in a match references
// the connection context (so the example needs a per-connection recipient
// decision at delivery).
func MatchReferencesConnection(match map[string]any) bool {
	for expr, condition := range match {
		if referencesConnection(expr, condition) {
			return true
		}
	}
	return false
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
