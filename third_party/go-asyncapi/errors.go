// Package asyncapi provides a parser and validator for AsyncAPI 3.0.0 documents.
package asyncapi

import (
	"fmt"
	"strings"
)

// ParseError represents an error that occurred during parsing.
type ParseError struct {
	Message string
	Cause   error
}

func (e *ParseError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *ParseError) Unwrap() error {
	return e.Cause
}

// ValidationError represents a validation error at a specific path.
type ValidationError struct {
	Path    string // JSON pointer to error location
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationResult contains the results of document validation.
type ValidationResult struct {
	Errors []ValidationError
}

// IsValid returns true if there are no validation errors.
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// Error returns a combined error message for all validation errors.
func (r *ValidationResult) Error() string {
	if r.IsValid() {
		return ""
	}
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// Add adds a validation error to the result.
func (r *ValidationResult) Add(path, message string) {
	r.Errors = append(r.Errors, ValidationError{Path: path, Message: message})
}

// RefError represents an error during reference resolution.
type RefError struct {
	Ref     string
	Message string
	Cause   error
}

func (e *RefError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("$ref %q: %s: %v", e.Ref, e.Message, e.Cause)
	}
	return fmt.Sprintf("$ref %q: %s", e.Ref, e.Message)
}

func (e *RefError) Unwrap() error {
	return e.Cause
}
