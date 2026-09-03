package asyncapi

import (
	"encoding/json"
)

// Document is the root AsyncAPI 3.0.0 specification object.
type Document struct {
	AsyncAPI           string                   `json:"asyncapi" yaml:"asyncapi"`
	ID                 string                   `json:"id,omitempty" yaml:"id,omitempty"`
	Info               Info                     `json:"info" yaml:"info"`
	Servers            map[string]*ServerRef    `json:"servers,omitempty" yaml:"servers,omitempty"`
	DefaultContentType string                   `json:"defaultContentType,omitempty" yaml:"defaultContentType,omitempty"`
	Channels           map[string]*ChannelRef   `json:"channels,omitempty" yaml:"channels,omitempty"`
	Operations         map[string]*OperationRef `json:"operations,omitempty" yaml:"operations,omitempty"`
	Components         *Components              `json:"components,omitempty" yaml:"components,omitempty"`

	// extensions holds spec extensions (x-* fields)
	extensions map[string]json.RawMessage

	// raw holds the original document bytes for validation
	raw []byte
}

// Extension returns a spec extension by name (e.g., "x-custom").
func (d *Document) Extension(name string) json.RawMessage {
	if d.extensions == nil {
		return nil
	}
	return d.extensions[name]
}

// SetExtension sets a spec extension.
func (d *Document) SetExtension(name string, value json.RawMessage) {
	if d.extensions == nil {
		d.extensions = make(map[string]json.RawMessage)
	}
	d.extensions[name] = value
}

// Raw returns the original document bytes.
func (d *Document) Raw() []byte {
	return d.raw
}

// Version returns the AsyncAPI version string.
func (d *Document) Version() string {
	return d.AsyncAPI
}

// Title returns the API title.
func (d *Document) Title() string {
	return d.Info.Title
}

// GetServer returns a server by name.
func (d *Document) GetServer(name string) *Server {
	if ref, ok := d.Servers[name]; ok && ref != nil && ref.Value != nil {
		return ref.Value
	}
	return nil
}

// GetChannel returns a channel by ID.
func (d *Document) GetChannel(id string) *Channel {
	if ref, ok := d.Channels[id]; ok && ref != nil && ref.Value != nil {
		return ref.Value
	}
	return nil
}

// GetOperation returns an operation by ID.
func (d *Document) GetOperation(id string) *Operation {
	if ref, ok := d.Operations[id]; ok && ref != nil && ref.Value != nil {
		return ref.Value
	}
	return nil
}

// GetMessage returns a message from components by name.
func (d *Document) GetMessage(name string) *Message {
	if d.Components == nil {
		return nil
	}
	if ref, ok := d.Components.Messages[name]; ok && ref != nil && ref.Value != nil {
		return ref.Value
	}
	return nil
}

// GetSchema returns a schema from components by name.
func (d *Document) GetSchema(name string) *Schema {
	if d.Components == nil {
		return nil
	}
	if ref, ok := d.Components.Schemas[name]; ok && ref != nil && ref.Value != nil {
		return ref.Value
	}
	return nil
}
