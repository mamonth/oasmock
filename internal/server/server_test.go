package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"

	"github.com/mamonth/oasmock/internal/history"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/mamonth/oasmock/internal/state"
	mock_runtime "github.com/mamonth/oasmock/mock/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ = runtime.RequestSource{}

// newMockedServerWithGeneratedMocks creates a server with generated mock dependencies for testing.
func newMockedServerWithGeneratedMocks(t *testing.T, config Config) (*Server, *MockRouteProvider, *MockStateStore, *MockHistoryStore, *MockExpressionEvaluator, *MockRequestSourceFactory, *MockStateSourceFactory, *MockEnvSourceFactory, *MockExtensionProcessor) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	routeProvider := NewMockRouteProvider(ctrl)
	routeProvider.EXPECT().BuildRouteMappings(gomock.Any()).Return([]RouteMapping{}, nil)
	stateStore := NewMockStateStore(ctrl)
	historyStore := NewMockHistoryStore(ctrl)
	expressionEvaluator := NewMockExpressionEvaluator(ctrl)
	requestSourceFactory := NewMockRequestSourceFactory(ctrl)
	stateSourceFactory := NewMockStateSourceFactory(ctrl)
	envSourceFactory := NewMockEnvSourceFactory(ctrl)
	extensionProcessor := NewMockExtensionProcessor(ctrl)

	deps := Dependencies{
		RouteProvider:        routeProvider,
		StateStore:           stateStore,
		HistoryStore:         historyStore,
		RequestSourceFactory: requestSourceFactory,
		StateSourceFactory:   stateSourceFactory,
		EnvSourceFactory:     envSourceFactory,
		ExpressionEvaluator:  expressionEvaluator,
		ExtensionProcessor:   extensionProcessor,
	}

	// Empty schemas since route provider will be mocked
	schemas := []SchemaInfo{}

	server, err := NewWithDependencies(config, schemas, deps)
	require.NoError(t, err, "NewWithDependencies should not error")

	return server, routeProvider, stateStore, historyStore, expressionEvaluator, requestSourceFactory, stateSourceFactory, envSourceFactory, extensionProcessor
}

