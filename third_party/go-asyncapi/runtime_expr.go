package asyncapi

import (
	"fmt"
	"strings"
)

// RuntimeExpr represents a parsed runtime expression.
// Format: $message.source#/json/pointer
type RuntimeExpr struct {
	Source   string // "message"
	Location string // "header" or "payload"
	Pointer  string // JSON pointer, e.g., "/correlationId"
}

// ParseRuntimeExpr parses a runtime expression like "$message.header#/correlationId".
func ParseRuntimeExpr(expr string) (*RuntimeExpr, error) {
	if !strings.HasPrefix(expr, "$") {
		return nil, fmt.Errorf("runtime expression must start with $: %s", expr)
	}

	expr = expr[1:] // Remove leading $

	// Split on #
	parts := strings.SplitN(expr, "#", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("runtime expression must contain #: %s", expr)
	}

	// Parse source.location
	sourceParts := strings.SplitN(parts[0], ".", 2)
	if len(sourceParts) != 2 {
		return nil, fmt.Errorf("runtime expression source must be source.location: %s", parts[0])
	}

	result := &RuntimeExpr{
		Source:   sourceParts[0],
		Location: sourceParts[1],
		Pointer:  parts[1],
	}

	// Validate source
	if result.Source != "message" {
		return nil, fmt.Errorf("runtime expression source must be 'message': %s", result.Source)
	}

	// Validate location
	if result.Location != "header" && result.Location != "payload" {
		return nil, fmt.Errorf("runtime expression location must be 'header' or 'payload': %s", result.Location)
	}

	return result, nil
}

// String returns the string representation of the runtime expression.
func (r *RuntimeExpr) String() string {
	return fmt.Sprintf("$%s.%s#%s", r.Source, r.Location, r.Pointer)
}
