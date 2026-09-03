package asyncapi

// MergeTraits applies traits to operations and messages using RFC 7386 JSON Merge Patch.
// Traits are merged in order, and explicit values on the target object take precedence.
func (d *Document) MergeTraits() error {
	// Merge operation traits
	for _, opRef := range d.Operations {
		if opRef == nil || opRef.Value == nil || len(opRef.Value.Traits) == 0 {
			continue
		}
		if err := mergeOperationTraits(opRef.Value); err != nil {
			return err
		}
	}

	// Merge message traits in channels
	for _, chRef := range d.Channels {
		if chRef == nil || chRef.Value == nil {
			continue
		}
		for _, msgRef := range chRef.Value.Messages {
			if msgRef == nil || msgRef.Value == nil || len(msgRef.Value.Traits) == 0 {
				continue
			}
			if err := mergeMessageTraits(msgRef.Value); err != nil {
				return err
			}
		}
	}

	// Merge message traits in components
	if d.Components != nil {
		for _, msgRef := range d.Components.Messages {
			if msgRef == nil || msgRef.Value == nil || len(msgRef.Value.Traits) == 0 {
				continue
			}
			if err := mergeMessageTraits(msgRef.Value); err != nil {
				return err
			}
		}
	}

	return nil
}

// mergeOperationTraits merges traits into an operation.
// Per RFC 7386, values in the target take precedence over trait values.
func mergeOperationTraits(op *Operation) error {
	for _, traitRef := range op.Traits {
		if traitRef == nil || traitRef.Value == nil {
			continue
		}
		trait := traitRef.Value

		// Merge each field only if the operation doesn't have it set
		if op.Title == "" && trait.Title != "" {
			op.Title = trait.Title
		}
		if op.Summary == "" && trait.Summary != "" {
			op.Summary = trait.Summary
		}
		if op.Description == "" && trait.Description != "" {
			op.Description = trait.Description
		}
		if op.Security == nil && trait.Security != nil {
			op.Security = trait.Security
		}
		if op.Tags == nil && trait.Tags != nil {
			op.Tags = trait.Tags
		}
		if op.ExternalDocs == nil && trait.ExternalDocs != nil {
			op.ExternalDocs = trait.ExternalDocs
		}
		if op.Bindings == nil && trait.Bindings != nil {
			op.Bindings = trait.Bindings
		}
	}
	return nil
}

// mergeMessageTraits merges traits into a message.
// Per RFC 7386, values in the target take precedence over trait values.
func mergeMessageTraits(msg *Message) error {
	for _, traitRef := range msg.Traits {
		if traitRef == nil || traitRef.Value == nil {
			continue
		}
		trait := traitRef.Value

		// Merge each field only if the message doesn't have it set
		if msg.Headers == nil && trait.Headers != nil {
			msg.Headers = trait.Headers
		}
		if msg.CorrelationID == nil && trait.CorrelationID != nil {
			msg.CorrelationID = trait.CorrelationID
		}
		if msg.ContentType == "" && trait.ContentType != "" {
			msg.ContentType = trait.ContentType
		}
		if msg.Name == "" && trait.Name != "" {
			msg.Name = trait.Name
		}
		if msg.Title == "" && trait.Title != "" {
			msg.Title = trait.Title
		}
		if msg.Summary == "" && trait.Summary != "" {
			msg.Summary = trait.Summary
		}
		if msg.Description == "" && trait.Description != "" {
			msg.Description = trait.Description
		}
		if msg.Tags == nil && trait.Tags != nil {
			msg.Tags = trait.Tags
		}
		if msg.ExternalDocs == nil && trait.ExternalDocs != nil {
			msg.ExternalDocs = trait.ExternalDocs
		}
		if msg.Bindings == nil && trait.Bindings != nil {
			msg.Bindings = trait.Bindings
		}
		if msg.Examples == nil && trait.Examples != nil {
			msg.Examples = trait.Examples
		}
	}
	return nil
}