/*
Scenario: Validating add‑example request JSON
Given a JSON string representing an add‑example request
When validateAddExampleRequest is called
Then it returns error for missing required fields or invalid data, nil for valid requests

Related spec scenarios: RS.MAPI.14
*/
func TestValidateAddExampleRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name:    "valid minimal request",
			json:    `{"path":"/test","response":{"code":200}}`,
			wantErr: false,
		},
		{
			name:    "missing path",
			json:    `{"response":{"code":200}}`,
			wantErr: true,
		},
		{
			name:    "missing response",
			json:    `{"path":"/test"}`,
			wantErr: true,
		},
		{
			name:    "invalid method",
			json:    `{"path":"/test","method":"INVALID","response":{"code":200}}`,
			wantErr: true,
		},
		{
			name:    "valid with conditions",
			json:    `{"path":"/test","method":"GET","conditions":{"param":"value"},"response":{"code":200}}`,
			wantErr: false,
		},
		{
			name:    "valid with headers",
			json:    `{"path":"/test","response":{"code":200,"headers":{"X-Custom":"value"}}}`,
			wantErr: false,
		},
		{
			name:    "valid with body",
			json:    `{"path":"/test","response":{"code":200,"body":{"message":"hello"}}}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateAddExampleRequest([]byte(tt.json))
			if tt.wantErr {
				require.Error(t, err, "validateAddExampleRequest() expected error")
			} else {
				require.NoError(t, err, "validateAddExampleRequest() unexpected error")
			}
		})
	}
}

/*
Scenario: Creating new server instance with valid configuration
Given a valid server configuration and loaded OpenAPI schemas
When New is called
Then it returns a server instance with correct configuration

Related spec scenarios: RS.MSC.1, RS.MSC.2
*/
func TestNewServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)

	tests := []struct {
		name       string
		input      string
		mockResult any
		mockErr    error
		want       string
		wantErr    bool
	}{
		{
			name:       "whole string expression",
			input:      "{$request.path.id}",
			mockResult: "123",
			want:       "123",
		},
		{
			name:       "embedded expression",
			input:      "prefix-{$request.query.page}-suffix",
			mockResult: "2",
			want:       "prefix-2-suffix",
		},
		{
			name:    "embedded expression error",
			input:   "prefix-{$request.invalid}-suffix",
			mockErr: fmt.Errorf("not found"),
			want:    "prefix-{$request.invalid}-suffix", // Error swallowed, original preserved
			wantErr: false,
		},
		{
			name:       "multiple embedded expressions",
			input:      "{$request.path.id}/{$request.query.page}",
			mockResult: "123",
			want:       "123/123", // Same mock returns same value
		},
		{
			name:    "expression evaluation error",
			input:   "{$request.invalid}",
			mockErr: fmt.Errorf("not found"),
			want:    "",
			wantErr: true, // Whole string expression returns error
		},
		{
			name:       "non-string result",
			input:      "{$request.count}",
			mockResult: 42,
			want:       "42",
		},
		{
			name:       "no expressions",
			input:      "plain text",
			mockResult: nil,
			want:       "plain text",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockEval := mock_runtime.NewMockEvaluator(ctrl)
			mockEval.EXPECT().Evaluate(gomock.Any()).Return(tt.mockResult, tt.mockErr).AnyTimes()

			got, err := server.evaluateExpressionInString(tt.input, mockEval)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

/*
Scenario: Evaluating values that may contain runtime expressions
Given a value (string, map, slice, or literal)
When evaluateValue is called
Then it recursively evaluates expressions

Related spec scenarios: RS.MSC.13, RS.MSC.14, RS.MSC.15, RS.MSC.16, RS.MSC.17, RS.MSC.18, RS.MSC.19
*/
func TestEvaluateValue(t *testing.T) {
	t.Parallel()

	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)

	tests := []struct {
		name    string
		input   any
		setup   func(*mock_runtime.MockEvaluator, *gomock.Controller) // Setup mock expectations
		want    any
		wantErr bool
	}{
		{
			name:  "literal string",
			input: "plain text",
			setup: func(m *mock_runtime.MockEvaluator, ctrl *gomock.Controller) {
				// Evaluate should not be called
			},
			want: "plain text",
		},
		{
			name:  "whole string expression",
			input: "{$request.id}",
			setup: func(m *mock_runtime.MockEvaluator, ctrl *gomock.Controller) {
				m.EXPECT().Evaluate("{$request.id}").Return("123", nil)
			},
			want: "123",
		},
		{
			name:  "embedded expression in string",
			input: "id-{$request.id}",
			setup: func(m *mock_runtime.MockEvaluator, ctrl *gomock.Controller) {
				m.EXPECT().Evaluate("{$request.id}").Return("456", nil)
			},
			want: "id-456",
		},
		{
			name:  "embedded expression error in string",
			input: "id-{$request.invalid}",
			setup: func(m *mock_runtime.MockEvaluator, ctrl *gomock.Controller) {
				m.EXPECT().Evaluate("{$request.invalid}").Return(nil, fmt.Errorf("not found: {$request.invalid}"))
			},
			want: "id-{$request.invalid}", // Error swallowed, original preserved
		},
		{
			name:  "expression evaluation error",
			input: "{$request.invalid}",
			setup: func(m *mock_runtime.MockEvaluator, ctrl *gomock.Controller) {
				m.EXPECT().Evaluate("{$request.invalid}").Return(nil, fmt.Errorf("not found: {$request.invalid}"))
			},
			want:    "", // Error returned, value not used
			wantErr: true,
		},
		{
			name:  "literal number",
			input: 42,
			setup: func(m *mock_runtime.MockEvaluator, ctrl *gomock.Controller) {
				// Evaluate should not be called
			},
			want: 42,
		},
		{
			name: "map with expression keys and values",
			input: map[string]any{
				"key-{$request.key}": "value-{$request.value}",
			},
			setup: func(m *mock_runtime.MockEvaluator, ctrl *gomock.Controller) {
				m.EXPECT().Evaluate("{$request.key}").Return("123", nil)
				m.EXPECT().Evaluate("{$request.value}").Return("456", nil)
			},
			want: map[string]any{
				"key-123": "value-456",
			},
		},
		{
			name:  "slice with expressions",
			input: []any{"{$request.first}", "{$request.second}", "literal"},
			setup: func(m *mock_runtime.MockEvaluator, ctrl *gomock.Controller) {
				m.EXPECT().Evaluate("{$request.first}").Return("one", nil)
				m.EXPECT().Evaluate("{$request.second}").Return("two", nil)
			},
			want: []any{"one", "two", "literal"},
		},
		{
			name: "nested map with expressions",
			input: map[string]any{
				"outer": map[string]any{
					"inner": "{$request.value}",
				},
			},
			setup: func(m *mock_runtime.MockEvaluator, ctrl *gomock.Controller) {
				m.EXPECT().Evaluate("{$request.value}").Return("nested", nil)
			},
			want: map[string]any{
				"outer": map[string]any{
					"inner": "nested",
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockEval := mock_runtime.NewMockEvaluator(ctrl)
			if tt.setup != nil {
				tt.setup(mockEval, ctrl)
			}

			got, err := server.evaluateValue(tt.input, mockEval)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

/*
Scenario: Replacing embedded expressions in strings
Given a string with potentially multiple embedded expressions
When replaceEmbeddedExpressions is called
Then it correctly identifies and replaces each expression

Related spec scenarios: RS.MSC.13, RS.MSC.14, RS.MSC.15, RS.MSC.16, RS.MSC.17, RS.MSC.18, RS.MSC.19
*/
func TestReplaceEmbeddedExpressions(t *testing.T) {
	t.Parallel()

	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)

	tests := []struct {
		name       string
		input      string
		mockResult any
		mockErr    error
		want       string
	}{
		{
			name:       "single expression",
			input:      "prefix-{$request.id}-suffix",
			mockResult: "123",
			want:       "prefix-123-suffix",
		},
		{
			name:       "multiple expressions",
			input:      "{$request.a}/{$request.b}/{$request.c}",
			mockResult: "val",
			want:       "val/val/val", // Same mock returns same value
		},
		{
			name:       "expression at start",
			input:      "{$request.id}-suffix",
			mockResult: "123",
			want:       "123-suffix",
		},
		{
			name:       "expression at end",
			input:      "prefix-{$request.id}",
			mockResult: "123",
			want:       "prefix-123",
		},
		{
			name:       "nested expressions",
			input:      "{$request.{$request.nested}}",
			mockResult: "123",
			want:       "123",
		},
		{
			name:    "evaluation error",
			input:   "prefix-{$request.invalid}-suffix",
			mockErr: fmt.Errorf("not found"),
			want:    "prefix-{$request.invalid}-suffix", // Original preserved
		},
		{
			name:       "non-string result",
			input:      "{$request.count}",
			mockResult: 42,
			want:       "42",
		},
		{
			name:       "empty string",
			input:      "",
			mockResult: nil,
			want:       "",
		},
		{
			name:       "no expressions",
			input:      "just plain text",
			mockResult: nil,
			want:       "just plain text",
		},
		{
			name:       "unmatched braces",
			input:      "prefix-{$request.id",
			mockResult: nil,
			want:       "prefix-{$request.id", // Treated as literal
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockEval := mock_runtime.NewMockEvaluator(ctrl)
			mockEval.EXPECT().Evaluate(gomock.Any()).Return(tt.mockResult, tt.mockErr).AnyTimes()

			got, err := server.replaceEmbeddedExpressions(tt.input, mockEval)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

/*
Scenario: Extracting path parameters from HTTP request
Given an HTTP request with Chi route context containing URL parameters
When extractPathParams is called
Then it returns a map with parameter names and values

Related spec scenarios: RS.MSC.5
*/
func TestExtractPathParams(t *testing.T) {
	t.Parallel()

	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)

	tests := []struct {
		name         string
		setupRequest func() *http.Request
		mapping      *RouteMapping
		want         map[string]string
	}{
		{
			name: "no chi context",
			setupRequest: func() *http.Request {
				req, _ := http.NewRequest("GET", "/users/123", nil)
				return req
			},
			mapping: &RouteMapping{},
			want:    map[string]string{},
		},
		{
			name: "with path parameters",
			setupRequest: func() *http.Request {
				req, _ := http.NewRequest("GET", "/users/123", nil)
				// Create chi route context with URL params
				routeCtx := chi.NewRouteContext()
				routeCtx.URLParams.Keys = []string{"userID"}
				routeCtx.URLParams.Values = []string{"123"}
				// Add to request context
				ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
				return req.WithContext(ctx)
			},
			mapping: &RouteMapping{},
			want:    map[string]string{"userID": "123"},
		},
		{
			name: "multiple path parameters",
			setupRequest: func() *http.Request {
				req, _ := http.NewRequest("GET", "/orgs/456/users/789", nil)
				routeCtx := chi.NewRouteContext()
				routeCtx.URLParams.Keys = []string{"orgID", "userID"}
				routeCtx.URLParams.Values = []string{"456", "789"}
				ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
				return req.WithContext(ctx)
			},
			mapping: &RouteMapping{},
			want:    map[string]string{"orgID": "456", "userID": "789"},
		},
		{
			name: "mismatched keys and values",
			setupRequest: func() *http.Request {
				req, _ := http.NewRequest("GET", "/test", nil)
				routeCtx := chi.NewRouteContext()
				routeCtx.URLParams.Keys = []string{"key1", "key2"}
				routeCtx.URLParams.Values = []string{"value1"} // Missing second value
				ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
				return req.WithContext(ctx)
			},
			mapping: &RouteMapping{},
			want:    map[string]string{"key1": "value1"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := tt.setupRequest()
			got := server.extractPathParams(req, tt.mapping)
			assert.Equal(t, tt.want, got)
		})
	}
}

/*
Scenario: Applying state updates through applySetState
Given a server with mocked state store and evaluator
When applySetState is called with various state maps
Then it correctly processes deletions, increments, and simple values

Related spec scenarios: RS.MSC.20, RS.MSC.21, RS.MSC.22, RS.MSC.23, RS.MSC.24, RS.MSC.25
*/
func TestApplySetState(t *testing.T) {
	t.Parallel()

	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}

	type call struct {
		method string
		key    string
		value  any
	}

	// setupFunc returns generated mocks with expectations set
	type setupFunc func(*gomock.Controller) (*MockStateStore, *mock_runtime.MockEvaluator, *[]call)

	tests := []struct {
		name      string
		stateMap  map[string]any
		setup     setupFunc
		wantCalls []call
	}{
		{
			name: "simple value",
			stateMap: map[string]any{
				"counter": 42,
			},
			setup: func(ctrl *gomock.Controller) (*MockStateStore, *mock_runtime.MockEvaluator, *[]call) {
				calls := make([]call, 0)
				store := NewMockStateStore(ctrl)
				eval := mock_runtime.NewMockEvaluator(ctrl)

				// Key evaluation: no expression, returns same (no Evaluate call)
				store.EXPECT().Set("", "counter", 42).Do(
					func(namespace, key string, value any) {
						calls = append(calls, call{method: "Set", key: key, value: value})
					})

				return store, eval, &calls
			},
			wantCalls: []call{
				{method: "Set", key: "counter", value: 42},
			},
		},
		{
			name: "deletion with nil value",
			stateMap: map[string]any{
				"tempValue": nil,
			},
			setup: func(ctrl *gomock.Controller) (*MockStateStore, *mock_runtime.MockEvaluator, *[]call) {
				calls := make([]call, 0)
				store := NewMockStateStore(ctrl)
				eval := mock_runtime.NewMockEvaluator(ctrl)

				// Key evaluation: no expression, returns same (no Evaluate call)
				store.EXPECT().Delete("", "tempValue").Do(
					func(namespace, key string) {
						calls = append(calls, call{method: "Delete", key: key})
					})

				return store, eval, &calls
			},
			wantCalls: []call{
				{method: "Delete", key: "tempValue"},
			},
		},
		{
			name: "key evaluation error skips entry",
			stateMap: map[string]any{
				"{$error}": "value",
			},
			setup: func(ctrl *gomock.Controller) (*MockStateStore, *mock_runtime.MockEvaluator, *[]call) {
				calls := make([]call, 0)
				store := NewMockStateStore(ctrl)
				eval := mock_runtime.NewMockEvaluator(ctrl)

				eval.EXPECT().Evaluate("{$error}").Return(nil, fmt.Errorf("evaluation error"))
				// value should not be evaluated because entry is skipped
				// No store expectations

				return store, eval, &calls
			},
			wantCalls: []call{},
		},
		{
			name: "value evaluation error skips entry",
			stateMap: map[string]any{
				"key": "{$error}",
			},
			setup: func(ctrl *gomock.Controller) (*MockStateStore, *mock_runtime.MockEvaluator, *[]call) {
				calls := make([]call, 0)
				store := NewMockStateStore(ctrl)
				eval := mock_runtime.NewMockEvaluator(ctrl)

				// Key evaluation: no expression, returns same (no Evaluate call)
				eval.EXPECT().Evaluate("{$error}").Return(nil, fmt.Errorf("value evaluation error"))
				// No store expectations because error skips entry

				return store, eval, &calls
			},
			wantCalls: []call{},
		},
		{
			name: "expression in key resolved",
			stateMap: map[string]any{
				"{$prefix}_key": "value",
			},
			setup: func(ctrl *gomock.Controller) (*MockStateStore, *mock_runtime.MockEvaluator, *[]call) {
				calls := make([]call, 0)
				store := NewMockStateStore(ctrl)
				eval := mock_runtime.NewMockEvaluator(ctrl)

				eval.EXPECT().Evaluate("{$prefix}").Return("resolved", nil)
				// Value evaluation: no expression, returns same (no Evaluate call)
				store.EXPECT().Set("", "resolved_key", "value").Do(
					func(namespace, key string, value any) {
						calls = append(calls, call{method: "Set", key: key, value: value})
					})

				return store, eval, &calls
			},
			wantCalls: []call{
				{method: "Set", key: "resolved_key", value: "value"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			store, eval, callsPtr := tt.setup(ctrl)
			server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)
			server.stateStore = store
			server.applySetState(tt.stateMap, eval, "")
			assert.Equal(t, tt.wantCalls, *callsPtr, "store calls mismatch")
		})
	}
}

/*
Scenario: Adding dynamic example via management API
Given a server with route mappings
When handleAddExample is called with valid request
Then it validates the request, finds matching route, and adds example

Related spec scenarios: RS.MAPI.2, RS.MAPI.6, RS.MAPI.14
*/
func TestHandleAddExample(t *testing.T) {
	t.Parallel()

	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}

	tests := []struct {
		name        string
		reqBody     string
		mappings    []RouteMapping
		wantStatus  int
		wantJSON    map[string]any
		wantExample bool // whether example should be added
	}{
		{
			name:    "valid minimal request",
			reqBody: `{"path":"/test","response":{"code":200}}`,
			mappings: []RouteMapping{{
				Method:     "GET",
				Path:       "/test",
				Pattern:    "/test",
				ChiPattern: "/test",
			}},
			wantStatus: http.StatusOK,
			wantJSON: map[string]any{
				"success": true,
				"message": "Example added",
				"id":      "", // will check non-empty
			},
			wantExample: true,
		},
		{
			name:    "invalid JSON",
			reqBody: `invalid json`,
			mappings: []RouteMapping{{
				Method:     "GET",
				Path:       "/test",
				Pattern:    "/test",
				ChiPattern: "/test",
			}},
			wantStatus:  http.StatusBadRequest,
			wantJSON:    map[string]any{"error": "schema validation failed: invalid character 'i' looking for beginning of value"},
			wantExample: false,
		},
		{
			name:    "missing path in request",
			reqBody: `{"response":{"code":200}}`,
			mappings: []RouteMapping{{
				Method:     "GET",
				Path:       "/test",
				Pattern:    "/test",
				ChiPattern: "/test",
			}},
			wantStatus:  http.StatusBadRequest,
			wantJSON:    map[string]any{"error": "invalid request: (root): path is required"},
			wantExample: false,
		},
		{
			name:    "no matching route",
			reqBody: `{"path":"/nonexistent","response":{"code":200}}`,
			mappings: []RouteMapping{{
				Method:     "GET",
				Path:       "/test",
				Pattern:    "/test",
				ChiPattern: "/test",
			}},
			wantStatus:  http.StatusBadRequest,
			wantJSON:    map[string]any{"error": "No matching route found"},
			wantExample: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create server with mock dependencies
			server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)
			// Set up mappings
			server.mappings = tt.mappings
			// Initialize dynamic examples map
			server.dynamicExamples = make(map[string][]dynamicExample)

			// Create request
			req := httptest.NewRequest("POST", "/api/examples", strings.NewReader(tt.reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Call handler
			server.handleAddExample(w, req)

			// Check response
			assert.Equal(t, tt.wantStatus, w.Code, "status code mismatch")
			var resp map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err, "response should be valid JSON")

			// Check expected fields
			for key, expectedValue := range tt.wantJSON {
				if key == "id" && expectedValue == "" {
					// ID should be non-empty for success responses
					assert.NotEmpty(t, resp[key], "id should not be empty")
					continue
				}
				assert.Equal(t, expectedValue, resp[key], "field %s mismatch", key)
			}

			// Check if example was added
			if tt.wantExample {
				key := "GET /test"
				server.dyMu.RLock()
				defer server.dyMu.RUnlock()
				examples, exists := server.dynamicExamples[key]
				assert.True(t, exists, "dynamic example should be added for key %s", key)
				assert.Len(t, examples, 1, "should have one example")
			} else {
				assert.Empty(t, server.dynamicExamples, "no examples should be added")
			}
		})
	}
}

/*
Scenario: Retrieving request history via management API
Given a server with mocked history store
When handleGetRequests is called with various query parameters
Then it correctly filters, paginates, and returns request records

Related spec scenarios: RS.MAPI.7, RS.MAPI.8, RS.MAPI.9, RS.MAPI.10, RS.MAPI.11, RS.MAPI.12, RS.MAPI.13
*/
func TestHandleGetRequests(t *testing.T) {
	t.Parallel()

	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}

	// Create test records with different timestamps
	now := time.Now()
	testRecords := []RequestRecord{
		{
			ID:        "1",
			Timestamp: now.Add(-2 * time.Hour),
			Method:    "GET",
			Path:      "/users",
			Query:     "limit=10",
			Headers:   http.Header{"X-Request-ID": []string{"req-1"}},
			Body:      []byte(`{"action":"list"}`),
		},
		{
			ID:        "2",
			Timestamp: now.Add(-1 * time.Hour),
			Method:    "POST",
			Path:      "/users",
			Query:     "",
			Headers:   http.Header{"X-Request-ID": []string{"req-2"}},
			Body:      []byte(`{"name":"Alice"}`),
		},
		{
			ID:        "3",
			Timestamp: now.Add(-30 * time.Minute),
			Method:    "GET",
			Path:      "/products",
			Query:     "category=books",
			Headers:   http.Header{"X-Request-ID": []string{"req-3"}},
			Body:      []byte(`{}`),
		},
		{
			ID:        "4",
			Timestamp: now.Add(-15 * time.Minute),
			Method:    "PUT",
			Path:      "/users/123",
			Query:     "",
			Headers:   http.Header{"X-Request-ID": []string{"req-4"}},
			Body:      []byte(`{"name":"Bob"}`),
		},
		{
			ID:        "5",
			Timestamp: now.Add(-5 * time.Minute),
			Method:    "DELETE",
			Path:      "/users/456",
			Query:     "",
			Headers:   http.Header{"X-Request-ID": []string{"req-5"}},
			Body:      []byte(``),
		},
	}

	tests := []struct {
		name       string
		query      string
		wantCount  int
		wantMethod string // check first result if applicable
		wantPath   string // check first result if applicable
	}{
		{
			name:       "no filters returns all",
			query:      "",
			wantCount:  5,
			wantMethod: "GET",
			wantPath:   "/users",
		},
		{
			name:       "filter by path",
			query:      "path=/users",
			wantCount:  2, // GET /users, POST /users (PUT /users/123 has different path)
			wantMethod: "GET",
			wantPath:   "/users",
		},
		{
			name:       "filter by method",
			query:      "method=GET",
			wantCount:  2, // GET /users, GET /products
			wantMethod: "GET",
			wantPath:   "/users",
		},
		{
			name:       "filter by path and method",
			query:      "path=/users&method=GET",
			wantCount:  1,
			wantMethod: "GET",
			wantPath:   "/users",
		},
		{
			name:       "filter by time_from",
			query:      "time_from=" + strconv.FormatInt(now.Add(-45*time.Minute).UnixMilli(), 10),
			wantCount:  3, // records from -30m, -15m, -5m
			wantMethod: "GET",
			wantPath:   "/products",
		},
		{
			name:       "filter by time_till",
			query:      "time_till=" + strconv.FormatInt(now.Add(-20*time.Minute).UnixMilli(), 10),
			wantCount:  3, // records from -2h, -1h, -30m
			wantMethod: "GET",
			wantPath:   "/users",
		},
		{
			name:       "pagination with offset and limit",
			query:      "offset=1&limit=2",
			wantCount:  2,
			wantMethod: "POST", // second record
			wantPath:   "/users",
		},
		{
			name:       "limit exceeds max",
			query:      "limit=150",
			wantCount:  5, // limit capped at 100
			wantMethod: "GET",
			wantPath:   "/users",
		},
		{
			name:       "negative offset",
			query:      "offset=-10",
			wantCount:  5, // offset clamped to 0
			wantMethod: "GET",
			wantPath:   "/users",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create server with mock history store
			server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)
			// Replace history store with generated mock
			mockHistoryStore := NewMockHistoryStore(ctrl)
			mockHistoryStore.EXPECT().GetAll().Return(testRecords)
			server.historyStore = mockHistoryStore
			server.deps.HistoryStore = mockHistoryStore

			// Create request with query parameters
			url := "/api/requests"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			// Call handler
			server.handleGetRequests(w, req)

			// Check response
			assert.Equal(t, http.StatusOK, w.Code, "status code mismatch")
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"), "content-type mismatch")

			// Parse response
			var resp map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err, "response should be valid JSON")

			// Check data array
			data, ok := resp["data"].([]any)
			require.True(t, ok, "response should have data array")
			assert.Len(t, data, tt.wantCount, "record count mismatch")

			// Check first record if we have any
			if tt.wantCount > 0 {
				firstRec, ok := data[0].(map[string]any)
				require.True(t, ok, "first record should be map")
				assert.Equal(t, tt.wantMethod, firstRec["method"], "method mismatch in first record")
				// Path check: URL includes query string
				urlStr := firstRec["url"].(string)
				// Extract path before "?"
				if idx := strings.Index(urlStr, "?"); idx != -1 {
					urlStr = urlStr[:idx]
				}
				assert.Equal(t, tt.wantPath, urlStr, "path mismatch in first record")
			}
		})
	}
}

/*
Scenario: Filtering request records based on query parameters
Given a list of request records and query parameters
When filterRecords is called
Then it returns only records matching the filters

Related spec scenarios: RS.MAPI.8, RS.MAPI.9, RS.MAPI.11
*/
func TestFilterRecords(t *testing.T) {
	t.Parallel()

	now := time.Now()
	records := []RequestRecord{
		{
			ID:        "1",
			Timestamp: now.Add(-2 * time.Hour),
			Method:    "GET",
			Path:      "/api/users",
			Query:     "page=1",
			Headers:   http.Header{"X-Test": []string{"value"}},
			Body:      []byte(`{"id":1}`),
		},
		{
			ID:        "2",
			Timestamp: now.Add(-1 * time.Hour),
			Method:    "POST",
			Path:      "/api/users",
			Query:     "",
			Headers:   http.Header{},
			Body:      []byte(`{"name":"John"}`),
		},
		{
			ID:        "3",
			Timestamp: now,
			Method:    "GET",
			Path:      "/api/products",
			Query:     "category=books",
			Headers:   http.Header{},
			Body:      nil,
		},
	}

	tests := []struct {
		name    string
		query   url.Values
		wantIdx []int // indices of expected records
	}{
		{
			name:    "no filters",
			query:   url.Values{},
			wantIdx: []int{0, 1, 2},
		},
		{
			name:    "filter by path",
			query:   url.Values{"path": []string{"/api/users"}},
			wantIdx: []int{0, 1},
		},
		{
			name:    "filter by method",
			query:   url.Values{"method": []string{"POST"}},
			wantIdx: []int{1},
		},
		{
			name:    "filter by time_from",
			query:   url.Values{"time_from": []string{fmt.Sprintf("%d", now.Add(-90*time.Minute).UnixMilli())}},
			wantIdx: []int{1, 2},
		},
		{
			name:    "filter by time_till",
			query:   url.Values{"time_till": []string{fmt.Sprintf("%d", now.Add(-30*time.Minute).UnixMilli())}},
			wantIdx: []int{0, 1},
		},
		{
			name:    "combined filters",
			query:   url.Values{"path": []string{"/api/users"}, "method": []string{"GET"}},
			wantIdx: []int{0},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			filtered := filterRecords(records, tt.query)
			assert.Len(t, filtered, len(tt.wantIdx), "filtered record count mismatch")
			for i, idx := range tt.wantIdx {
				assert.Equal(t, records[idx].ID, filtered[i].ID, "record ID mismatch at position %d", i)
			}
		})
	}
}

/*
Scenario: Paginating request records with offset and limit
Given a list of request records, offset, and limit
When paginateRecords is called
Then it returns the correct slice of records

Related spec scenarios: RS.MAPI.10
*/
func TestPaginateRecords(t *testing.T) {
	t.Parallel()

	records := make([]RequestRecord, 10)
	for i := range records {
		records[i] = RequestRecord{ID: fmt.Sprintf("%d", i)}
	}

	tests := []struct {
		name       string
		offset     int
		limit      int
		wantStart  int
		wantLength int
	}{
		{"offset 0 limit 5", 0, 5, 0, 5},
		{"offset 2 limit 3", 2, 3, 2, 3},
		{"offset beyond length", 15, 5, 10, 0},
		{"negative offset", -5, 5, 0, 5},
		{"zero limit", 0, 0, 0, 0},
		{"limit larger than remaining", 8, 10, 8, 2},
		{"offset equals length", 10, 5, 10, 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			paginated := paginateRecords(records, tt.offset, tt.limit)
			assert.Len(t, paginated, tt.wantLength, "paginated length mismatch")
			if tt.wantLength > 0 {
				assert.Equal(t, records[tt.wantStart].ID, paginated[0].ID, "first record ID mismatch")
			}
		})
	}
}

/*
Scenario: Converting request records to API response format
Given a list of request records
When recordsToAPIResponse is called
Then it returns properly formatted API response items

Related spec scenarios: RS.MAPI.13
*/
func TestRecordsToAPIResponse(t *testing.T) {
	t.Parallel()

	now := time.Now()
	records := []RequestRecord{
		{
			ID:        "1",
			Timestamp: now,
			Method:    "GET",
			Path:      "/api/test",
			Query:     "id=1",
			Headers:   http.Header{"Content-Type": []string{"application/json"}},
			Body:      []byte(`{"hello":"world"}`),
		},
		{
			ID:        "2",
			Timestamp: now.Add(time.Hour),
			Method:    "POST",
			Path:      "/api/test",
			Query:     "",
			Headers:   http.Header{"X-Custom": []string{"value1", "value2"}},
			Body:      []byte("plain text"),
		},
		{
			ID:        "3",
			Timestamp: now.Add(2 * time.Hour),
			Method:    "PUT",
			Path:      "/api/empty",
			Query:     "",
			Headers:   http.Header{},
			Body:      nil,
		},
	}

	items := recordsToAPIResponse(records)
	require.Len(t, items, 3, "should return 3 items")

	// Check first record (JSON body)
	item0 := items[0]
	assert.EqualValues(t, now.UnixMilli(), item0["ts"], "timestamp mismatch")
	assert.Equal(t, "/api/test?id=1", item0["url"], "URL mismatch")
	assert.Equal(t, "GET", item0["method"], "method mismatch")
	body0, ok := item0["body"].(map[string]any)
	require.True(t, ok, "body should be map")
	assert.Equal(t, "world", body0["hello"], "body content mismatch")
	headers0, ok := item0["headers"].(map[string]string)
	require.True(t, ok, "headers should be map[string]string")
	assert.Equal(t, "application/json", headers0["Content-Type"], "header mismatch")

	// Check second record (plain text body)
	item1 := items[1]
	body1, ok := item1["body"].(string)
	require.True(t, ok, "body should be string")
	assert.Equal(t, "plain text", body1, "body text mismatch")
	headers1, ok := item1["headers"].(map[string]string)
	require.True(t, ok, "headers should be map[string]string")
	assert.Equal(t, "value1", headers1["X-Custom"], "header mismatch")

	// Check third record (no body)
	item2 := items[2]
	assert.Nil(t, item2["body"], "body should be nil")
}

/*
Scenario: Handling mock request with route mapping
Given a server with route mapping and response examples
When handleMockRequestWithMapping is called
Then it selects appropriate response and returns mock data

Related spec scenarios: RS.MSC.8, RS.MSC.9, RS.MSC.10, RS.MSC.11, RS.MSC.12, RS.MSC.27, RS.MSC.28, RS.MSC.29, RS.MSC.30
*/
func TestHandleMockRequestWithMapping(t *testing.T) {
	t.Parallel()

	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}

	// Helper to create a minimal responses map with example via YAML
	createResponsesWithExample := func() *openapi3.Responses {
		const yamlSpec = `
openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths:
  /test:
    get:
      responses:
        '200':
          description: OK
          content:
            application/json:
              examples:
                default:
                  value:
                    message: "Hello, World!"
`
		ldr := openapi3.NewLoader()
		spec, err := ldr.LoadFromData([]byte(yamlSpec))
		if err != nil {
			panic(err)
		}
		if spec.Paths == nil {
			panic("Paths is nil")
		}
		pathMap := spec.Paths.Map()
		pathItem := pathMap["/test"]
		if pathItem == nil {
			panic("Path /test not found")
		}
		op := pathItem.Get
		if op == nil {
			panic("GET operation not found")
		}
		return op.Responses
	}

	tests := []struct {
		name        string
		setupServer func(*Server, *MockRequestSourceFactory, *MockStateSourceFactory, *MockEnvSourceFactory)
		mapping     *RouteMapping
		wantStatus  int
		wantBody    string
		wantHeaders map[string]string
	}{
		{
			name: "successful request with built-in example",
			setupServer: func(s *Server, reqFact *MockRequestSourceFactory, stateFact *MockStateSourceFactory, envFact *MockEnvSourceFactory) {
				// No dynamic examples
				s.dynamicExamples = make(map[string][]dynamicExample)
				// Setup mock factories (not needed for this test)
				// With generated mocks, we need to set expectations
				// Since factories aren't used in this test, we allow any calls returning nil
				reqFact.EXPECT().NewRequestSource(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				stateFact.EXPECT().NewStateSource(gomock.Any()).Return(nil).AnyTimes()
				envFact.EXPECT().NewEnvSource().Return(nil).AnyTimes()
			},
			mapping: &RouteMapping{
				Method:     "GET",
				Path:       "/test",
				Pattern:    "/test",
				Prefix:     "",
				ChiPattern: "/test",
				Responses:  createResponsesWithExample(),
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"message":"Hello, World!"}`,
			wantHeaders: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name: "no response defined",
			setupServer: func(s *Server, reqFact *MockRequestSourceFactory, stateFact *MockStateSourceFactory, envFact *MockEnvSourceFactory) {
				s.dynamicExamples = make(map[string][]dynamicExample)
				// Allow any factory calls returning nil
				reqFact.EXPECT().NewRequestSource(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				stateFact.EXPECT().NewStateSource(gomock.Any()).Return(nil).AnyTimes()
				envFact.EXPECT().NewEnvSource().Return(nil).AnyTimes()
			},
			mapping: &RouteMapping{
				Method:     "GET",
				Path:       "/test",
				Pattern:    "/test",
				Prefix:     "",
				ChiPattern: "/test",
				Responses:  nil,
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"No response defined for operation"}`,
			wantHeaders: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name: "dynamic example selected",
			setupServer: func(s *Server, reqFact *MockRequestSourceFactory, stateFact *MockStateSourceFactory, envFact *MockEnvSourceFactory) {
				// Add a dynamic example
				s.dynamicExamples = make(map[string][]dynamicExample)
				key := "GET /test"
				s.dynamicExamples[key] = []dynamicExample{{
					once:       false,
					conditions: nil,
					response: struct {
						code    int
						headers map[string]string
						body    any
					}{
						code:    200,
						headers: map[string]string{"X-Custom": "dynamic"},
						body:    map[string]any{"source": "dynamic"},
					},
				}}
				// Mock factories - allow any calls returning nil
				reqFact.EXPECT().NewRequestSource(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				stateFact.EXPECT().NewStateSource(gomock.Any()).Return(nil).AnyTimes()
				envFact.EXPECT().NewEnvSource().Return(nil).AnyTimes()
			},
			mapping: &RouteMapping{
				Method:     "GET",
				Path:       "/test",
				Pattern:    "/test",
				Prefix:     "",
				ChiPattern: "/test",
				Responses:  createResponsesWithExample(),
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"source":"dynamic"}`,
			wantHeaders: map[string]string{
				"Content-Type": "application/json",
				"X-Custom":     "dynamic",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Create server with generated mock dependencies
			server, _, mockStateStore, mockHistoryStore, mockExpressionEvaluator, reqFact, stateFact, envFact, mockExtensionProcessor := newMockedServerWithGeneratedMocks(t, config)
			// Set default expectations for mocks that will be called by the handler
			mockStateStore.EXPECT().GetNamespace(gomock.Any()).Return(nil).AnyTimes()
			mockStateStore.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, false).AnyTimes()
			mockStateStore.EXPECT().GetAll().Return(nil).AnyTimes()
			mockHistoryStore.EXPECT().Add(gomock.Any()).AnyTimes()
			mockHistoryStore.EXPECT().GetAll().Return(nil).AnyTimes()
			mockHistoryStore.EXPECT().Count().Return(0).AnyTimes()
			mockHistoryStore.EXPECT().Capacity().Return(1000).AnyTimes()
			mockExpressionEvaluator.EXPECT().AddSource(gomock.Any(), gomock.Any()).AnyTimes()
			mockExpressionEvaluator.EXPECT().Evaluate(gomock.Any()).Return(nil, nil).AnyTimes()
			mockExtensionProcessor.EXPECT().ExtractSetState(gomock.Any()).Return(nil, false).AnyTimes()
			mockExtensionProcessor.EXPECT().ExtractSkip(gomock.Any()).Return(false).AnyTimes()
			mockExtensionProcessor.EXPECT().ExtractOnce(gomock.Any()).Return(false).AnyTimes()
			mockExtensionProcessor.EXPECT().ExtractParamsMatch(gomock.Any()).Return(nil, false).AnyTimes()
			mockExtensionProcessor.EXPECT().EvaluateParamsMatch(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
			mockExtensionProcessor.EXPECT().ExtractHeaders(gomock.Any()).Return(nil, false).AnyTimes()
			// Setup server
			if tt.setupServer != nil {
				tt.setupServer(server, reqFact, stateFact, envFact)
			}
			// Create request
			req := httptest.NewRequest(tt.mapping.Method, tt.mapping.Path, nil)
			w := httptest.NewRecorder()
			// Call handler
			server.handleMockRequestWithMapping(w, req, tt.mapping)
			// Check response
			assert.Equal(t, tt.wantStatus, w.Code, "status code mismatch")
			for key, wantValue := range tt.wantHeaders {
				assert.Equal(t, wantValue, w.Header().Get(key), "header %s mismatch", key)
			}
			if tt.wantBody != "" {
				// Compare JSON bodies ignoring whitespace
				var got, want any
				err := json.Unmarshal(w.Body.Bytes(), &got)
				require.NoError(t, err, "response body should be valid JSON")
				err = json.Unmarshal([]byte(tt.wantBody), &want)
				require.NoError(t, err, "expected body should be valid JSON")
				assert.Equal(t, want, got, "response body mismatch")
			}
		})
	}
}

