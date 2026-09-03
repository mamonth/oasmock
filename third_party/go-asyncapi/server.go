package asyncapi

// Server represents a message broker, server, or any computer program capable of sending/receiving data.
type Server struct {
	Host            string                        `json:"host" yaml:"host"`
	Protocol        string                        `json:"protocol" yaml:"protocol"`
	ProtocolVersion string                        `json:"protocolVersion,omitempty" yaml:"protocolVersion,omitempty"`
	Pathname        string                        `json:"pathname,omitempty" yaml:"pathname,omitempty"`
	Description     string                        `json:"description,omitempty" yaml:"description,omitempty"`
	Title           string                        `json:"title,omitempty" yaml:"title,omitempty"`
	Summary         string                        `json:"summary,omitempty" yaml:"summary,omitempty"`
	Variables       map[string]*ServerVariableRef `json:"variables,omitempty" yaml:"variables,omitempty"`
	Security        []*SecuritySchemeRef          `json:"security,omitempty" yaml:"security,omitempty"`
	Tags            []*TagRef                     `json:"tags,omitempty" yaml:"tags,omitempty"`
	ExternalDocs    *ExternalDocsRef              `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
	Bindings        *ServerBindingsRef            `json:"bindings,omitempty" yaml:"bindings,omitempty"`
}

// ServerVariable represents a variable for server URL template substitution.
type ServerVariable struct {
	Enum        []string `json:"enum,omitempty" yaml:"enum,omitempty"`
	Default     string   `json:"default,omitempty" yaml:"default,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Examples    []string `json:"examples,omitempty" yaml:"examples,omitempty"`
}
