package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mamonth/oasmock/internal/runtime"
)

// ---------------------------------------------------------------------------
// Runtime expression evaluation
// ---------------------------------------------------------------------------

func (e *exampleEngine) replaceEmbeddedExpressions(str string, eval runtime.Evaluator) (string, error) {
	var result strings.Builder
	i := 0
	for i < len(str) {
		// Find start of expression "{$"
		start := strings.Index(str[i:], "{$")
		if start == -1 {
			result.WriteString(str[i:])
			break
		}
		start += i
		// Write literal part before expression
		result.WriteString(str[i:start])
		// Find matching '}'
		braceDepth := 1
		j := start + 2
		for j < len(str) && braceDepth > 0 {
			ch := str[j]
			if ch == '{' && j+1 < len(str) && str[j+1] == '$' {
				braceDepth++
				j += 2
				continue
			}
			if ch == '}' {
				braceDepth--
				if braceDepth == 0 {
					break
				}
			}
			j++
		}
		if braceDepth != 0 {
			// Unmatched braces, treat as literal
			result.WriteString(str[start:])
			break
		}
		// j now points at the closing '}'
		end := j
		expr := str[start : end+1]
		// Evaluate expression
		value, err := eval.Evaluate(expr)
		if err != nil {
			// If evaluation fails, keep the original expression
			result.WriteString(expr)
		} else {
			// Convert value to string
			switch v := value.(type) {
			case string:
				result.WriteString(v)
			default:
				b, err := json.Marshal(v)
				if err != nil {
					result.WriteString(expr)
				} else {
					result.Write(b)
				}
			}
		}
		i = end + 1
	}
	return result.String(), nil
}

func (e *exampleEngine) evaluateExpressionInString(str string, eval runtime.Evaluator) (string, error) {
	// First, check if the whole string is a runtime expression (optimization)
	if strings.HasPrefix(str, "{$") && strings.HasSuffix(str, "}") && !strings.Contains(str[2:], "{$") {
		result, err := eval.Evaluate(str)
		if err != nil {
			return "", err
		}
		// Convert result to string
		switch v := result.(type) {
		case string:
			return v, nil
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
	}
	// Otherwise replace embedded expressions
	return e.replaceEmbeddedExpressions(str, eval)
}

// maxEvaluationDepth bounds recursive templating of nested values so a
// pathological dynamic example cannot overflow the stack.
const maxEvaluationDepth = 256

func (e *exampleEngine) evaluateValue(val any, eval runtime.Evaluator) (any, error) {
	return e.evaluateValueDepth(val, eval, 0)
}

func (e *exampleEngine) evaluateValueDepth(val any, eval runtime.Evaluator, depth int) (any, error) {
	if depth > maxEvaluationDepth {
		return nil, fmt.Errorf("value nesting exceeds maximum depth of %d", maxEvaluationDepth)
	}
	// Handle strings: they may contain embedded runtime expressions
	if str, ok := val.(string); ok {
		// Check if the whole string is a single runtime expression (no other characters)
		if strings.HasPrefix(str, "{$") && strings.HasSuffix(str, "}") && strings.Count(str, "{$") == 1 {
			return eval.Evaluate(str)
		}
		// Otherwise replace embedded expressions
		return e.replaceEmbeddedExpressions(str, eval)
	}
	// Recursively handle maps and slices
	switch v := val.(type) {
	case map[string]any:
		result := make(map[string]any)
		for k, item := range v {
			resolvedK, err := e.evaluateExpressionInString(k, eval)
			if err != nil {
				return nil, err
			}
			resolvedItem, err := e.evaluateValueDepth(item, eval, depth+1)
			if err != nil {
				return nil, err
			}
			result[resolvedK] = resolvedItem
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			resolvedItem, err := e.evaluateValueDepth(item, eval, depth+1)
			if err != nil {
				return nil, err
			}
			result[i] = resolvedItem
		}
		return result, nil
	default:
		// Literal value
		return val, nil
	}
}
