package asyncapi

// Components holds reusable objects for different aspects of the AsyncAPI specification.
type Components struct {
	Schemas           map[string]*SchemaRef            `json:"schemas,omitempty" yaml:"schemas,omitempty"`
	Servers           map[string]*ServerRef            `json:"servers,omitempty" yaml:"servers,omitempty"`
	Channels          map[string]*ChannelRef           `json:"channels,omitempty" yaml:"channels,omitempty"`
	Operations        map[string]*OperationRef         `json:"operations,omitempty" yaml:"operations,omitempty"`
	Messages          map[string]*MessageRef           `json:"messages,omitempty" yaml:"messages,omitempty"`
	SecuritySchemes   map[string]*SecuritySchemeRef    `json:"securitySchemes,omitempty" yaml:"securitySchemes,omitempty"`
	ServerVariables   map[string]*ServerVariableRef    `json:"serverVariables,omitempty" yaml:"serverVariables,omitempty"`
	Parameters        map[string]*ParameterRef         `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	CorrelationIDs    map[string]*CorrelationIDRef     `json:"correlationIds,omitempty" yaml:"correlationIds,omitempty"`
	Replies           map[string]*OperationReplyRef    `json:"replies,omitempty" yaml:"replies,omitempty"`
	ReplyAddresses    map[string]*ReplyAddressRef      `json:"replyAddresses,omitempty" yaml:"replyAddresses,omitempty"`
	ExternalDocs      map[string]*ExternalDocsRef      `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	Tags              map[string]*TagRef               `json:"tags,omitempty" yaml:"tags,omitempty"`
	OperationTraits   map[string]*OperationTraitRef    `json:"operationTraits,omitempty" yaml:"operationTraits,omitempty"`
	MessageTraits     map[string]*MessageTraitRef      `json:"messageTraits,omitempty" yaml:"messageTraits,omitempty"`
	ServerBindings    map[string]*ServerBindingsRef    `json:"serverBindings,omitempty" yaml:"serverBindings,omitempty"`
	ChannelBindings   map[string]*ChannelBindingsRef   `json:"channelBindings,omitempty" yaml:"channelBindings,omitempty"`
	OperationBindings map[string]*OperationBindingsRef `json:"operationBindings,omitempty" yaml:"operationBindings,omitempty"`
	MessageBindings   map[string]*MessageBindingsRef   `json:"messageBindings,omitempty" yaml:"messageBindings,omitempty"`
}
