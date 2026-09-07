// Command gen-control-schema derives the runtime request-validation JSON schema
// for POST /_mock/examples directly from api/openapi.yaml, so the Go server
// never hand-duplicates the OpenAPI contract (single source of truth).
//
// It resolves components.schemas.AddExampleRequest into a standalone, fully
// inlined JSON schema (following every $ref within components.schemas) and
// writes a Go source file exposing that schema as a byte constant.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	schemaName    = "AddExampleRequest"
	jsonSchemaRef = "#/components/schemas/"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen-control-schema:", err)
		os.Exit(1)
	}
}

func run() error {
	inPath, outPath, pkg, err := parseFlags()
	if err != nil {
		return err
	}
	schemas, err := loadSchemas(inPath)
	if err != nil {
		return err
	}
	source, ok := schemas[schemaName]
	if !ok {
		return fmt.Errorf("components.schemas.%s not found in %s", schemaName, inPath)
	}

	resolved, err := deref(source, schemas)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", schemaName, err)
	}
	if containsRef(resolved) {
		return fmt.Errorf("resolve left an unresolvable $ref in %s; schemas must stay acyclic and self-contained", schemaName)
	}

	jsonBytes, err := json.MarshalIndent(resolved, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal resolved schema: %w", err)
	}
	output, err := buildGoSource(jsonBytes, pkg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, output, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	return nil
}

// parseFlags reads the -in/-out/-pkg flags and validates them.
func parseFlags() (inPath, outPath, pkg string, err error) {
	flag.StringVar(&inPath, "in", "", "path to api/openapi.yaml")
	flag.StringVar(&outPath, "out", "", "path to generated Go file")
	flag.StringVar(&pkg, "pkg", "server", "name of the generated Go package")
	flag.Parse()
	if inPath == "" || outPath == "" {
		return "", "", "", fmt.Errorf("-in and -out are required")
	}
	if pkg == "" {
		return "", "", "", fmt.Errorf("-pkg must not be empty")
	}
	return inPath, outPath, pkg, nil
}

// loadSchemas reads and parses the components.schemas of an OpenAPI document.
func loadSchemas(inPath string) (map[string]any, error) {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", inPath, err)
	}

	var doc struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", inPath, err)
	}
	if doc.Components.Schemas == nil {
		return nil, fmt.Errorf("no components.schemas found in %s", inPath)
	}
	return doc.Components.Schemas, nil
}

// deref returns a deep copy of src with every $ref to components.schemas
// replaced by the referenced schema, recursively. Refs outside
// components.schemas (e.g. examples) are left untouched; any remaining ref
// aborts generation (see containsRef). Circular $refs are rejected: a schema
// that cannot be inlined into a finite artifact is an error, not a hang.
func deref(src any, schemas map[string]any) (any, error) {
	return derefImpl(src, schemas, map[string]bool{})
}

func derefImpl(src any, schemas map[string]any, stack map[string]bool) (any, error) {
	switch v := src.(type) {
	case map[string]any:
		return derefMap(v, schemas, stack)
	case []any:
		return derefSlice(v, schemas, stack)
	default:
		return src, nil
	}
}

func derefMap(v map[string]any, schemas map[string]any, stack map[string]bool) (any, error) {
	out := make(map[string]any, len(v))
	for k, val := range v {
		if k == "$ref" {
			ref, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("$ref value is not a string: %v", val)
			}
			if strings.HasPrefix(ref, jsonSchemaRef) {
				resolved, err := inlineRef(ref, schemas, stack)
				if err != nil {
					return nil, err
				}
				return resolved, nil
			}
			out[k] = val
			continue
		}
		child, err := derefImpl(val, schemas, stack)
		if err != nil {
			return nil, err
		}
		out[k] = child
	}
	return out, nil
}

func derefSlice(v []any, schemas map[string]any, stack map[string]bool) (any, error) {
	out := make([]any, len(v))
	for i, item := range v {
		child, err := derefImpl(item, schemas, stack)
		if err != nil {
			return nil, err
		}
		out[i] = child
	}
	return out, nil
}

// inlineRef resolves a components.schemas ref to its target schema, guarding
// against cycles, so a recursive schema fails loudly instead of recursing
// forever during inlining.
func inlineRef(ref string, schemas map[string]any, stack map[string]bool) (any, error) {
	name := strings.TrimPrefix(ref, jsonSchemaRef)
	target, ok := schemas[name]
	if !ok {
		return nil, fmt.Errorf("unresolved $ref %q", ref)
	}
	if stack[name] {
		return nil, fmt.Errorf("circular $ref %q; component schemas must be acyclic", ref)
	}
	stack[name] = true
	resolved, err := derefImpl(target, schemas, stack)
	delete(stack, name)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// containsRef reports whether any nested $ref remains in a resolved schema.
func containsRef(v any) bool {
	switch val := v.(type) {
	case map[string]any:
		if _, ok := val["$ref"]; ok {
			return true
		}
		for _, child := range val {
			if containsRef(child) {
				return true
			}
		}
	case []any:
		for _, child := range val {
			if containsRef(child) {
				return true
			}
		}
	}
	return false
}

// buildGoSource renders the resolved schema as a Go byte constant in the given
// package, formatted by go/format.
func buildGoSource(schema []byte, pkg string) ([]byte, error) {
	raw := fmt.Sprintf(`// Code generated by gen-control-schema; DO NOT EDIT.
// Source: api/openapi.yaml -> components.schemas.%[1]s.
// The request body of POST /_mock/examples is validated against this schema at
// runtime, keeping the Go server's validation in lock-step with the OpenAPI
// contract (single source of truth). Regenerate with: go generate ./internal/server

package %[2]s

// %[1]sSchemaJSON is the fully-inlined JSON schema for a %[1]s request body.
var %[1]sSchemaJSON = []byte(%[3]q)
`, schemaName, pkg, string(schema))

	formatted, err := format.Source([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("format generated source: %w", err)
	}
	return formatted, nil
}