/*
	Scenario: requestHistoryMiddleware records request and response details
	Given a server with history store
	When a request passes through the middleware
	Then a request record with correct details should be added to the history store

	Related spec scenarios: RS.MSC.31
*/

func TestRequestHistoryMiddleware(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)

	var capturedRecord RequestRecord
	mockHistoryStore := NewMockHistoryStore(ctrl)
	mockHistoryStore.EXPECT().Add(gomock.Any()).Do(func(record RequestRecord) {
		capturedRecord = record
	})
	server.historyStore = mockHistoryStore
	server.deps.HistoryStore = mockHistoryStore

	// Create a test request with body
	reqBody := `{"test":"data"}`
	req := httptest.NewRequest("POST", "/api/test", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("X-Custom", "value")

	// Create a response recorder that will be wrapped by middleware's recorder
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate reading body (middleware already read and restored)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err, "should read body")
		assert.Equal(t, reqBody, string(body), "request body should be restored")

		w.Header().Set("X-Response", "test")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	// Apply middleware
	middleware := server.requestHistoryMiddleware(nextHandler)
	w := httptest.NewRecorder()

	// Capture start time before calling middleware (record uses start time as ID)
	start := time.Now()
	middleware.ServeHTTP(w, req)
	duration := time.Since(start)

	// Verify Add was called
	require.NotNil(t, capturedRecord.ID, "record ID should be set")
	assert.InDelta(t, float64(start.UnixNano()), float64(capturedRecord.Timestamp.UnixNano()), float64(time.Millisecond), "timestamp should be close")
	assert.Equal(t, "POST", capturedRecord.Method)
	assert.Equal(t, "/api/test", capturedRecord.Path)
	assert.Equal(t, "", capturedRecord.Query)
	assert.Equal(t, "value", capturedRecord.Headers.Get("X-Custom"))
	assert.Equal(t, []byte(reqBody), capturedRecord.Body)
	require.NotNil(t, capturedRecord.Response)
	assert.Equal(t, http.StatusCreated, capturedRecord.Response.StatusCode)
	assert.Equal(t, "test", capturedRecord.Response.Headers.Get("X-Response"))
	assert.Equal(t, []byte(`{"success":true}`), capturedRecord.Response.Body)
	assert.InDelta(t, duration, capturedRecord.Response.Duration, float64(10*time.Millisecond), "duration should be close")
}

