package asyncapi

// Channel describes a shared communication channel.
type Channel struct {
	Address      *string                  `json:"address" yaml:"address"` // nullable per spec
	Messages     map[string]*MessageRef   `json:"messages,omitempty" yaml:"messages,omitempty"`
	Title        string                   `json:"title,omitempty" yaml:"title,omitempty"`
	Summary      string                   `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description  string                   `json:"description,omitempty" yaml:"description,omitempty"`
	Servers      []*ServerRef             `json:"servers,omitempty" yaml:"servers,omitempty"`
	Parameters   map[string]*ParameterRef `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Tags         []*TagRef                `json:"tags,omitempty" yaml:"tags,omitempty"`
	ExternalDocs *ExternalDocsRef         `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	Bindings     *ChannelBindingsRef      `json:"bindings,omitempty" yaml:"bindings,omitempty"`
}

// Parameter describes a parameter included in a channel address.
type Parameter struct {
	Enum        []string `json:"enum,omitempty" yaml:"enum,omitempty"`
	Default     string   `json:"default,omitempty" yaml:"default,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Examples    []string `json:"examples,omitempty" yaml:"examples,omitempty"`
	Location    string   `json:"location,omitempty" yaml:"location,omitempty"` // runtime expression
}
