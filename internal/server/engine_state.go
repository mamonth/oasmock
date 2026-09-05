package server

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/mamonth/oasmock/internal/runtime"
)

// ---------------------------------------------------------------------------
// State mutation (x-mock-set-state)
// ---------------------------------------------------------------------------

func (e *exampleEngine) handleDeleteState(prefix, resolvedKey string) {
	e.stateStore.Delete(prefix, resolvedKey)
	if e.verbose {
		slog.Debug("Deleted state", "key", resolvedKey, "namespace", prefix)
	}
}

func (e *exampleEngine) handleIncrementState(prefix, resolvedKey string, incVal any, eval runtime.Evaluator) error {
	resolvedInc, err := e.evaluateValue(incVal, eval)
	if err != nil {
		if e.verbose {
			slog.Debug("Failed to evaluate increment value", "error", err)
		}
		return err
	}
	// Convert to float64
	var delta float64
	switch v := resolvedInc.(type) {
	case float64:
		delta = v
	case int:
		delta = float64(v)
	case string:
		// Try to parse as number
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			delta = f
		} else {
			if e.verbose {
				slog.Debug("Increment value is not a number", "value", v)
			}
			return fmt.Errorf("increment value is not a number: %s", v)
		}
	default:
		if e.verbose {
			slog.Debug("Increment value has unsupported type", "type", fmt.Sprintf("%T", v))
		}
		return fmt.Errorf("increment value has unsupported type: %T", v)
	}
	newVal, err := e.stateStore.Increment(prefix, resolvedKey, delta)
	if err != nil {
		if e.verbose {
			slog.Debug("Failed to increment state", "key", resolvedKey, "error", err)
		}
		return err
	}
	if e.verbose {
		slog.Debug("Incremented state", "key", resolvedKey, "namespace", prefix, "delta", delta, "newValue", newVal)
	}
	return nil
}

func (e *exampleEngine) handleValueObjectState(prefix, resolvedKey string, valObj any, eval runtime.Evaluator) error {
	resolvedVal, err := e.evaluateValue(valObj, eval)
	if err != nil {
		if e.verbose {
			slog.Debug("Failed to evaluate value object", "error", err)
		}
		return err
	}
	e.stateStore.Set(prefix, resolvedKey, resolvedVal)
	if e.verbose {
		slog.Debug("Set state", "key", resolvedKey, "namespace", prefix, "value", resolvedVal)
	}
	return nil
}

func (e *exampleEngine) handleMapState(prefix, resolvedKey string, m map[string]any, eval runtime.Evaluator) (handled bool, err error) {
	if incVal, hasInc := m["increment"]; hasInc {
		err = e.handleIncrementState(prefix, resolvedKey, incVal, eval)
		return true, err
	}
	if valObj, hasVal := m["value"]; hasVal {
		err = e.handleValueObjectState(prefix, resolvedKey, valObj, eval)
		return true, err
	}
	return false, nil
}

func (e *exampleEngine) handleSimpleState(prefix, resolvedKey string, val any, eval runtime.Evaluator) error {
	resolvedVal, err := e.evaluateValue(val, eval)
	if err != nil {
		if e.verbose {
			slog.Debug("Failed to evaluate value for key", "key", resolvedKey, "error", err)
		}
		return err
	}
	e.stateStore.Set(prefix, resolvedKey, resolvedVal)
	if e.verbose {
		slog.Debug("Set state", "key", resolvedKey, "namespace", prefix, "value", resolvedVal)
	}
	return nil
}

func (e *exampleEngine) ApplySetState(stateMap map[string]any, eval runtime.Evaluator, prefix string) {
	for key, val := range stateMap {
		// Evaluate runtime expressions in key
		resolvedKey, err := e.evaluateExpressionInString(key, eval)
		if err != nil {
			if e.verbose {
				slog.Debug("Failed to evaluate key", "key", key, "error", err)
			}
			continue
		}

		// Handle null value (delete)
		if val == nil {
			e.handleDeleteState(prefix, resolvedKey)
			continue
		}

		// Handle map (increment or value object)
		if m, ok := val.(map[string]any); ok {
			handled, _ := e.handleMapState(prefix, resolvedKey, m, eval)
			if handled {
				// Error already logged inside helpers
				continue
			}
			// Not a recognized map structure, fall through to simple value
		}

		// Simple value (could be runtime expression)
		if err := e.handleSimpleState(prefix, resolvedKey, val, eval); err != nil {
			// Error already logged inside helper
			continue
		}
	}
}