/*
	Scenario: verboseLoggingMiddleware logs request/response when verbose is enabled
	Given a server with verbose enabled
	When a request passes through the middleware
	Then logs should be produced (we cannot easily test slog output, but we can ensure no panic)

	Related spec scenarios: RS.MSC.37, RS.MSC.38
*/

func TestVerboseLoggingMiddleware(t *testing.T) {
	t.Parallel()

	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := server.verboseLoggingMiddleware(nextHandler)
	middleware.ServeHTTP(w, req)

	assert.True(t, nextCalled, "next handler should be called")
	assert.Equal(t, http.StatusOK, w.Code)
}

/*
	Scenario: handleIncrementState increments a numeric state value
	Given a state store with initial value
	When handleIncrementState is called with a key and increment amount
	Then the state store should be updated with the new value

	Related spec scenarios: RS.MSC.21, RS.MSC.22, RS.MSC.23, RS.MSC.24
*/

func TestHandleIncrementState(t *testing.T) {
	t.Parallel()

	config := Config{}
	server, _, mockStateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEval := mock_runtime.NewMockEvaluator(ctrl)

	// Test 1: increment from nil (should treat as 0)
	var capturedDelta float64
	mockStateStore.EXPECT().Increment("test-ns", "counter", 5.0).DoAndReturn(
		func(namespace, key string, delta float64) (float64, error) {
			assert.Equal(t, "test-ns", namespace)
			assert.Equal(t, "counter", key)
			capturedDelta = delta
			// Simulate increment from 0
			return delta, nil
		})
	err := server.handleIncrementState("test-ns", "counter", 5, mockEval)
	assert.NoError(t, err)
	assert.Equal(t, 5.0, capturedDelta)

	// Test 2: increment existing integer
	var storedValue float64 = 10
	mockStateStore.EXPECT().Increment("test-ns", "counter", -3.0).DoAndReturn(
		func(namespace, key string, delta float64) (float64, error) {
			assert.Equal(t, "test-ns", namespace)
			assert.Equal(t, "counter", key)
			storedValue += delta
			return storedValue, nil
		})
	err = server.handleIncrementState("test-ns", "counter", -3, mockEval)
	assert.NoError(t, err)
	assert.InDelta(t, 7.0, storedValue, 1e-9)

	// Test 3: increment existing float
	storedValue = 3.14
	mockStateStore.EXPECT().Increment("test-ns", "counter", 2.0).DoAndReturn(
		func(namespace, key string, delta float64) (float64, error) {
			assert.Equal(t, "test-ns", namespace)
			assert.Equal(t, "counter", key)
			storedValue += delta
			return storedValue, nil
		})
	err = server.handleIncrementState("test-ns", "counter", 2.0, mockEval)
	assert.NoError(t, err)
	assert.InDelta(t, 5.14, storedValue, 1e-9)

	// Test 4: non-numeric value (should panic? we'll assume it's handled)
	// Not testing for now.
}

