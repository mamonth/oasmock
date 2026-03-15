package runtime

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockDataSourceFromMap creates a MockDataSource that returns values from a map.
func newMockDataSourceFromMap(ctrl *gomock.Controller, data map[string]any) *MockDataSource {
	mock := NewMockDataSource(ctrl)
	for path, val := range data {
		mock.EXPECT().Get(path).Return(val, true).AnyTimes()
	}
	// Default expectation for any other path
	mock.EXPECT().Get(gomock.Any()).Return(nil, false).AnyTimes()
	return mock
}

/*
Scenario: Splitting escaped dot‑notation path into components
Given a dot‑separated string with optional backslash‑escaped dots
When splitEscapedPath is called
Then it returns correct components, treating backslash‑escaped dots as literal dots

Related spec scenarios: RS.EXT.17
*/
func TestSplitEscapedPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected []string
	}{
		{"a.b.c", []string{"a", "b", "c"}},
		{"a\\.b.c", []string{"a.b", "c"}},
		{"a.b\\.c", []string{"a", "b.c"}},
		{"a\\.b\\.c", []string{"a.b.c"}},
		{"", []string{}},
		{"simple", []string{"simple"}},
		{"a\\.b\\.c.d.e\\.f", []string{"a.b.c", "d", "e.f"}},
		{"\\..\\.", []string{".", "."}}, // edge case
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			result := splitEscapedPath(tt.input)
			assert.Equal(t, tt.expected, result, "splitEscapedPath(%q)", tt.input)
		})
	}
}

/*
Scenario: Getting values from request source by dot‑notation path
Given a request source with path, query, header, body, and cookie data
When Get is called with a dot‑notation path
Then it returns the appropriate value and found flag, handling nested structures and missing keys

Related spec scenarios: RS.MSC.13, RS.MSC.14, RS.MSC.15, RS.MSC.16, RS.MSC.17
*/
func TestRequestSourceGet(t *testing.T) {
	t.Parallel()

	src := &RequestSource{
		PathParams: map[string]string{
			"id":   "123",
			"name": "test",
		},
		QueryParams: map[string][]string{
			"page":  {"1"},
			"sort":  {"name", "desc"},
			"empty": {},
		},
		Headers: map[string][]string{
			"content-type": {"application/json"},
			"x-custom":     {"value1", "value2"},
		},
		Body: map[string]any{
			"user": map[string]any{
				"name": "Alice",
				"age":  30,
			},
		},
		Cookies: map[string]string{
			"session": "abc123",
			"token":   "xyz",
		},
	}

	tests := []struct {
		path     string
		expected any
		found    bool
	}{
		{"path.id", "123", true},
		{"path.name", "test", true},
		{"path.missing", nil, false},
		{"query.page", "1", true},
		{"query.sort", []string{"name", "desc"}, true},
		{"query.empty", nil, false},
		{"header.content-type", "application/json", true},
		{"header.X-Custom", []string{"value1", "value2"}, true},
		{"body.user.name", "Alice", true},
		{"body.user.age", 30, true},
		{"body.user.missing", nil, false},
		{"cookie.session", "abc123", true},
		{"cookie.token", "xyz", true},
		// Escaped dot
		{"cookie.dot\\.key", nil, false}, // no such cookie
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			val, found := src.Get(tt.path)
			assert.Equal(t, tt.found, found, "Get(%q) found mismatch", tt.path)
			if tt.found {
				assert.Equal(t, tt.expected, val, "Get(%q) value mismatch", tt.path)
			}
		})
	}
}

/*
Scenario: Evaluating expression templates with multiple data sources
Given an evaluator with request, state, and environment sources
When Evaluate is called with expression containing variables and modifiers
Then it returns evaluated result or error for invalid expressions

Related spec scenarios: RS.MSC.13, RS.MSC.14, RS.MSC.15, RS.MSC.16, RS.MSC.17, RS.MSC.18, RS.MSC.19, RS.EXT.14, RS.EXT.15
*/
func TestEvaluator(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	eval := NewEvaluator()

	// Setup mock request source
	reqSrc := newMockDataSourceFromMap(ctrl, map[string]any{
		"path.userId":    "42",
		"query.mode":     "test",
		"header.accept":  "application/json",
		"body.data":      "value",
		"cookie.session": "s123",
	})
	eval.AddSource("request", reqSrc)

	// Setup mock state source
	stateSrc := newMockDataSourceFromMap(ctrl, map[string]any{
		"counter":           5,
		"user.profile.name": "Bob",
		"user.profile":      map[string]any{"name": "Bob"},
	})
	eval.AddSource("state", stateSrc)

	// Setup mock env source
	envSrc := newMockDataSourceFromMap(ctrl, map[string]any{
		"PORT": "8080",
		"MODE": "development",
	})
	eval.AddSource("env", envSrc)

	tests := []struct {
		expr     string
		expected any
		wantErr  bool
	}{
		// Basic expressions
		{"{$request.path.userId}", "42", false},
		{"{$request.query.mode}", "test", false},
		{"{$request.header.accept}", "application/json", false},
		{"{$request.body.data}", "value", false},
		{"{$request.cookie.session}", "s123", false},
		{"{$state.counter}", 5, false},
		{"{$env.PORT}", "8080", false},
		// Nested path
		{"{$state.user.profile.name}", "Bob", false},
		// Modifiers
		{"{$request.path.missing|default:unknown}", "unknown", false},
		{"{$state.user.profile.name|default:anonymous}", "Bob", false}, // value exists, default ignored
		{"{$state.user.profile|getByPath:name}", "Bob", false},
		// Errors
		{"invalid", nil, true},
		{"{$missing.source}", nil, true},
		// Missing path returns nil, not error
		{"{$request.missing}", nil, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.expr, func(t *testing.T) {
			t.Parallel()
			result, err := eval.Evaluate(tt.expr)
			if tt.wantErr {
				require.Error(t, err, "Evaluate(%q) expected error", tt.expr)
				return
			}
			require.NoError(t, err, "Evaluate(%q) unexpected error", tt.expr)
			assert.Equal(t, tt.expected, result, "Evaluate(%q)", tt.expr)
		})
	}
}
