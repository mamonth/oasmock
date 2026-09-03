package asyncapi

import (
	"fmt"
	"sort"
	"strings"

	benelser "github.com/benelser/go-asyncapi"
	"gopkg.in/yaml.v3"
)

// Parse parses AsyncAPI 3.x YAML/JSON data into a neutral Document view.
// It loads and resolves references through the backing parser, applies
// structural validation (version, mandatory fields, supported protocols),
// and maps the result to the protocol-neutral model.
func Parse(data []byte) (*Document, error) {
	raw, err := benelser.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("invalid AsyncAPI schema: %w", err)
	}

	if err := raw.ResolveRefs(); err != nil {
		return nil, fmt.Errorf("invalid AsyncAPI schema: %w", err)
	}

	if err := validate(raw); err != nil {
		return nil, err
	}

	doc, err := mapDocument(raw)
	if err != nil {
		return nil, err
	}

	captureSignalR(doc, data)
	return doc, nil
}

// validate performs structural validation against spec requirements.
func validate(raw *benelser.Document) error {
	version := raw.AsyncAPI
	if version == "" {
		return fmt.Errorf("invalid AsyncAPI schema: missing asyncapi version")
	}
	if !strings.HasPrefix(version, "3.") {
		return fmt.Errorf("invalid AsyncAPI schema: unsupported AsyncAPI version %q (only 3.x is supported)", version)
	}

	// All 3.x documents require at least one channel.
	if len(raw.Channels) == 0 {
		return fmt.Errorf("invalid AsyncAPI schema: missing mandatory channels")
	}

	// 3.0.0 requires operations; 3.1.0 allows operations to be replaced by webhooks.
	if version == "3.0.0" && len(raw.Operations) == 0 {
		return fmt.Errorf("invalid AsyncAPI schema: missing mandatory operations")
	}

	return validateProtocols(raw)
}

// supportedProtocols is the set of protocol bindings the MVP can serve.
// amqp (and any other protocol) is treated as unsupported for now.
var supportedProtocols = map[string]bool{
	ProtocolHTTP: true,
	ProtocolWS:   true,
}

// validateProtocols reports an error when a channel only declares protocol
// bindings that the mock server cannot serve, naming the offending protocol.
func validateProtocols(raw *benelser.Document) error {
	for chID, chRef := range raw.Channels {
		ch := chRef.Value
		if ch == nil {
			continue
		}
		prots := channelProtocols(ch)
		if len(prots) == 0 {
			continue
		}
		supported := false
		for _, p := range prots {
			if supportedProtocols[p] {
				supported = true
				break
			}
		}
		if !supported {
			return fmt.Errorf("invalid AsyncAPI schema: channel %q declares unsupported protocol binding(s): %s",
				chID, strings.Join(prots, ", "))
		}
	}
	return nil
}

// channelProtocols returns the protocol binding names declared on a channel.
func channelProtocols(ch *benelser.Channel) []string {
	if ch == nil || ch.Bindings == nil {
		return nil
	}
	return iterBindings(ch.Bindings.Value)
}

// iterBindings extracts the declared protocol names from channel bindings.
func iterBindings(b *benelser.ChannelBindings) []string {
	if b == nil {
		return nil
	}
	var out []string
	if b.HTTP != nil {
		out = append(out, ProtocolHTTP)
	}
	if b.WS != nil {
		out = append(out, ProtocolWS)
	}
	if b.AMQP != nil {
		out = append(out, "amqp")
	}
	if b.Kafka != nil {
		out = append(out, "kafka")
	}
	if b.MQTT != nil {
		out = append(out, "mqtt")
	}
	if b.NATS != nil {
		out = append(out, "nats")
	}
	for k := range b.Raw {
		out = append(out, k)
	}
	return out
}

// captureSignalR surfaces the root-level x-signalr extension on the document.
// The backing parser's Document keeps no root extension map populated, so the
// raw document is decoded and the extension looked up directly.
func captureSignalR(doc *Document, data []byte) {
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return
	}
	v, ok := m[signalRKey]
	if !ok {
		return
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return
	}
	doc.SignalR = &SignalRConfig{Raw: raw}
	if p, ok := raw["path"].(string); ok {
		doc.SignalR.Path = p
	}
}

// signalRKey is the root-level x-signalr extension name.
const signalRKey = "x-signalr"

