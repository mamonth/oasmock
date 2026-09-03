package asyncapi

// Visitor defines callbacks for walking an AsyncAPI document.
// Return false from any callback to stop walking.
type Visitor struct {
	// VisitServer is called for each server.
	VisitServer func(name string, server *Server) bool

	// VisitChannel is called for each channel.
	VisitChannel func(name string, channel *Channel) bool

	// VisitOperation is called for each operation.
	VisitOperation func(name string, operation *Operation) bool

	// VisitMessage is called for each message (from channels and components).
	VisitMessage func(name string, message *Message) bool

	// VisitSchema is called for each schema in components.
	VisitSchema func(name string, schema *Schema) bool

	// VisitSecurityScheme is called for each security scheme.
	VisitSecurityScheme func(name string, scheme *SecurityScheme) bool
}

// Walk traverses the document and calls visitor callbacks.
// References should be resolved before walking for full access.
func (d *Document) Walk(v *Visitor) {
	if v == nil {
		return
	}

	// Walk servers
	if v.VisitServer != nil {
		for name, ref := range d.Servers {
			if ref != nil && ref.Value != nil {
				if !v.VisitServer(name, ref.Value) {
					return
				}
			}
		}
	}

	// Walk channels
	if v.VisitChannel != nil {
		for name, ref := range d.Channels {
			if ref != nil && ref.Value != nil {
				if !v.VisitChannel(name, ref.Value) {
					return
				}
			}
		}
	}

	// Walk operations
	if v.VisitOperation != nil {
		for name, ref := range d.Operations {
			if ref != nil && ref.Value != nil {
				if !v.VisitOperation(name, ref.Value) {
					return
				}
			}
		}
	}

	// Walk messages (from channels)
	if v.VisitMessage != nil {
		seen := make(map[string]bool)
		for _, chRef := range d.Channels {
			if chRef == nil || chRef.Value == nil {
				continue
			}
			for name, msgRef := range chRef.Value.Messages {
				if msgRef != nil && msgRef.Value != nil && !seen[name] {
					seen[name] = true
					if !v.VisitMessage(name, msgRef.Value) {
						return
					}
				}
			}
		}
		// Also from components
		if d.Components != nil {
			for name, msgRef := range d.Components.Messages {
				if msgRef != nil && msgRef.Value != nil && !seen[name] {
					seen[name] = true
					if !v.VisitMessage(name, msgRef.Value) {
						return
					}
				}
			}
		}
	}

	// Walk schemas
	if v.VisitSchema != nil && d.Components != nil {
		for name, ref := range d.Components.Schemas {
			if ref != nil && ref.Value != nil {
				if !v.VisitSchema(name, ref.Value) {
					return
				}
			}
		}
	}

	// Walk security schemes
	if v.VisitSecurityScheme != nil && d.Components != nil {
		for name, ref := range d.Components.SecuritySchemes {
			if ref != nil && ref.Value != nil {
				if !v.VisitSecurityScheme(name, ref.Value) {
					return
				}
			}
		}
	}
}

// AllServers returns all servers as a slice.
func (d *Document) AllServers() []*Server {
	var servers []*Server
	for _, ref := range d.Servers {
		if ref != nil && ref.Value != nil {
			servers = append(servers, ref.Value)
		}
	}
	return servers
}

// AllChannels returns all channels as a slice.
func (d *Document) AllChannels() []*Channel {
	var channels []*Channel
	for _, ref := range d.Channels {
		if ref != nil && ref.Value != nil {
			channels = append(channels, ref.Value)
		}
	}
	return channels
}

// AllOperations returns all operations as a slice.
func (d *Document) AllOperations() []*Operation {
	var ops []*Operation
	for _, ref := range d.Operations {
		if ref != nil && ref.Value != nil {
			ops = append(ops, ref.Value)
		}
	}
	return ops
}

