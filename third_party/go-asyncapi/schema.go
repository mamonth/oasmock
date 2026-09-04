package asyncapi

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// Schema is JSON Schema Draft-07 with AsyncAPI extensions.
type Schema struct {
	// Core vocabulary
	ID     string `json:"$id,omitempty" yaml:"$id,omitempty"`
	Schema string `json:"$schema,omitempty" yaml:"$schema,omitempty"`

	// Type
	Type  Types `json:"type,omitempty" yaml:"type,omitempty"`
	Const any   `json:"const,omitempty" yaml:"const,omitempty"`
	Enum  []any `json:"enum,omitempty" yaml:"enum,omitempty"`

	// Numeric
	MultipleOf       *float64 `json:"multipleOf,omitempty" yaml:"multipleOf,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty" yaml:"maximum,omitempty"`
	ExclusiveMaximum *float64 `json:"exclusiveMaximum,omitempty" yaml:"exclusiveMaximum,omitempty"`
	Minimum          *float64 `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	ExclusiveMinimum *float64 `json:"exclusiveMinimum,omitempty" yaml:"exclusiveMinimum,omitempty"`

	// String
	MaxLength *int64 `json:"maxLength,omitempty" yaml:"maxLength,omitempty"`
	MinLength *int64 `json:"minLength,omitempty" yaml:"minLength,omitempty"`
	Pattern   string `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Format    string `json:"format,omitempty" yaml:"format,omitempty"`

	// Content
	ContentEncoding  string `json:"contentEncoding,omitempty" yaml:"contentEncoding,omitempty"`
	ContentMediaType string `json:"contentMediaType,omitempty" yaml:"contentMediaType,omitempty"`

	// Array
	Items           *SchemaRef    `json:"items,omitempty" yaml:"items,omitempty"`
	AdditionalItems *BoolOrSchema `json:"additionalItems,omitempty" yaml:"additionalItems,omitempty"`
	MaxItems        *int64        `json:"maxItems,omitempty" yaml:"maxItems,omitempty"`
	MinItems        *int64        `json:"minItems,omitempty" yaml:"minItems,omitempty"`
	UniqueItems     bool          `json:"uniqueItems,omitempty" yaml:"uniqueItems,omitempty"`
	Contains        *SchemaRef    `json:"contains,omitempty" yaml:"contains,omitempty"`

	// Object
	Properties           map[string]*SchemaRef `json:"properties,omitempty" yaml:"properties,omitempty"`
	Required             []string              `json:"required,omitempty" yaml:"required,omitempty"`
	AdditionalProperties *BoolOrSchema         `json:"additionalProperties,omitempty" yaml:"additionalProperties,omitempty"`
	MaxProperties        *int64                `json:"maxProperties,omitempty" yaml:"maxProperties,omitempty"`
	MinProperties        *int64                `json:"minProperties,omitempty" yaml:"minProperties,omitempty"`
	PatternProperties    map[string]*SchemaRef `json:"patternProperties,omitempty" yaml:"patternProperties,omitempty"`
	PropertyNames        *SchemaRef            `json:"propertyNames,omitempty" yaml:"propertyNames,omitempty"`
	Dependencies         map[string]*SchemaRef `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`

	// Composition
	AllOf []*SchemaRef `json:"allOf,omitempty" yaml:"allOf,omitempty"`
	AnyOf []*SchemaRef `json:"anyOf,omitempty" yaml:"anyOf,omitempty"`
	OneOf []*SchemaRef `json:"oneOf,omitempty" yaml:"oneOf,omitempty"`
	Not   *SchemaRef   `json:"not,omitempty" yaml:"not,omitempty"`

	// Conditionals
	If   *SchemaRef `json:"if,omitempty" yaml:"if,omitempty"`
	Then *SchemaRef `json:"then,omitempty" yaml:"then,omitempty"`
	Else *SchemaRef `json:"else,omitempty" yaml:"else,omitempty"`

	// Metadata
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
	Examples    []any  `json:"examples,omitempty" yaml:"examples,omitempty"`
	ReadOnly    bool   `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	WriteOnly   bool   `json:"writeOnly,omitempty" yaml:"writeOnly,omitempty"`

	// Definitions
	Definitions map[string]*SchemaRef `json:"definitions,omitempty" yaml:"definitions,omitempty"`

	// AsyncAPI extensions
	Discriminator string           `json:"discriminator,omitempty" yaml:"discriminator,omitempty"`
	ExternalDocs  *ExternalDocsRef `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	Deprecated    bool             `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
}

// Types handles JSON Schema type which can be string or []string.
type Types []string

func (t *Types) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*t = Types{single}
		return nil
	}
	var multi []string
	if err := json.Unmarshal(data, &multi); err != nil {
		return err
	}
	*t = multi
	return nil
}

func (t *Types) UnmarshalYAML(node *yaml.Node) error {
	var single string
	if err := node.Decode(&single); err == nil {
		*t = Types{single}
		return nil
	}
	var multi []string
	if err := node.Decode(&multi); err != nil {
		return err
	}
	*t = multi
	return nil
}

func (t Types) MarshalJSON() ([]byte, error) {
	if len(t) == 1 {
		return json.Marshal(t[0])
	}
	return json.Marshal([]string(t))
}

// Contains returns true if the type list contains the given type.
func (t Types) Contains(typ string) bool {
	for _, v := range t {
		if v == typ {
			return true
		}
	}
	return false
}

// Is returns true if the type list has exactly one type matching the given type.
func (t Types) Is(typ string) bool {
	return len(t) == 1 && t[0] == typ
}

// BoolOrSchema handles additionalProperties/additionalItems which can be bool or Schema.
type BoolOrSchema struct {
	Allowed bool    // if false, no additional properties/items
	Schema  *Schema // if non-nil, additional properties/items must match
}

func (b *BoolOrSchema) UnmarshalJSON(data []byte) error {
	var boolVal bool
	if err := json.Unmarshal(data, &boolVal); err == nil {
		b.Allowed = boolVal
		b.Schema = nil
		return nil
	}
	b.Allowed = true
	b.Schema = &Schema{}
	return json.Unmarshal(data, b.Schema)
}

func (b *BoolOrSchema) UnmarshalYAML(node *yaml.Node) error {
	var boolVal bool
	if err := node.Decode(&boolVal); err == nil {
		b.Allowed = boolVal
		b.Schema = nil
		return nil
	}
	b.Allowed = true
	b.Schema = &Schema{}
	return node.Decode(b.Schema)
}

func (b *BoolOrSchema) MarshalJSON() ([]byte, error) {
	if b.Schema != nil {
		return json.Marshal(b.Schema)
	}
	return json.Marshal(b.Allowed)
}
