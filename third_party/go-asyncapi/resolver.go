package asyncapi

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RefResolver resolves $ref pointers within a document.
type RefResolver struct {
	doc       *Document
	loader    *Loader
	resolving map[string]bool      // cycle detection
	cache     map[string]*Document // external document cache
}

// NewRefResolver creates a new reference resolver for the given document.
func NewRefResolver(doc *Document) *RefResolver {
	return &RefResolver{
		doc:       doc,
		loader:    NewLoader(),
		resolving: make(map[string]bool),
		cache:     make(map[string]*Document),
	}
}

// WithLoader sets a custom loader for external references.
func (r *RefResolver) WithLoader(loader *Loader) *RefResolver {
	r.loader = loader
	return r
}

// ResolveRefs resolves all references in the document.
func (d *Document) ResolveRefs() error {
	resolver := NewRefResolver(d)
	return resolver.ResolveAll()
}

// ResolveAll resolves all references in the document.
func (r *RefResolver) ResolveAll() error {
	// Resolve servers
	for name, ref := range r.doc.Servers {
		if err := r.resolveServerRef(ref, "/servers/"+name); err != nil {
			return err
		}
	}

	// Resolve channels
	for name, ref := range r.doc.Channels {
		if err := r.resolveChannelRef(ref, "/channels/"+name); err != nil {
			return err
		}
	}

	// Resolve operations
	for name, ref := range r.doc.Operations {
		if err := r.resolveOperationRef(ref, "/operations/"+name); err != nil {
			return err
		}
	}

	// Resolve components
	if r.doc.Components != nil {
		if err := r.resolveComponents(); err != nil {
			return err
		}
	}

	return nil
}

func (r *RefResolver) resolveComponents() error {
	c := r.doc.Components

	for name, ref := range c.Schemas {
		if err := r.resolveSchemaRef(ref, "/components/schemas/"+name); err != nil {
			return err
		}
	}

	for name, ref := range c.Messages {
		if err := r.resolveMessageRef(ref, "/components/messages/"+name); err != nil {
			return err
		}
	}

	for name, ref := range c.Servers {
		if err := r.resolveServerRef(ref, "/components/servers/"+name); err != nil {
			return err
		}
	}

	for name, ref := range c.Channels {
		if err := r.resolveChannelRef(ref, "/components/channels/"+name); err != nil {
			return err
		}
	}

	for name, ref := range c.Operations {
		if err := r.resolveOperationRef(ref, "/components/operations/"+name); err != nil {
			return err
		}
	}

	return nil
}

func (r *RefResolver) resolveServerRef(ref *ServerRef, path string) error {
	if ref == nil {
		return nil
	}
	if ref.Ref == "" {
		return nil // Already resolved (inline)
	}

	// Check for cycle
	if r.resolving[ref.Ref] {
		return &RefError{Ref: ref.Ref, Message: "circular reference detected"}
	}
	r.resolving[ref.Ref] = true
	defer delete(r.resolving, ref.Ref)

	// Resolve the reference
	resolved, err := r.lookupServer(ref.Ref)
	if err != nil {
		return &RefError{Ref: ref.Ref, Message: "failed to resolve", Cause: err}
	}
	ref.Value = resolved
	return nil
}

func (r *RefResolver) resolveChannelRef(ref *ChannelRef, path string) error {
	if ref == nil {
		return nil
	}
	if ref.Ref == "" {
		// Inline - resolve nested refs
		if ref.Value != nil {
			for msgName, msgRef := range ref.Value.Messages {
				if err := r.resolveMessageRef(msgRef, path+"/messages/"+msgName); err != nil {
					return err
				}
			}
			for paramName, paramRef := range ref.Value.Parameters {
				if err := r.resolveParameterRef(paramRef, path+"/parameters/"+paramName); err != nil {
					return err
				}
			}
		}
		return nil
	}

	// Check for cycle
	if r.resolving[ref.Ref] {
		return &RefError{Ref: ref.Ref, Message: "circular reference detected"}
	}
	r.resolving[ref.Ref] = true
	defer delete(r.resolving, ref.Ref)

	// Resolve the reference
	resolved, err := r.lookupChannel(ref.Ref)
	if err != nil {
		return &RefError{Ref: ref.Ref, Message: "failed to resolve", Cause: err}
	}
	ref.Value = resolved
	return nil
}

