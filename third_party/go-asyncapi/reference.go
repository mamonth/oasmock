package asyncapi

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// Reference represents a JSON Reference ($ref).
type Reference struct {
	Ref string `json:"$ref" yaml:"$ref"`
}

// IsRef returns true if this is a reference (has $ref set).
func (r *Reference) IsRef() bool {
	return r.Ref != ""
}

// refContainer is used for detecting $ref in raw data.
type refContainer struct {
	Ref string `json:"$ref" yaml:"$ref"`
}

// ServerRef wraps a Server that may be a $ref or inline definition.
type ServerRef struct {
	Ref    string  `json:"-" yaml:"-"`
	Value  *Server `json:"-" yaml:"-"`
	inline Server
}

func (r *ServerRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *ServerRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *ServerRef) MarshalJSON() ([]byte, error) {
	if r.Ref != "" {
		return json.Marshal(Reference{Ref: r.Ref})
	}
	if r.Value != nil {
		return json.Marshal(r.Value)
	}
	return json.Marshal(&r.inline)
}

// ServerVariableRef wraps a ServerVariable that may be a $ref or inline definition.
type ServerVariableRef struct {
	Ref    string          `json:"-" yaml:"-"`
	Value  *ServerVariable `json:"-" yaml:"-"`
	inline ServerVariable
}

