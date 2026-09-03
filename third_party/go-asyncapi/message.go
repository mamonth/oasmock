package asyncapi

import (
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
)

// Message describes a message received on a given channel and operation.
type Message struct {
	Headers       *MultiFormatSchemaRef `json:"headers,omitempty" yaml:"headers,omitempty"`
	Payload       *MultiFormatSchemaRef `json:"payload,omitempty" yaml:"payload,omitempty"`
	CorrelationID *CorrelationIDRef     `json:"correlationId,omitempty" yaml:"correlationId,omitempty"`
	ContentType   string                `json:"contentType,omitempty" yaml:"contentType,omitempty"`
	Name          string                `json:"name,omitempty" yaml:"name,omitempty"`
	Title         string                `json:"title,omitempty" yaml:"title,omitempty"`
	Summary       string                `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description   string                `json:"description,omitempty" yaml:"description,omitempty"`
	Tags          []*TagRef             `json:"tags,omitempty" yaml:"tags,omitempty"`
	ExternalDocs  *ExternalDocsRef      `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	Bindings      *MessageBindingsRef   `json:"bindings,omitempty" yaml:"bindings,omitempty"`
	Examples      []*MessageExample     `json:"examples,omitempty" yaml:"examples,omitempty"`
	Traits        []*MessageTraitRef    `json:"traits,omitempty" yaml:"traits,omitempty"`
}

// MessageTrait describes a trait that may be applied to a Message.
type MessageTrait struct {
	Headers       *MultiFormatSchemaRef `json:"headers,omitempty" yaml:"headers,omitempty"`
	CorrelationID *CorrelationIDRef     `json:"correlationId,omitempty" yaml:"correlationId,omitempty"`
	ContentType   string                `json:"contentType,omitempty" yaml:"contentType,omitempty"`
	Name          string                `json:"name,omitempty" yaml:"name,omitempty"`
	Title         string                `json:"title,omitempty" yaml:"title,omitempty"`
	Summary       string                `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description   string                `json:"description,omitempty" yaml:"description,omitempty"`
	Tags          []*TagRef             `json:"tags,omitempty" yaml:"tags,omitempty"`
	ExternalDocs  *ExternalDocsRef      `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	Bindings      *MessageBindingsRef   `json:"bindings,omitempty" yaml:"bindings,omitempty"`
	Examples      []*MessageExample     `json:"examples,omitempty" yaml:"examples,omitempty"`
}

// MessageExample represents an example of a Message.
type MessageExample struct {
	Headers map[string]any `json:"headers,omitempty" yaml:"headers,omitempty"`
	Payload any            `json:"payload,omitempty" yaml:"payload,omitempty"`
	Name    string         `json:"name,omitempty" yaml:"name,omitempty"`
	Summary string         `json:"summary,omitempty" yaml:"summary,omitempty"`

	extensions map[string]any
}

// Extension returns a spec extension by name (e.g., "x-mock-match").
func (e *MessageExample) Extension(name string) (any, bool) {
	if e == nil || e.extensions == nil {
		return nil, false
	}
	v, ok := e.extensions[name]
	return v, ok
}

// Extensions returns a copy of all spec extensions of the example.
func (e *MessageExample) Extensions() map[string]any {
	out := make(map[string]any, len(e.extensions))
	for k, v := range e.extensions {
		out[k] = v
	}
	return out
}

// captureExtensions records all x-* fields from a generic map.
func (e *MessageExample) captureExtensions(m map[string]any) {
	for k, v := range m {
		if strings.HasPrefix(k, "x-") {
			if e.extensions == nil {
				e.extensions = make(map[string]any)
			}
			e.extensions[k] = v
		}
	}
}

// UnmarshalJSON keeps unknown x-* keys on the example.
func (e *MessageExample) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	e.captureExtensions(m)
	// Drop extension keys so the strict struct decode below ignores them.
	for k := range m {
		if strings.HasPrefix(k, "x-") {
			delete(m, k)
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	type plain MessageExample
	return json.Unmarshal(raw, (*plain)(e))
}

// UnmarshalYAML keeps unknown x-* keys on the example.
func (e *MessageExample) UnmarshalYAML(node *yaml.Node) error {
	var m map[string]any
	if err := node.Decode(&m); err != nil {
		return err
	}
	e.captureExtensions(m)
	for k := range m {
		if strings.HasPrefix(k, "x-") {
			delete(m, k)
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	type plain MessageExample
	return json.Unmarshal(raw, (*plain)(e))
}

// CorrelationID specifies an identifier for message tracing or matching.
type CorrelationID struct {
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Location    string `json:"location" yaml:"location"` // runtime expression
}
