package loader

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mamonth/oasmock/internal/asyncapi"
)

// Kind is the type of a loaded specification.
type Kind string

const (
	// KindOpenAPI is an OpenAPI 3.x specification.
	KindOpenAPI Kind = "openapi"
	// KindAsyncAPI is an AsyncAPI 3.x specification.
	KindAsyncAPI Kind = "asyncapi"
)

// SchemaInfo holds a loaded specification and its path prefix.
type SchemaInfo struct {
	Spec   *openapi3.T
	Kind   Kind
	Async  *asyncapi.Document
	Prefix string
}

// LoadSchemas loads multiple schemas (OpenAPI or AsyncAPI) from file paths.
// Each source path is paired with a prefix (empty string for no prefix).
// Returns a slice of SchemaInfo in the same order as sources.
func LoadSchemas(sources []string, prefixes []string) ([]SchemaInfo, error) {
	if len(sources) != len(prefixes) && len(prefixes) != 0 {
		return nil, fmt.Errorf("number of prefixes must match number of sources or be empty")
	}

	infos := make([]SchemaInfo, len(sources))
	for i, source := range sources {
		prefix := ""
		if i < len(prefixes) {
			prefix = prefixes[i]
		}

		info, err := loadSingleSchema(source)
		if err != nil {
			return nil, fmt.Errorf("failed to load schema %q: %w", source, err)
		}
		info.Prefix = prefix

		infos[i] = info
	}
	return infos, nil
}

// detectKind inspects raw spec bytes and dispatches on the root version key.
// Returns an error when the file is neither an OpenAPI nor an AsyncAPI spec.
func detectKind(data []byte) (Kind, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("empty schema file")
	}

	// Open the document as a generic mapping to find the root version key.
	doc, err := decodeDocument(trimmed)
	if err != nil {
		// Not parseable as YAML/JSON at all — not a valid spec file.
		return "", fmt.Errorf("file is not a valid OpenAPI or AsyncAPI schema: %w", err)
	}

	_, hasOpenAPI := doc["openapi"]
	_, hasAsyncAPI := doc["asyncapi"]
	switch {
	case hasOpenAPI:
		return KindOpenAPI, nil
	case hasAsyncAPI:
		return KindAsyncAPI, nil
	default:
		return "", fmt.Errorf("schema file has neither an 'openapi' nor an 'asyncapi' root key")
	}
}

func loadSingleSchema(path string) (SchemaInfo, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return SchemaInfo{}, fmt.Errorf("invalid path %q: %w", path, err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return SchemaInfo{}, fmt.Errorf("cannot read file %q: %w", path, err)
	}

	kind, err := detectKind(data)
	if err != nil {
		return SchemaInfo{}, fmt.Errorf("invalid schema %q: %w", path, err)
	}

	switch kind {
	case KindOpenAPI:
		spec, err := loadOpenAPI(absPath, data)
		if err != nil {
			return SchemaInfo{}, err
		}
		return SchemaInfo{Spec: spec, Kind: kind}, nil
	case KindAsyncAPI:
		doc, err := asyncapi.Parse(data)
		if err != nil {
			return SchemaInfo{}, fmt.Errorf("invalid AsyncAPI schema %q: %w", path, err)
		}
		return SchemaInfo{Async: doc, Kind: kind}, nil
	default:
		return SchemaInfo{}, fmt.Errorf("unsupported schema kind %q", kind)
	}
}

func loadOpenAPI(absPath string, data []byte) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAPI schema %q: %w", absPath, err)
	}

	// Validate the spec
	if err := spec.Validate(loader.Context); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI schema %q: %w", absPath, err)
	}

	return spec, nil
}

// decodeDocument parses YAML or JSON bytes into a generic mapping so the
// root version key can be inspected before choosing a loader.
func decodeDocument(data []byte) (map[string]any, error) {
	if !strings.HasPrefix(strings.TrimLeft(string(data), " \t\r\n"), "{") {
		return decodeYAMLDocument(data)
	}
	return decodeJSONDocument(data)
}
