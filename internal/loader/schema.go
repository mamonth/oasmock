package loader

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi3"
)

// SchemaInfo holds a loaded OpenAPI spec and its path prefix.
type SchemaInfo struct {
	Spec   *openapi3.T
	Prefix string
}

// LoadSchemas loads multiple OpenAPI schemas from file paths.
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

		spec, err := loadSingleSchema(source)
		if err != nil {
			return nil, fmt.Errorf("failed to load schema %q: %w", source, err)
		}

		infos[i] = SchemaInfo{
			Spec:   spec,
			Prefix: prefix,
		}
	}
	return infos, nil
}

func loadSingleSchema(path string) (*openapi3.T, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", path, err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read file %q: %w", path, err)
	}

	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAPI schema %q: %w", path, err)
	}

	// Validate the spec
	if err := spec.Validate(loader.Context); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI schema %q: %w", path, err)
	}

	return spec, nil
}
