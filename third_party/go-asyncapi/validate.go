package asyncapi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

//go:embed internal/jsonschema/asyncapi-3.0.0.json
var asyncAPISchema []byte

var schemaCompiler *jsonschema.Compiler
var compiledSchema *jsonschema.Schema

func init() {
	schemaCompiler = jsonschema.NewCompiler()
	schemaCompiler.Draft = jsonschema.Draft7

	if err := schemaCompiler.AddResource("asyncapi-3.0.0.json", bytes.NewReader(asyncAPISchema)); err != nil {
		panic(fmt.Sprintf("failed to add AsyncAPI schema: %v", err))
	}

	var err error
	compiledSchema, err = schemaCompiler.Compile("asyncapi-3.0.0.json")
	if err != nil {
		panic(fmt.Sprintf("failed to compile AsyncAPI schema: %v", err))
	}
}

// Validate validates the document against JSON Schema and semantic rules.
func (d *Document) Validate() error {
	result := d.ValidateAll()
	if !result.IsValid() {
		return &ParseError{Message: result.Error()}
	}
	return nil
}

// ValidateAll performs all validation and returns detailed results.
func (d *Document) ValidateAll() *ValidationResult {
	result := &ValidationResult{}

	// Layer 1: Basic required field checks
	d.validateRequired(result)

	// Layer 2: JSON Schema validation
	d.validateJSONSchema(result)

	// Layer 3: Semantic validation
	d.validateSemantics(result)

	return result
}

func (d *Document) validateRequired(result *ValidationResult) {
	if d.AsyncAPI == "" {
		result.Add("/asyncapi", "asyncapi version is required")
	}
	if d.Info.Title == "" {
		result.Add("/info/title", "title is required")
	}
	if d.Info.Version == "" {
		result.Add("/info/version", "version is required")
	}
}

func (d *Document) validateJSONSchema(result *ValidationResult) {
	if len(d.raw) == 0 {
		return
	}

	// Convert YAML to JSON for schema validation if needed
	var jsonData []byte
	if isJSON(d.raw) {
		jsonData = d.raw
	} else {
		var obj interface{}
		if err := yaml.Unmarshal(d.raw, &obj); err != nil {
			result.Add("/", fmt.Sprintf("failed to parse for validation: %v", err))
			return
		}
		var err error
		jsonData, err = json.Marshal(obj)
		if err != nil {
			result.Add("/", fmt.Sprintf("failed to convert to JSON: %v", err))
			return
		}
	}

	var doc interface{}
	if err := json.Unmarshal(jsonData, &doc); err != nil {
		result.Add("/", fmt.Sprintf("failed to parse JSON: %v", err))
		return
	}

	if err := compiledSchema.Validate(doc); err != nil {
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			addSchemaErrors(result, ve, "")
		} else {
			result.Add("/", fmt.Sprintf("schema validation failed: %v", err))
		}
	}
}

func addSchemaErrors(result *ValidationResult, ve *jsonschema.ValidationError, prefix string) {
	if ve.Message != "" {
		path := prefix
		if ve.InstanceLocation != "" {
			path = ve.InstanceLocation
		}
		result.Add(path, ve.Message)
	}
	for _, cause := range ve.Causes {
		addSchemaErrors(result, cause, prefix)
	}
}

func (d *Document) validateSemantics(result *ValidationResult) {
	// Unique operationIds
	d.validateUniqueOperationIDs(result)

	// Channel parameter resolution
	d.validateChannelParameters(result)

	// Server variable resolution
	d.validateServerVariables(result)

	// Unique tags
	d.validateUniqueTags(result)
}

func (d *Document) validateUniqueOperationIDs(result *ValidationResult) {
	seen := make(map[string]string)
	for id := range d.Operations {
		if existing, ok := seen[id]; ok {
			result.Add("/operations/"+id, fmt.Sprintf("duplicate operationId (also at %s)", existing))
		}
		seen[id] = "/operations/" + id
	}
}

func (d *Document) validateChannelParameters(result *ValidationResult) {
	paramRegex := regexp.MustCompile(`\{([^}]+)\}`)

	for name, chRef := range d.Channels {
		if chRef == nil || chRef.Value == nil || chRef.Value.Address == nil {
			continue
		}

		address := *chRef.Value.Address
		matches := paramRegex.FindAllStringSubmatch(address, -1)

		for _, match := range matches {
			paramName := match[1]
			if _, ok := chRef.Value.Parameters[paramName]; !ok {
				result.Add(
					fmt.Sprintf("/channels/%s", name),
					fmt.Sprintf("parameter {%s} in address not defined in parameters", paramName),
				)
			}
		}
	}
}

func (d *Document) validateServerVariables(result *ValidationResult) {
	varRegex := regexp.MustCompile(`\{([^}]+)\}`)

	for name, srvRef := range d.Servers {
		if srvRef == nil || srvRef.Value == nil {
			continue
		}

		srv := srvRef.Value
		varsToCheck := []string{srv.Host, srv.Pathname}

		for _, str := range varsToCheck {
			if str == "" {
				continue
			}

			matches := varRegex.FindAllStringSubmatch(str, -1)
			for _, match := range matches {
				varName := match[1]
				if _, ok := srv.Variables[varName]; !ok {
					result.Add(
						fmt.Sprintf("/servers/%s", name),
						fmt.Sprintf("variable {%s} not defined in variables", varName),
					)
				}
			}
		}
	}
}

func (d *Document) validateUniqueTags(result *ValidationResult) {
	// Check info tags
	if d.Info.Tags != nil {
		seen := make(map[string]bool)
		for i, tagRef := range d.Info.Tags {
			if tagRef == nil || tagRef.Value == nil {
				continue
			}
			if seen[tagRef.Value.Name] {
				result.Add(
					fmt.Sprintf("/info/tags/%d", i),
					fmt.Sprintf("duplicate tag name: %s", tagRef.Value.Name),
				)
			}
			seen[tagRef.Value.Name] = true
		}
	}

	// Check operation tags
	for opName, opRef := range d.Operations {
		if opRef == nil || opRef.Value == nil || opRef.Value.Tags == nil {
			continue
		}
		seen := make(map[string]bool)
		for i, tagRef := range opRef.Value.Tags {
			if tagRef == nil || tagRef.Value == nil {
				continue
			}
			if seen[tagRef.Value.Name] {
				result.Add(
					fmt.Sprintf("/operations/%s/tags/%d", opName, i),
					fmt.Sprintf("duplicate tag name: %s", tagRef.Value.Name),
				)
			}
			seen[tagRef.Value.Name] = true
		}
	}
}

// ValidateSchema validates just the JSON Schema layer.
func (d *Document) ValidateSchema() error {
	result := &ValidationResult{}
	d.validateJSONSchema(result)
	if !result.IsValid() {
		return &ParseError{Message: result.Error()}
	}
	return nil
}

// ValidateSemantics validates just the semantic rules.
func (d *Document) ValidateSemantics() error {
	result := &ValidationResult{}
	d.validateSemantics(result)
	if !result.IsValid() {
		return &ParseError{Message: result.Error()}
	}
	return nil
}

// isJSON is duplicated here to avoid import cycle
func isJSONValidate(data []byte) bool {
	data = bytes.TrimSpace(data)
	return len(data) > 0 && (data[0] == '{' || data[0] == '[')
}

// Suppress unused import warning
var _ = strings.Contains
