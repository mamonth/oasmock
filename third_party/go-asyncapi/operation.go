package asyncapi

// Action is the operation action type.
type Action string

const (
	// ActionSend indicates the application sends messages to the channel.
	ActionSend Action = "send"
	// ActionReceive indicates the application receives messages from the channel.
	ActionReceive Action = "receive"
)

// Operation describes a specific operation.
type Operation struct {
	Action       Action                `json:"action" yaml:"action"`
	Channel      *ChannelRef           `json:"channel" yaml:"channel"`
	Title        string                `json:"title,omitempty" yaml:"title,omitempty"`
	Summary      string                `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description  string                `json:"description,omitempty" yaml:"description,omitempty"`
	Security     []*SecuritySchemeRef  `json:"security,omitempty" yaml:"security,omitempty"`
	Tags         []*TagRef             `json:"tags,omitempty" yaml:"tags,omitempty"`
	ExternalDocs *ExternalDocsRef      `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	Bindings     *OperationBindingsRef `json:"bindings,omitempty" yaml:"bindings,omitempty"`
	Traits       []*OperationTraitRef  `json:"traits,omitempty" yaml:"traits,omitempty"`
	Messages     []*MessageRef         `json:"messages,omitempty" yaml:"messages,omitempty"`
	Reply        *OperationReplyRef    `json:"reply,omitempty" yaml:"reply,omitempty"`
}

// IsSender returns true if this operation sends messages.
func (o *Operation) IsSender() bool { return o.Action == ActionSend }

// IsReceiver returns true if this operation receives messages.
func (o *Operation) IsReceiver() bool { return o.Action == ActionReceive }

// OperationTrait describes a trait that may be applied to an Operation.
type OperationTrait struct {
	Title        string                `json:"title,omitempty" yaml:"title,omitempty"`
	Summary      string                `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description  string                `json:"description,omitempty" yaml:"description,omitempty"`
	Security     []*SecuritySchemeRef  `json:"security,omitempty" yaml:"security,omitempty"`
	Tags         []*TagRef             `json:"tags,omitempty" yaml:"tags,omitempty"`
	ExternalDocs *ExternalDocsRef      `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	Bindings     *OperationBindingsRef `json:"bindings,omitempty" yaml:"bindings,omitempty"`
}

// OperationReply describes the reply part of a request/reply operation.
type OperationReply struct {
	Address  *ReplyAddressRef `json:"address,omitempty" yaml:"address,omitempty"`
	Channel  *ChannelRef      `json:"channel,omitempty" yaml:"channel,omitempty"`
	Messages []*MessageRef    `json:"messages,omitempty" yaml:"messages,omitempty"`
}

// ReplyAddress specifies where an operation sends the reply.
type ReplyAddress struct {
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Location    string `json:"location" yaml:"location"` // runtime expression
}