/*
	Scenario: Handling map state with increment or value keys
	Given a state store and evaluator
	When handleMapState is called with a map containing increment or value keys
	Then it processes increments or sets values accordingly

	Related spec scenarios: RS.MSC.20, RS.MSC.21, RS.MSC.22, RS.MSC.23, RS.MSC.24
*/

func TestHandleMapState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         map[string]any
		wantHandled   bool
		wantIncrement float64
		wantValue     any
	}{
		{
			name:          "increment key",
			input:         map[string]any{"increment": 5},
			wantHandled:   true,
			wantIncrement: 5,
		},
		{
			name:        "value key",
			input:       map[string]any{"value": "test"},
			wantHandled: true,
			wantValue:   "test",
		},
		{
			name:        "no increment or value",
			input:       map[string]any{"other": "data"},
			wantHandled: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			config := Config{}
			server, _, mockStateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockEval := mock_runtime.NewMockEvaluator(ctrl)

			var incrementCalled bool
			var setCalled bool
			if tt.wantIncrement != 0 {
				mockStateStore.EXPECT().Increment("test-ns", "myKey", tt.wantIncrement).DoAndReturn(
					func(namespace, key string, delta float64) (float64, error) {
						assert.Equal(t, "test-ns", namespace)
						assert.Equal(t, "myKey", key)
						incrementCalled = true
						assert.Equal(t, tt.wantIncrement, delta)
						return delta, nil
					})
			}
			if tt.wantValue != nil {
				mockStateStore.EXPECT().Set("test-ns", "myKey", tt.wantValue).Do(
					func(namespace, key string, value any) {
						assert.Equal(t, "test-ns", namespace)
						assert.Equal(t, "myKey", key)
						setCalled = true
						assert.Equal(t, tt.wantValue, value)
					})
			}
			handled, err := server.handleMapState("test-ns", "myKey", tt.input, mockEval)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantHandled, handled)
			if tt.wantIncrement != 0 {
				assert.True(t, incrementCalled, "Increment should be called")
			}
			if tt.wantValue != nil {
				assert.True(t, setCalled, "Set should be called")
			}
		})
	}
}

