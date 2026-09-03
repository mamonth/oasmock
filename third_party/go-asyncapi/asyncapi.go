// Package asyncapi provides a parser, validator, and utilities for AsyncAPI 3.0.0 documents.
//
// The package follows the AsyncAPI 3.0.0 specification as the source of truth.
// See https://www.asyncapi.com/docs/reference/specification/v3.0.0
//
// Basic usage:
//
//	doc, err := asyncapi.LoadFromFile("api.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	if err := doc.Validate(); err != nil {
//	    log.Fatal(err)
//	}
//
//	for name, op := range doc.Operations {
//	    fmt.Printf("Operation %s: %s\n", name, op.Value.Action)
//	}
package asyncapi