// AllMessages returns all unique messages from channels and components.
func (d *Document) AllMessages() []*Message {
	seen := make(map[*Message]bool)
	var messages []*Message

	for _, chRef := range d.Channels {
		if chRef == nil || chRef.Value == nil {
			continue
		}
		for _, msgRef := range chRef.Value.Messages {
			if msgRef != nil && msgRef.Value != nil && !seen[msgRef.Value] {
				seen[msgRef.Value] = true
				messages = append(messages, msgRef.Value)
			}
		}
	}

	if d.Components != nil {
		for _, msgRef := range d.Components.Messages {
			if msgRef != nil && msgRef.Value != nil && !seen[msgRef.Value] {
				seen[msgRef.Value] = true
				messages = append(messages, msgRef.Value)
			}
		}
	}

	return messages
}

// AllSchemas returns all schemas from components.
func (d *Document) AllSchemas() map[string]*Schema {
	schemas := make(map[string]*Schema)
	if d.Components != nil {
		for name, ref := range d.Components.Schemas {
			if ref != nil && ref.Value != nil {
				schemas[name] = ref.Value
			}
		}
	}
	return schemas
}

// OperationsByAction returns operations grouped by action (send/receive).
func (d *Document) OperationsByAction() (send, receive []*Operation) {
	for _, ref := range d.Operations {
		if ref == nil || ref.Value == nil {
			continue
		}
		switch ref.Value.Action {
		case ActionSend:
			send = append(send, ref.Value)
		case ActionReceive:
			receive = append(receive, ref.Value)
		}
	}
	return
}

// ChannelOperations returns all operations for a given channel.
func (d *Document) ChannelOperations(channelName string) []*Operation {
	var ops []*Operation
	for _, ref := range d.Operations {
		if ref == nil || ref.Value == nil || ref.Value.Channel == nil {
			continue
		}
		// Check if this operation references the channel
		if ref.Value.Channel.Ref == "#/channels/"+channelName {
			ops = append(ops, ref.Value)
		} else if ref.Value.Channel.Value != nil {
			// Check by resolved value (address comparison)
			if ch, ok := d.Channels[channelName]; ok && ch != nil && ch.Value == ref.Value.Channel.Value {
				ops = append(ops, ref.Value)
			}
		}
	}
	return ops
}

// MessagePayloadSchema returns the payload schema for a message, if any.
func (m *Message) PayloadSchema() *Schema {
	if m.Payload != nil && m.Payload.Value != nil {
		return m.Payload.Value
	}
	return nil
}

// MessageHeaderSchema returns the headers schema for a message, if any.
func (m *Message) HeaderSchema() *Schema {
	if m.Headers != nil && m.Headers.Value != nil {
		return m.Headers.Value
	}
	return nil
}

// SchemaProperties returns all properties of an object schema.
func (s *Schema) SchemaProperties() map[string]*Schema {
	props := make(map[string]*Schema)
	for name, ref := range s.Properties {
		if ref != nil && ref.Value != nil {
			props[name] = ref.Value
		}
	}
	return props
}

// IsObject returns true if this schema represents an object type.
func (s *Schema) IsObject() bool {
	return s.Type.Is("object") || len(s.Properties) > 0
}

// IsArray returns true if this schema represents an array type.
func (s *Schema) IsArray() bool {
	return s.Type.Is("array")
}

// IsString returns true if this schema represents a string type.
func (s *Schema) IsString() bool {
	return s.Type.Is("string")
}

// IsNumber returns true if this schema represents a number type.
func (s *Schema) IsNumber() bool {
	return s.Type.Is("number") || s.Type.Is("integer")
}

// IsBoolean returns true if this schema represents a boolean type.
func (s *Schema) IsBoolean() bool {
	return s.Type.Is("boolean")
}

// ItemSchema returns the schema for array items, if this is an array schema.
func (s *Schema) ItemSchema() *Schema {
	if s.Items != nil && s.Items.Value != nil {
		return s.Items.Value
	}
	return nil
}
