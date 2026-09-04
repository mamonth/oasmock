package asyncapi

// Info provides metadata about the API.
type Info struct {
	Title          string           `json:"title" yaml:"title"`
	Version        string           `json:"version" yaml:"version"`
	Description    string           `json:"description,omitempty" yaml:"description,omitempty"`
	TermsOfService string           `json:"termsOfService,omitempty" yaml:"termsOfService,omitempty"`
	Contact        *Contact         `json:"contact,omitempty" yaml:"contact,omitempty"`
	License        *License         `json:"license,omitempty" yaml:"license,omitempty"`
	Tags           []*TagRef        `json:"tags,omitempty" yaml:"tags,omitempty"`
	ExternalDocs   *ExternalDocsRef `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
}

// Contact information for the exposed API.
type Contact struct {
	Name  string `json:"name,omitempty" yaml:"name,omitempty"`
	URL   string `json:"url,omitempty" yaml:"url,omitempty"`
	Email string `json:"email,omitempty" yaml:"email,omitempty"`
}

// License information for the exposed API.
type License struct {
	Name string `json:"name" yaml:"name"`
	URL  string `json:"url,omitempty" yaml:"url,omitempty"`
}

// Tag allows adding metadata to a single tag.
type Tag struct {
	Name         string           `json:"name" yaml:"name"`
	Description  string           `json:"description,omitempty" yaml:"description,omitempty"`
	ExternalDocs *ExternalDocsRef `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`
}

// ExternalDocs allows referencing an external resource for extended documentation.
type ExternalDocs struct {
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	URL         string `json:"url" yaml:"url"`
}