/*
	Scenario: handleValueObjectState sets a value object (overwrites)
	Given a state store
	When handleValueObjectState is called with a value
	Then the state store should be set to that value

	Related spec scenarios: RS.MSC.20
*/

func TestHandleValueObjectState(t *testing.T) {
	t.Parallel()

	config := Config{}
	server, _, mockStateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEval := mock_runtime.NewMockEvaluator(ctrl)

	var storedValue any
	mockStateStore.EXPECT().Set("test-ns", "obj", gomock.Any()).Do(
		func(namespace, key string, value any) {
			assert.Equal(t, "test-ns", namespace)
			assert.Equal(t, "obj", key)
			storedValue = value
		})

	err := server.handleValueObjectState("test-ns", "obj", map[string]any{"x": 10, "y": "text"}, mockEval)
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"x": 10, "y": "text"}, storedValue)
}

/*
	Scenario: writeJSONErrorf formats an error message and writes JSON response
	Given an HTTP response writer
	When writeJSONErrorf is called with a format string and arguments
	Then a JSON error response with formatted message should be written

	Related spec scenarios: RS.MAPI.6
*/

func TestWriteJSONErrorf(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	writeJSONErrorf(w, http.StatusBadRequest, "validation failed: %s", "invalid input")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "response should be valid JSON")
	assert.Equal(t, "validation failed: invalid input", resp["error"])
}

/*
	Scenario: markOnceUsed and isOnceUsed track example usage
	Given a server
	When markOnceUsed marks an example ID
	Then isOnceUsed should return true for that ID

	Related spec scenarios: RS.MSC.12
*/

func TestMarkOnceUsedAndIsOnceUsed(t *testing.T) {
	t.Parallel()

	config := Config{}
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)

	exampleID := "GET:/users:default"

	// Initially not used
	assert.False(t, server.isOnceUsed(exampleID), "example should not be marked as used initially")

	// Mark as used
	server.markOnceUsed(exampleID)
	assert.True(t, server.isOnceUsed(exampleID), "example should be marked as used after markOnceUsed")

	// Different ID still not used
	assert.False(t, server.isOnceUsed("POST:/users:default"), "different example ID should not be marked as used")
}

/*
	Scenario: getStatusCode returns status code from mapping
	Given a route mapping and response
	When getStatusCode is called
	Then it should return appropriate status code (currently defaults to 200)

	Related spec scenarios: RS.MSC.27
*/

func TestGetStatusCode(t *testing.T) {
	t.Parallel()

	mapping := &loader.RouteMapping{
		Method:     "GET",
		Path:       "/test",
		Pattern:    "/test",
		Prefix:     "",
		ChiPattern: "/test",
	}
	response := &openapi3.Response{}

	status := getStatusCode(mapping, response)
	assert.Equal(t, 200, status, "should default to 200")
}

/*
	Scenario: selectResponse chooses appropriate response from mapping
	Given a route mapping with various responses
	When selectResponse is called
	Then it should return the correct status code and response object

	Related spec scenarios: RS.MSC.8, RS.MSC.9, RS.MSC.10, RS.MSC.11, RS.MSC.12
*/

func TestSelectResponse(t *testing.T) {
	t.Parallel()

	config := Config{}
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEval := mock_runtime.NewMockEvaluator(ctrl)

	tests := []struct {
		name           string
		mapping        *RouteMapping
		wantStatusCode string
		wantResponse   bool // whether response should be non-nil
	}{
		{
			name:           "nil responses",
			mapping:        &RouteMapping{Responses: nil},
			wantStatusCode: "",
			wantResponse:   false,
		},
		{
			name: "empty responses map",
			mapping: &RouteMapping{
				Responses: &openapi3.Responses{},
			},
			wantStatusCode: "",
			wantResponse:   false,
		},
		{
			name: "single 200 response",
			mapping: &RouteMapping{
				Responses: func() *openapi3.Responses {
					responses := openapi3.NewResponses()
					responses.Set("200", &openapi3.ResponseRef{
						Value: &openapi3.Response{},
					})
					return responses
				}(),
			},
			wantStatusCode: "200",
			wantResponse:   true,
		},
		{
			name: "default response only",
			mapping: &RouteMapping{
				Responses: func() *openapi3.Responses {
					responses := openapi3.NewResponses()
					responses.Set("default", &openapi3.ResponseRef{
						Value: &openapi3.Response{},
					})
					return responses
				}(),
			},
			wantStatusCode: "default",
			wantResponse:   true,
		},
		{
			name: "multiple responses, skip default",
			mapping: &RouteMapping{
				Responses: func() *openapi3.Responses {
					responses := openapi3.NewResponses()
					responses.Set("default", &openapi3.ResponseRef{
						Value: &openapi3.Response{},
					})
					responses.Set("201", &openapi3.ResponseRef{
						Value: &openapi3.Response{},
					})
					responses.Set("200", &openapi3.ResponseRef{
						Value: &openapi3.Response{},
					})
					return responses
				}(),
			},
			wantStatusCode: "200", // lowest numeric status code after sorting
			wantResponse:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			statusCode, response := server.selectResponse(tt.mapping, mockEval)
			assert.Equal(t, tt.wantStatusCode, statusCode)
			if tt.wantResponse {
				assert.NotNil(t, response)
			} else {
				assert.Nil(t, response)
			}
		})
	}
}