func (r *RefResolver) resolveOperationRef(ref *OperationRef, path string) error {
	if ref == nil {
		return nil
	}
	if ref.Ref == "" {
		// Inline - resolve nested refs
		if ref.Value != nil {
			if err := r.resolveChannelRef(ref.Value.Channel, path+"/channel"); err != nil {
				return err
			}
			for i, msgRef := range ref.Value.Messages {
				if err := r.resolveMessageRef(msgRef, fmt.Sprintf("%s/messages/%d", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}

	// Check for cycle
	if r.resolving[ref.Ref] {
		return &RefError{Ref: ref.Ref, Message: "circular reference detected"}
	}
	r.resolving[ref.Ref] = true
	defer delete(r.resolving, ref.Ref)

	// Resolve the reference
	resolved, err := r.lookupOperation(ref.Ref)
	if err != nil {
		return &RefError{Ref: ref.Ref, Message: "failed to resolve", Cause: err}
	}
	ref.Value = resolved
	return nil
}

func (r *RefResolver) resolveMessageRef(ref *MessageRef, path string) error {
	if ref == nil {
		return nil
	}
	if ref.Ref == "" {
		// Inline - resolve nested refs
		if ref.Value != nil {
			if err := r.resolveMultiFormatSchemaRef(ref.Value.Payload, path+"/payload"); err != nil {
				return err
			}
			if err := r.resolveMultiFormatSchemaRef(ref.Value.Headers, path+"/headers"); err != nil {
				return err
			}
		}
		return nil
	}

	// Check for cycle
	if r.resolving[ref.Ref] {
		return &RefError{Ref: ref.Ref, Message: "circular reference detected"}
	}
	r.resolving[ref.Ref] = true
	defer delete(r.resolving, ref.Ref)

	// Resolve the reference
	resolved, err := r.lookupMessage(ref.Ref)
	if err != nil {
		return &RefError{Ref: ref.Ref, Message: "failed to resolve", Cause: err}
	}
	ref.Value = resolved
	return nil
}

func (r *RefResolver) resolveParameterRef(ref *ParameterRef, path string) error {
	if ref == nil || ref.Ref == "" {
		return nil
	}

	if r.resolving[ref.Ref] {
		return &RefError{Ref: ref.Ref, Message: "circular reference detected"}
	}
	r.resolving[ref.Ref] = true
	defer delete(r.resolving, ref.Ref)

	resolved, err := r.lookupParameter(ref.Ref)
	if err != nil {
		return &RefError{Ref: ref.Ref, Message: "failed to resolve", Cause: err}
	}
	ref.Value = resolved
	return nil
}

func (r *RefResolver) resolveSchemaRef(ref *SchemaRef, path string) error {
	if ref == nil || ref.Ref == "" {
		return nil
	}

	if r.resolving[ref.Ref] {
		return &RefError{Ref: ref.Ref, Message: "circular reference detected"}
	}
	r.resolving[ref.Ref] = true
	defer delete(r.resolving, ref.Ref)

	resolved, err := r.lookupSchema(ref.Ref)
	if err != nil {
		return &RefError{Ref: ref.Ref, Message: "failed to resolve", Cause: err}
	}
	ref.Value = resolved
	return nil
}

func (r *RefResolver) resolveMultiFormatSchemaRef(ref *MultiFormatSchemaRef, path string) error {
	if ref == nil || ref.Ref == "" {
		return nil
	}

	if r.resolving[ref.Ref] {
		return &RefError{Ref: ref.Ref, Message: "circular reference detected"}
	}
	r.resolving[ref.Ref] = true
	defer delete(r.resolving, ref.Ref)

	resolved, err := r.lookupSchema(ref.Ref)
	if err != nil {
		return &RefError{Ref: ref.Ref, Message: "failed to resolve", Cause: err}
	}
	ref.Value = resolved
	return nil
}

// Lookup functions for resolving references

func (r *RefResolver) lookupServer(ref string) (*Server, error) {
	name := extractRefName(ref, "servers")
	if name == "" {
		return nil, fmt.Errorf("invalid server reference: %s", ref)
	}

	// Check root servers
	if srv, ok := r.doc.Servers[name]; ok && srv.Value != nil {
		return srv.Value, nil
	}

	// Check components
	if r.doc.Components != nil {
		if srv, ok := r.doc.Components.Servers[name]; ok && srv.Value != nil {
			return srv.Value, nil
		}
	}

	return nil, fmt.Errorf("server not found: %s", name)
}

func (r *RefResolver) lookupChannel(ref string) (*Channel, error) {
	name := extractRefName(ref, "channels")
	if name == "" {
		return nil, fmt.Errorf("invalid channel reference: %s", ref)
	}

	// Check root channels
	if ch, ok := r.doc.Channels[name]; ok && ch.Value != nil {
		return ch.Value, nil
	}

	// Check components
	if r.doc.Components != nil {
		if ch, ok := r.doc.Components.Channels[name]; ok && ch.Value != nil {
			return ch.Value, nil
		}
	}

	return nil, fmt.Errorf("channel not found: %s", name)
}

func (r *RefResolver) lookupOperation(ref string) (*Operation, error) {
	name := extractRefName(ref, "operations")
	if name == "" {
		return nil, fmt.Errorf("invalid operation reference: %s", ref)
	}

	// Check root operations
	if op, ok := r.doc.Operations[name]; ok && op.Value != nil {
		return op.Value, nil
	}

	// Check components
	if r.doc.Components != nil {
		if op, ok := r.doc.Components.Operations[name]; ok && op.Value != nil {
			return op.Value, nil
		}
	}

	return nil, fmt.Errorf("operation not found: %s", name)
}

func (r *RefResolver) lookupMessage(ref string) (*Message, error) {
	// Messages can be in channels or components
	// Format: #/channels/{channel}/messages/{message} or #/components/messages/{message}

	if strings.Contains(ref, "/channels/") && strings.Contains(ref, "/messages/") {
		// Extract channel and message names
		parts := strings.Split(ref, "/")
		var channelName, messageName string
		for i, part := range parts {
			if part == "channels" && i+1 < len(parts) {
				channelName = parts[i+1]
			}
			if part == "messages" && i+1 < len(parts) {
				messageName = parts[i+1]
			}
		}
		if channelName != "" && messageName != "" {
			if ch, ok := r.doc.Channels[channelName]; ok && ch.Value != nil {
				if msg, ok := ch.Value.Messages[messageName]; ok && msg.Value != nil {
					return msg.Value, nil
				}
			}
		}
	}

	// Check components/messages
	name := extractRefName(ref, "messages")
	if name != "" && r.doc.Components != nil {
		if msg, ok := r.doc.Components.Messages[name]; ok && msg.Value != nil {
			return msg.Value, nil
		}
	}

	return nil, fmt.Errorf("message not found: %s", ref)
}

func (r *RefResolver) lookupParameter(ref string) (*Parameter, error) {
	name := extractRefName(ref, "parameters")
	if name == "" {
		return nil, fmt.Errorf("invalid parameter reference: %s", ref)
	}

	if r.doc.Components != nil {
		if param, ok := r.doc.Components.Parameters[name]; ok && param.Value != nil {
			return param.Value, nil
		}
	}

	return nil, fmt.Errorf("parameter not found: %s", name)
}

func (r *RefResolver) lookupSchema(ref string) (*Schema, error) {
	name := extractRefName(ref, "schemas")
	if name == "" {
		return nil, fmt.Errorf("invalid schema reference: %s", ref)
	}

	if r.doc.Components != nil {
		if schema, ok := r.doc.Components.Schemas[name]; ok && schema.Value != nil {
			return schema.Value, nil
		}
	}

	return nil, fmt.Errorf("schema not found: %s", name)
}

// extractRefName extracts the name from a reference like "#/components/schemas/MySchema"
// given the expected parent path component (e.g., "schemas").
func extractRefName(ref, parent string) string {
	// Handle external refs with fragments
	if idx := strings.Index(ref, "#"); idx > 0 {
		ref = ref[idx:]
	}

	if !strings.HasPrefix(ref, "#/") {
		return ""
	}

	parts := strings.Split(ref[2:], "/")
	for i, part := range parts {
		if part == parent && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// isExternalRef returns true if the reference points to an external file/URL.
func isExternalRef(ref string) bool {
	return !strings.HasPrefix(ref, "#") && (strings.Contains(ref, "/") || strings.Contains(ref, "."))
}

// splitExternalRef splits an external ref into file path and fragment.
// e.g., "./common/schemas.yaml#/components/schemas/User" -> ("./common/schemas.yaml", "#/components/schemas/User")
func splitExternalRef(ref string) (filePath, fragment string) {
	if idx := strings.Index(ref, "#"); idx >= 0 {
		return ref[:idx], ref[idx:]
	}
	return ref, ""
}

// loadExternalDocument loads an external document and caches it.
func (r *RefResolver) loadExternalDocument(path string) (*Document, error) {
	// Check cache
	if doc, ok := r.cache[path]; ok {
		return doc, nil
	}

	// Resolve relative paths
	basePath := r.loader.BasePath
	if basePath == "" {
		basePath = "."
	}

	fullPath := path
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		fullPath = filepath.Join(basePath, path)
	}

	// Load the document
	var doc *Document
	var err error

	if strings.HasPrefix(fullPath, "http://") || strings.HasPrefix(fullPath, "https://") {
		doc, err = r.loader.LoadFromURL(fullPath)
	} else {
		doc, err = r.loader.LoadFromFile(fullPath)
	}

	if err != nil {
		return nil, err
	}

	// Cache and return
	r.cache[path] = doc
	return doc, nil
}
