package asyncapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader loads AsyncAPI documents from various sources.
type Loader struct {
	// ReadFile reads a file from the filesystem.
	// If nil, os.ReadFile is used.
	ReadFile func(path string) ([]byte, error)

	// ReadURL fetches content from a URL.
	// If nil, http.Get is used.
	ReadURL func(url string) ([]byte, error)

	// BasePath is the base path for resolving relative file references.
	BasePath string
}

// NewLoader creates a new Loader with default settings.
func NewLoader() *Loader {
	return &Loader{
		ReadFile: os.ReadFile,
		ReadURL:  defaultReadURL,
	}
}

func defaultReadURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// LoadFromFile loads an AsyncAPI document from a file.
func LoadFromFile(path string) (*Document, error) {
	loader := NewLoader()
	loader.BasePath = filepath.Dir(path)
	return loader.LoadFromFile(path)
}

// LoadFromData loads an AsyncAPI document from bytes.
func LoadFromData(data []byte) (*Document, error) {
	loader := NewLoader()
	return loader.LoadFromData(data)
}

// LoadFromFile loads an AsyncAPI document from a file.
func (l *Loader) LoadFromFile(path string) (*Document, error) {
	readFile := l.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}

	data, err := readFile(path)
	if err != nil {
		return nil, &ParseError{Message: "failed to read file", Cause: err}
	}

	if l.BasePath == "" {
		l.BasePath = filepath.Dir(path)
	}

	return l.LoadFromData(data)
}

// LoadFromURL loads an AsyncAPI document from a URL.
func (l *Loader) LoadFromURL(url string) (*Document, error) {
	readURL := l.ReadURL
	if readURL == nil {
		readURL = defaultReadURL
	}

	data, err := readURL(url)
	if err != nil {
		return nil, &ParseError{Message: "failed to fetch URL", Cause: err}
	}

	return l.LoadFromData(data)
}

// LoadFromData loads an AsyncAPI document from bytes.
func (l *Loader) LoadFromData(data []byte) (*Document, error) {
	doc := &Document{
		raw: data,
	}

	// Detect format and parse
	if isJSON(data) {
		if err := json.Unmarshal(data, doc); err != nil {
			return nil, &ParseError{Message: "failed to parse JSON", Cause: err}
		}
	} else {
		if err := yaml.Unmarshal(data, doc); err != nil {
			return nil, &ParseError{Message: "failed to parse YAML", Cause: err}
		}
	}

	// Validate version
	if !strings.HasPrefix(doc.AsyncAPI, "3.") {
		return nil, &ParseError{Message: "unsupported AsyncAPI version: " + doc.AsyncAPI}
	}

	return doc, nil
}

// isJSON returns true if data looks like JSON.
func isJSON(data []byte) bool {
	data = bytes.TrimSpace(data)
	return len(data) > 0 && (data[0] == '{' || data[0] == '[')
}