func mapDocument(raw *benelser.Document) (*Document, error) {
	doc := &Document{
		Version: raw.AsyncAPI,
	}

	// Map channels deterministically, recording pointer identity so that
	// operations can be linked back to the same channel instances.
	channelByID := make(map[string]*Channel)
	channelByPtr := make(map[*benelser.Channel]*Channel)
	for _, id := range sortedKeys(raw.Channels) {
		ch, err := mapChannel(id, raw.Channels[id])
		if err != nil {
			return nil, err
		}
		doc.Channels = append(doc.Channels, ch)
		channelByID[id] = ch
		if ref := raw.Channels[id]; ref != nil && ref.Value != nil {
			channelByPtr[ref.Value] = ch
		}
	}

	// Map operations; an operation references its channel through a resolved
	// *Channel node, matched by pointer identity against root channels.
	for _, id := range sortedKeys(raw.Operations) {
		opRef := raw.Operations[id]
		op := opRef.Value
		if op == nil {
			continue
		}
		out := &Operation{
			ID:     id,
			Action: Action(op.Action),
		}
		if op.Channel != nil && op.Channel.Value != nil {
			if ch, ok := channelByPtr[op.Channel.Value]; ok {
				out.Channel = ch
			} else {
				// Inline channel declared only at the operation; map it directly.
				if mapped, merr := mapChannel(id+"-channel", op.Channel); merr == nil {
					out.Channel = mapped
				}
			}
		}
		out.Messages = messageRefs(op)
		out.Bindings = mapOperationBindings(op.Bindings)
		doc.Operations = append(doc.Operations, out)
	}

	return doc, nil
}

// messageRefs maps the messages of an operation; when the operation declares
// none it falls back to the referenced channel's messages.
func messageRefs(op *benelser.Operation) []*Message {
	var refs []*benelser.MessageRef
	refs = append(refs, op.Messages...)
	if len(refs) == 0 && op.Channel != nil && op.Channel.Value != nil {
		for _, id := range sortedKeys(op.Channel.Value.Messages) {
			refs = append(refs, op.Channel.Value.Messages[id])
		}
	}
	// Names for channel-resident messages come from the channel's map keys.
	namesByPtr := make(map[*benelser.Message]string)
	if op.Channel != nil && op.Channel.Value != nil {
		for id, ref := range op.Channel.Value.Messages {
			if ref != nil && ref.Value != nil {
				namesByPtr[ref.Value] = id
			}
		}
	}
	var out []*Message
	for _, ref := range refs {
		m := ref.Value
		if m == nil {
			continue
		}
		out = append(out, mapMessage(m, namesByPtr[m]))
	}
	return out
}

func mapMessage(m *benelser.Message, nameHint string) *Message {
	msg := &Message{
		Name:        m.Name,
		ContentType: m.ContentType,
	}
	switch {
	case m.Title != "":
		msg.Name = m.Title
	case msg.Name == "" && nameHint != "":
		msg.Name = nameHint
	}
	for _, ex := range m.Examples {
		if ex == nil {
			continue
		}
		msg.Examples = append(msg.Examples, &Example{
			Name:       ex.Name,
			Headers:    ex.Headers,
			Payload:    ex.Payload,
			Extensions: ex.Extensions(),
		})
	}
	return msg
}

func mapChannel(id string, chRef *benelser.ChannelRef) (*Channel, error) {
	ch := chRef.Value
	if ch == nil {
		return nil, fmt.Errorf("invalid AsyncAPI schema: channel %q has no definition", id)
	}
	out := &Channel{
		ID:    id,
		Title: ch.Title,
	}
	if ch.Address != nil {
		out.Address = *ch.Address
	}
	for _, pid := range sortedKeys(ch.Parameters) {
		p := ch.Parameters[pid]
		if p == nil {
			continue
		}
		out.Parameters = append(out.Parameters, &Parameter{Name: pid})
	}
	for _, mid := range sortedKeys(ch.Messages) {
		m := ch.Messages[mid].Value
		if m == nil {
			continue
		}
		out.Messages = append(out.Messages, mapMessage(m, mid))
	}
	out.Bindings = mapChannelBindings(ch.Bindings)
	return out, nil
}

// mapChannelBindings extracts binding details from channel bindings.
func mapChannelBindings(bRef *benelser.ChannelBindingsRef) Bindings {
	if bRef == nil || bRef.Value == nil {
		return Bindings{}
	}
	b := bRef.Value
	out := Bindings{Protocols: iterBindings(b)}
	if b.HTTP != nil {
		out.HTTP = &HTTPBinding{}
	}
	if b.WS != nil {
		out.WS = &WSBinding{Method: b.WS.Method}
	}
	return out
}

// mapOperationBindings extracts binding details from operation bindings.
func mapOperationBindings(bRef *benelser.OperationBindingsRef) Bindings {
	if bRef == nil || bRef.Value == nil {
		return Bindings{}
	}
	b := bRef.Value
	out := Bindings{}
	if b.HTTP != nil {
		out.HTTP = &HTTPBinding{Method: strings.ToUpper(b.HTTP.Method)}
		out.Protocols = append(out.Protocols, ProtocolHTTP)
	}
	if b.WS != nil {
		out.WS = &WSBinding{}
		out.Protocols = append(out.Protocols, ProtocolWS)
	}
	return out
}

// sortedKeys returns map keys in ascending order for deterministic iteration.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
