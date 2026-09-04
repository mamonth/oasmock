package asyncapi

// SecuritySchemeType is the type of security scheme.
type SecuritySchemeType string

const (
	SecurityTypeUserPassword      SecuritySchemeType = "userPassword"
	SecurityTypeAPIKey            SecuritySchemeType = "apiKey"
	SecurityTypeX509              SecuritySchemeType = "X509"
	SecurityTypeSymmetricEncrypt  SecuritySchemeType = "symmetricEncryption"
	SecurityTypeAsymmetricEncrypt SecuritySchemeType = "asymmetricEncryption"
	SecurityTypeHTTPAPIKey        SecuritySchemeType = "httpApiKey"
	SecurityTypeHTTP              SecuritySchemeType = "http"
	SecurityTypeOAuth2            SecuritySchemeType = "oauth2"
	SecurityTypeOpenIDConnect     SecuritySchemeType = "openIdConnect"
	SecurityTypePlain             SecuritySchemeType = "plain"
	SecurityTypeScramSHA256       SecuritySchemeType = "scramSha256"
	SecurityTypeScramSHA512       SecuritySchemeType = "scramSha512"
	SecurityTypeGSSAPI            SecuritySchemeType = "gssapi"
)

// SecurityScheme defines a security scheme that can be used by operations.
type SecurityScheme struct {
	Type             SecuritySchemeType `json:"type" yaml:"type"`
	Description      string             `json:"description,omitempty" yaml:"description,omitempty"`
	Name             string             `json:"name,omitempty" yaml:"name,omitempty"`     // httpApiKey
	In               string             `json:"in,omitempty" yaml:"in,omitempty"`         // apiKey, httpApiKey
	Scheme           string             `json:"scheme,omitempty" yaml:"scheme,omitempty"` // http
	BearerFormat     string             `json:"bearerFormat,omitempty" yaml:"bearerFormat,omitempty"`
	Flows            *OAuthFlows        `json:"flows,omitempty" yaml:"flows,omitempty"`
	OpenIDConnectURL string             `json:"openIdConnectUrl,omitempty" yaml:"openIdConnectUrl,omitempty"`
}

// OAuthFlows allows configuration of the supported OAuth Flows.
type OAuthFlows struct {
	Implicit          *OAuthFlow `json:"implicit,omitempty" yaml:"implicit,omitempty"`
	Password          *OAuthFlow `json:"password,omitempty" yaml:"password,omitempty"`
	ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty" yaml:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty" yaml:"authorizationCode,omitempty"`
}

// OAuthFlow contains configuration details for a supported OAuth Flow.
type OAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty" yaml:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty" yaml:"tokenUrl,omitempty"`
	RefreshURL       string            `json:"refreshUrl,omitempty" yaml:"refreshUrl,omitempty"`
	AvailableScopes  map[string]string `json:"availableScopes,omitempty" yaml:"availableScopes,omitempty"`
}