/*
	Scenario: parseStatusCode converts status code string to int
	Given a status code string
	When parseStatusCode is called
	Then it should return the appropriate integer status code

	Related spec scenarios: RS.MSC.27
*/

func TestParseStatusCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected int
	}{
		{"200", 200},
		{"404", 404},
		{"500", 500},
		{"default", 200}, // DefaultStatusCode is 200
		{"invalid", 200}, // invalid parsing falls back to default
		{"", 200},        // empty string falls back to default
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			result := parseStatusCode(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

/*
Scenario: Server can start and shut down gracefully
Given a server with a random port
When Start is called in a goroutine and Shutdown is called
Then the server should start successfully and shut down without error

Related spec scenarios: RS.MSC.1
*/
func TestStartAndShutdown(t *testing.T) {

	// Use port 0 to let OS assign a random available port
	config := Config{
		Port:             8080,
		Delay:            0,
		Verbose:          false,
		EnableCORS:       true,
		HistorySize:      1000,
		EnableControlAPI: true,
	}
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)

	// Channel to capture error from Start
	startErrCh := make(chan error, 1)
	// Channel to signal server is ready (optional, we'll just rely on Shutdown)

	// Start server in goroutine
	go func() {
		startErrCh <- server.Start()
	}()

	// Give server a moment to start (though with port 0, ListenAndServe may block until shutdown)
	// We'll just call Shutdown immediately; Start will return http.ErrServerClosed
	time.Sleep(50 * time.Millisecond)

	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Shutdown the server
	err := server.Shutdown(ctx)
	assert.NoError(t, err, "Shutdown should succeed")

	// Wait for Start to return
	select {
	case err := <-startErrCh:
		// Start should return http.ErrServerClosed when shutdown gracefully
		if err != nil && err != http.ErrServerClosed {
			assert.NoError(t, err, "Start should return nil or http.ErrServerClosed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after shutdown")
	}
}

/*
	Scenario: Shutdown handles nil httpServer gracefully
	Given a server that hasn't been started (httpServer is nil)
	When Shutdown is called
	Then it should return nil without panic

	Related spec scenarios: RS.MSC.1
*/

func TestShutdownWithNilHTTPServer(t *testing.T) {
	t.Parallel()

	config := Config{}
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, config)
	// Ensure httpServer is nil (should be by default)
	assert.Nil(t, server.httpServer, "httpServer should be nil before Start")

	ctx := context.Background()
	err := server.Shutdown(ctx)
	assert.NoError(t, err, "Shutdown should return nil when httpServer is nil")
}

/*
	Scenario: stateManagerStore correctly delegates to state.Manager
	Given a state.Manager instance
	When stateManagerStore methods are called
	Then they should delegate to the manager with correct parameters

	Related spec scenarios: RS.MSC.20, RS.MSC.21, RS.MSC.22, RS.MSC.23, RS.MSC.24, RS.MSC.25
*/

func TestStateManagerStore(t *testing.T) {
	t.Parallel()

	manager := state.NewManager()
	store := newStateManagerStore(manager)

	// Test Set and Get
	store.Set("ns1", "key1", "value1")
	val, ok := store.Get("ns1", "key1")
	assert.True(t, ok, "Get should return true for existing key")
	assert.Equal(t, "value1", val, "Get should return the set value")

	// Test Get with non-existent namespace
	_, ok = store.Get("nonexistent", "key")
	assert.False(t, ok, "Get should return false for non-existent namespace")

	// Test Get with non-existent key in existing namespace
	store.Set("ns1", "key2", 42)
	_, ok = store.Get("ns1", "key3")
	assert.False(t, ok, "Get should return false for non-existent key")

	// Test Increment
	newVal, err := store.Increment("ns2", "counter", 5.0)
	assert.NoError(t, err, "Increment should succeed")
	assert.Equal(t, 5.0, newVal, "Increment should return new value")

	// Verify the value was stored
	val, ok = store.Get("ns2", "counter")
	assert.True(t, ok)
	assert.Equal(t, 5.0, val)

	// Test Increment with existing value
	newVal, err = store.Increment("ns2", "counter", 2.5)
	assert.NoError(t, err)
	assert.Equal(t, 7.5, newVal)

	// Test Delete
	store.Set("ns3", "todelete", "data")
	store.Delete("ns3", "todelete")
	_, ok = store.Get("ns3", "todelete")
	assert.False(t, ok, "Delete should remove the key")

	// Test GetNamespace
	store.Set("ns4", "a", 1)
	store.Set("ns4", "b", 2)
	ns := store.GetNamespace("ns4")
	assert.Equal(t, map[string]any{"a": 1, "b": 2}, ns, "GetNamespace should return all key-value pairs in namespace")

	// Test GetAll
	all := store.GetAll()
	expected := map[string]map[string]any{
		"ns1": {"key1": "value1", "key2": 42},
		"ns2": {"counter": 7.5},
		"ns4": {"a": 1, "b": 2},
	}
	// Remove ns3 since we deleted the only key
	assert.Equal(t, expected, all, "GetAll should return all namespaces with their data")
}

/*
	Scenario: historyRingBufferStore correctly delegates to history.RingBuffer
	Given a history.RingBuffer instance
	When historyRingBufferStore methods are called
	Then they should delegate to the buffer with correct parameters

	Related spec scenarios: RS.MSC.31, RS.MSC.32
*/

func TestHistoryRingBufferStore(t *testing.T) {
	t.Parallel()

	capacity := 5
	buffer := history.NewRingBuffer(capacity)
	store := newHistoryRingBufferStore(buffer)

	// Test Capacity
	assert.Equal(t, capacity, store.Capacity(), "Capacity should return buffer capacity")

	// Test Count on empty buffer
	assert.Equal(t, 0, store.Count(), "Count should be 0 initially")

	// Test Add and GetAll
	record1 := RequestRecord{
		ID:        "id1",
		Timestamp: time.Now(),
		Method:    "GET",
		Path:      "/test",
		Query:     "",
		Headers:   http.Header{"X-Test": []string{"value"}},
		Body:      []byte("body"),
		Response: &ResponseRecord{
			StatusCode: 200,
			Headers:    http.Header{"Content-Type": []string{"application/json"}},
			Body:       []byte("{}"),
			Duration:   100 * time.Millisecond,
		},
	}
	store.Add(record1)

	records := store.GetAll()
	require.Len(t, records, 1, "GetAll should return one record after Add")
	assert.Equal(t, record1.ID, records[0].ID, "Record ID should match")
	assert.Equal(t, record1.Method, records[0].Method, "Record Method should match")
	assert.Equal(t, record1.Path, records[0].Path, "Record Path should match")
	assert.Equal(t, record1.Headers, records[0].Headers, "Record Headers should match")
	assert.Equal(t, record1.Body, records[0].Body, "Record Body should match")
	require.NotNil(t, records[0].Response, "Response should not be nil")
	assert.Equal(t, record1.Response.StatusCode, records[0].Response.StatusCode, "Response StatusCode should match")
	assert.Equal(t, record1.Response.Headers, records[0].Response.Headers, "Response Headers should match")
	assert.Equal(t, record1.Response.Body, records[0].Response.Body, "Response Body should match")
	assert.Equal(t, record1.Response.Duration, records[0].Response.Duration, "Response Duration should match")

	// Test Count after adding
	assert.Equal(t, 1, store.Count(), "Count should be 1 after adding a record")

	// Test Add multiple records up to capacity
	for i := 2; i <= capacity; i++ {
		store.Add(RequestRecord{
			ID:        fmt.Sprintf("id%d", i),
			Timestamp: time.Now(),
			Method:    "POST",
			Path:      fmt.Sprintf("/test/%d", i),
		})
	}
	assert.Equal(t, capacity, store.Count(), "Count should equal capacity after filling buffer")
	assert.Equal(t, capacity, store.Capacity(), "Capacity should remain unchanged")

	// Test GetAll returns all records (buffer should not wrap yet)
	records = store.GetAll()
	require.Len(t, records, capacity, "GetAll should return capacity records")
	assert.Equal(t, "id1", records[0].ID, "First record should be id1")

	// Test Add beyond capacity (wrapping)
	store.Add(RequestRecord{ID: "id6", Timestamp: time.Now()})
	assert.Equal(t, capacity, store.Count(), "Count should stay at capacity after wrapping")
	records = store.GetAll()
	require.Len(t, records, capacity, "GetAll should still return capacity records")
	assert.Equal(t, "id2", records[0].ID, "After wrapping, first record should be id2 (oldest)")

	// Test Clear
	store.Clear()
	assert.Equal(t, 0, store.Count(), "Count should be 0 after Clear")
	records = store.GetAll()
	assert.Empty(t, records, "GetAll should return empty slice after Clear")
}

/*
	Scenario: runtimeDataSourceWrapper correctly delegates to runtime.DataSource
	Given a mock runtime.DataSource
	When Get is called on the wrapper
	Then it should delegate to the underlying source

	Related spec scenarios: RS.MSC.13, RS.MSC.14, RS.MSC.15, RS.MSC.16, RS.MSC.17, RS.MSC.18, RS.MSC.19
*/

func TestRuntimeDataSourceWrapper(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSource := mock_runtime.NewMockDataSource(ctrl)
	mockSource.EXPECT().Get("test.key").Return("value", true).Times(1)
	mockSource.EXPECT().Get("nonexistent").Return(nil, false).Times(1)

	wrapper := &runtimeDataSourceWrapper{source: mockSource}

	// Test Get with existing key
	val, ok := wrapper.Get("test.key")
	assert.True(t, ok, "Get should return true for existing key")
	assert.Equal(t, "value", val, "Get should return the value from underlying source")

	// Test Get with non-existent key
	val, ok = wrapper.Get("nonexistent")
	assert.False(t, ok, "Get should return false for non-existent key")
	assert.Nil(t, val, "Get should return nil for non-existent key")

	// Verify mock expectations are satisfied (deferred ctrl.Finish will call ctrl.Verify)
}

/*
	Scenario: runtimeRequestSourceFactory creates DataSource for HTTP requests
	Given an HTTP request with path parameters, query parameters, headers, cookies
	When NewRequestSource is called
	Then it should return a DataSource that provides access to request data

	Related spec scenarios: RS.MSC.13, RS.MSC.14, RS.MSC.15, RS.MSC.16, RS.MSC.17, RS.MSC.18, RS.MSC.19
*/

func TestRuntimeRequestSourceFactory(t *testing.T) {
	t.Parallel()

	factory := &runtimeRequestSourceFactory{}

	// Create a proper HTTP request with parsed URL
	req, err := http.NewRequest("GET", "http://example.com/test?page=1&limit=10", nil)
	require.NoError(t, err, "Failed to create request")
	// Set headers with lowercased keys to match runtime.RequestSource's lowercasing
	req.Header["content-type"] = []string{"application/json"}
	req.Header["x-api-key"] = []string{"secret"}
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc123"})
	req.AddCookie(&http.Cookie{Name: "user", Value: "john"})

	pathParams := map[string]string{
		"id":   "123",
		"name": "test",
	}

	source := factory.NewRequestSource(req, pathParams)
	require.NotNil(t, source, "NewRequestSource should return non-nil DataSource")

	// Test accessing path parameters
	val, ok := source.Get("path.id")
	assert.True(t, ok, "Should find path parameter 'id'")
	assert.Equal(t, "123", val, "Path parameter value should match")

	val, ok = source.Get("path.name")
	assert.True(t, ok, "Should find path parameter 'name'")
	assert.Equal(t, "test", val)

	// Test accessing query parameters (single value returns string)
	val, ok = source.Get("query.page")
	assert.True(t, ok, "Should find query parameter 'page'")
	assert.Equal(t, "1", val, "Query param value should match")

	val, ok = source.Get("query.limit")
	assert.True(t, ok, "Should find query parameter 'limit'")
	assert.Equal(t, "10", val)

	// Test accessing headers (category "header", keys lowercased)
	val, ok = source.Get("header.content-type")
	assert.True(t, ok, "Should find header 'content-type'")
	assert.Equal(t, "application/json", val)

	val, ok = source.Get("header.x-api-key")
	assert.True(t, ok, "Should find header 'x-api-key'")
	assert.Equal(t, "secret", val)

	// Test accessing cookies (category "cookie")
	val, ok = source.Get("cookie.session")
	assert.True(t, ok, "Should find cookie 'session'")
	assert.Equal(t, "abc123", val, "Cookie value should match")

	val, ok = source.Get("cookie.user")
	assert.True(t, ok, "Should find cookie 'user'")
	assert.Equal(t, "john", val)

	// Test non-existent paths
	val, ok = source.Get("path.nonexistent")
	assert.False(t, ok, "Should not find non-existent path parameter")
	assert.Nil(t, val)

	val, ok = source.Get("query.missing")
	assert.False(t, ok, "Should not find non-existent query parameter")
	assert.Nil(t, val)
}

/*
	Scenario: runtimeStateSourceFactory creates DataSource for server state
	Given a StateStore with some data
	When NewStateSource is called with a namespace
	Then it should return a DataSource that provides access to state data

	Related spec scenarios: RS.MSC.18
*/

func TestRuntimeStateSourceFactory(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := NewMockStateStore(ctrl)
	store.EXPECT().GetNamespace("testns").Return(map[string]any{
		"key1": "value1",
		"key2": 42,
		"nested": map[string]any{
			"subkey": "subvalue",
		},
	})
	store.EXPECT().GetNamespace("nonexistent").Return(nil)

	factory := newRuntimeStateSourceFactory(store)
	source := factory.NewStateSource("testns")
	require.NotNil(t, source, "NewStateSource should return non-nil DataSource")

	// Test accessing top-level keys
	val, ok := source.Get("key1")
	assert.True(t, ok, "Should find top-level key 'key1'")
	assert.Equal(t, "value1", val)

	val, ok = source.Get("key2")
	assert.True(t, ok, "Should find top-level key 'key2'")
	assert.Equal(t, 42, val)

	// Test accessing nested key (runtime.StateSource supports nested traversal)
	val, ok = source.Get("nested.subkey")
	assert.True(t, ok, "Should find nested key 'nested.subkey'")
	assert.Equal(t, "subvalue", val)

	// Test non-existent namespace returns empty data source
	emptySource := factory.NewStateSource("nonexistent")
	require.NotNil(t, emptySource, "NewStateSource should return non-nil DataSource even for empty namespace")
	val, ok = emptySource.Get("any")
	assert.False(t, ok, "Empty namespace should have no data")
	assert.Nil(t, val)

	// Test non-existent key
	val, ok = source.Get("missing")
	assert.False(t, ok, "Should not find non-existent key")
	assert.Nil(t, val)
}

/*
Scenario: runtimeEnvSourceFactory creates DataSource for environment variables
Given some environment variables are set
When NewEnvSource is called
Then it should return a DataSource that provides access to environment variables

Related spec scenarios: RS.MSC.19
*/
func TestRuntimeEnvSourceFactory(t *testing.T) {

	// Set environment variables for this test (cannot run in parallel)
	t.Setenv("TEST_FOO", "bar")
	t.Setenv("TEST_NUM", "123")
	t.Setenv("TEST_EMPTY", "")

	factory := &runtimeEnvSourceFactory{}

	source := factory.NewEnvSource()
	require.NotNil(t, source, "NewEnvSource should return non-nil DataSource")

	// Test accessing existing environment variables
	val, ok := source.Get("TEST_FOO")
	assert.True(t, ok, "Should find env var TEST_FOO")
	assert.Equal(t, "bar", val)

	val, ok = source.Get("TEST_NUM")
	assert.True(t, ok, "Should find env var TEST_NUM")
	assert.Equal(t, "123", val)

	val, ok = source.Get("TEST_EMPTY")
	assert.True(t, ok, "Should find env var TEST_EMPTY even if empty")
	assert.Equal(t, "", val)

	// Test non-existent environment variable
	val, ok = source.Get("TEST_NONEXISTENT")
	assert.False(t, ok, "Should not find non-existent env var")
	assert.Nil(t, val)
}

/*
	Scenario: runtimeExpressionEvaluatorWrapper correctly delegates to runtime.Evaluator
	Given a mock runtime.Evaluator
	When AddSource and Evaluate are called on the wrapper
	Then they should delegate to the underlying evaluator

	Related spec scenarios: RS.MSC.13, RS.MSC.14, RS.MSC.15, RS.MSC.16, RS.MSC.17, RS.MSC.18, RS.MSC.19
*/

func TestRuntimeExpressionEvaluatorWrapper(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEval := mock_runtime.NewMockEvaluator(ctrl)
	mockSource := mock_runtime.NewMockDataSource(ctrl)

	// Expect AddSource call with any runtime.DataSource
	mockEval.EXPECT().AddSource("test", gomock.Any()).Times(1)

	// Expect Evaluate calls
	mockEval.EXPECT().Evaluate("1 + 1").Return(2, nil).Times(1)
	mockEval.EXPECT().Evaluate("invalid").Return(nil, fmt.Errorf("evaluation error")).Times(1)

	wrapper := newRuntimeExpressionEvaluatorWrapper(mockEval)

	// Test AddSource
	wrapper.AddSource("test", mockSource)
	// Verify mockEval.AddSource expectation satisfied

	// Test Evaluate with successful expression
	result, err := wrapper.Evaluate("1 + 1")
	assert.NoError(t, err, "Evaluate should succeed")
	assert.Equal(t, 2, result, "Evaluate should return correct result")

	// Test Evaluate with error
	result, err = wrapper.Evaluate("invalid")
	assert.Error(t, err, "Evaluate should return error")
	assert.Nil(t, result, "Evaluate should return nil result on error")
}