func (r *ServerVariableRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *ServerVariableRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// ChannelRef wraps a Channel that may be a $ref or inline definition.
type ChannelRef struct {
	Ref    string   `json:"-" yaml:"-"`
	Value  *Channel `json:"-" yaml:"-"`
	inline Channel
}

func (r *ChannelRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *ChannelRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// ParameterRef wraps a Parameter that may be a $ref or inline definition.
type ParameterRef struct {
	Ref    string     `json:"-" yaml:"-"`
	Value  *Parameter `json:"-" yaml:"-"`
	inline Parameter
}

func (r *ParameterRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *ParameterRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// OperationRef wraps an Operation that may be a $ref or inline definition.
type OperationRef struct {
	Ref    string     `json:"-" yaml:"-"`
	Value  *Operation `json:"-" yaml:"-"`
	inline Operation
}

func (r *OperationRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *OperationRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// OperationTraitRef wraps an OperationTrait that may be a $ref or inline definition.
type OperationTraitRef struct {
	Ref    string          `json:"-" yaml:"-"`
	Value  *OperationTrait `json:"-" yaml:"-"`
	inline OperationTrait
}

func (r *OperationTraitRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *OperationTraitRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// OperationReplyRef wraps an OperationReply that may be a $ref or inline definition.
type OperationReplyRef struct {
	Ref    string          `json:"-" yaml:"-"`
	Value  *OperationReply `json:"-" yaml:"-"`
	inline OperationReply
}

func (r *OperationReplyRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *OperationReplyRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// ReplyAddressRef wraps a ReplyAddress that may be a $ref or inline definition.
type ReplyAddressRef struct {
	Ref    string        `json:"-" yaml:"-"`
	Value  *ReplyAddress `json:"-" yaml:"-"`
	inline ReplyAddress
}

func (r *ReplyAddressRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *ReplyAddressRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// MessageRef wraps a Message that may be a $ref or inline definition.
type MessageRef struct {
	Ref    string   `json:"-" yaml:"-"`
	Value  *Message `json:"-" yaml:"-"`
	inline Message
}

func (r *MessageRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *MessageRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// MessageTraitRef wraps a MessageTrait that may be a $ref or inline definition.
type MessageTraitRef struct {
	Ref    string        `json:"-" yaml:"-"`
	Value  *MessageTrait `json:"-" yaml:"-"`
	inline MessageTrait
}

func (r *MessageTraitRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *MessageTraitRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// TagRef wraps a Tag that may be a $ref or inline definition.
type TagRef struct {
	Ref    string `json:"-" yaml:"-"`
	Value  *Tag   `json:"-" yaml:"-"`
	inline Tag
}

func (r *TagRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *TagRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// ExternalDocsRef wraps an ExternalDocs that may be a $ref or inline definition.
type ExternalDocsRef struct {
	Ref    string        `json:"-" yaml:"-"`
	Value  *ExternalDocs `json:"-" yaml:"-"`
	inline ExternalDocs
}

func (r *ExternalDocsRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *ExternalDocsRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// SchemaRef wraps a Schema that may be a $ref or inline definition.
type SchemaRef struct {
	Ref    string  `json:"-" yaml:"-"`
	Value  *Schema `json:"-" yaml:"-"`
	inline Schema
}

func (r *SchemaRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *SchemaRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// MultiFormatSchemaRef wraps a schema with optional schemaFormat.
type MultiFormatSchemaRef struct {
	Ref          string  `json:"-" yaml:"-"`
	SchemaFormat string  `json:"schemaFormat,omitempty" yaml:"schemaFormat,omitempty"`
	Value        *Schema `json:"-" yaml:"-"`
	inline       Schema
}

func (r *MultiFormatSchemaRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	// Try as MultiFormatSchema first
	var mfs struct {
		SchemaFormat string          `json:"schemaFormat"`
		Schema       json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(data, &mfs); err == nil && mfs.SchemaFormat != "" {
		r.SchemaFormat = mfs.SchemaFormat
		if len(mfs.Schema) > 0 {
			if err := json.Unmarshal(mfs.Schema, &r.inline); err != nil {
				return err
			}
			r.Value = &r.inline
		}
		return nil
	}
	// Otherwise treat as inline Schema
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *MultiFormatSchemaRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	// Try as MultiFormatSchema first
	var mfs struct {
		SchemaFormat string    `yaml:"schemaFormat"`
		Schema       yaml.Node `yaml:"schema"`
	}
	if err := node.Decode(&mfs); err == nil && mfs.SchemaFormat != "" {
		r.SchemaFormat = mfs.SchemaFormat
		if mfs.Schema.Kind != 0 {
			if err := mfs.Schema.Decode(&r.inline); err != nil {
				return err
			}
			r.Value = &r.inline
		}
		return nil
	}
	// Otherwise treat as inline Schema
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// SecuritySchemeRef wraps a SecurityScheme that may be a $ref or inline definition.
type SecuritySchemeRef struct {
	Ref    string          `json:"-" yaml:"-"`
	Value  *SecurityScheme `json:"-" yaml:"-"`
	inline SecurityScheme
	// Scopes for this specific usage (oauth2/openIdConnect)
	Scopes []string `json:"scopes,omitempty" yaml:"scopes,omitempty"`
}

func (r *SecuritySchemeRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *SecuritySchemeRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// CorrelationIDRef wraps a CorrelationID that may be a $ref or inline definition.
type CorrelationIDRef struct {
	Ref    string         `json:"-" yaml:"-"`
	Value  *CorrelationID `json:"-" yaml:"-"`
	inline CorrelationID
}

func (r *CorrelationIDRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *CorrelationIDRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// ServerBindingsRef wraps ServerBindings that may be a $ref or inline definition.
type ServerBindingsRef struct {
	Ref    string          `json:"-" yaml:"-"`
	Value  *ServerBindings `json:"-" yaml:"-"`
	inline ServerBindings
}

func (r *ServerBindingsRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *ServerBindingsRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// ChannelBindingsRef wraps ChannelBindings that may be a $ref or inline definition.
type ChannelBindingsRef struct {
	Ref    string           `json:"-" yaml:"-"`
	Value  *ChannelBindings `json:"-" yaml:"-"`
	inline ChannelBindings
}

func (r *ChannelBindingsRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *ChannelBindingsRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// OperationBindingsRef wraps OperationBindings that may be a $ref or inline definition.
type OperationBindingsRef struct {
	Ref    string             `json:"-" yaml:"-"`
	Value  *OperationBindings `json:"-" yaml:"-"`
	inline OperationBindings
}

func (r *OperationBindingsRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *OperationBindingsRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

// MessageBindingsRef wraps MessageBindings that may be a $ref or inline definition.
type MessageBindingsRef struct {
	Ref    string           `json:"-" yaml:"-"`
	Value  *MessageBindings `json:"-" yaml:"-"`
	inline MessageBindings
}

func (r *MessageBindingsRef) UnmarshalJSON(data []byte) error {
	var ref refContainer
	if err := json.Unmarshal(data, &ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := json.Unmarshal(data, &r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}

func (r *MessageBindingsRef) UnmarshalYAML(node *yaml.Node) error {
	var ref refContainer
	if err := node.Decode(&ref); err == nil && ref.Ref != "" {
		r.Ref = ref.Ref
		return nil
	}
	if err := node.Decode(&r.inline); err != nil {
		return err
	}
	r.Value = &r.inline
	return nil
}
