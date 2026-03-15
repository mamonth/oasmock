package extensions

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mamonth/oasmock/internal/runtime"
	mock_runtime "github.com/mamonth/oasmock/mock/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockRuntimeEvaluatorFromSources creates a mock runtime.Evaluator using gomock
// that simulates evaluation of expressions based on the provided data sources.
// Currently supports only request source with query params.
func newMockRuntimeEvaluatorFromSources(t *testing.T, sources map[string]runtime.DataSource) *mock_runtime.MockEvaluator {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockEval := mock_runtime.NewMockEvaluator(ctrl)
	// Allow AddSource to be called any number of times
	mockEval.EXPECT().AddSource(gomock.Any(), gomock.Any()).AnyTimes()

	// Build expected evaluations from request sources
	for name, source := range sources {
		if req, ok := source.(*runtime.RequestSource); ok {
			for param, values := range req.QueryParams {
				if len(values) > 0 {
					expr := "{$" + name + ".query." + param + "}"
					mockEval.EXPECT().Evaluate(expr).Return(values[0], nil).AnyTimes()
				}
			}
		}
	}
	// For any other expression, return nil, nil (simulate not found)
	mockEval.EXPECT().Evaluate(gomock.Any()).Return(nil, nil).AnyTimes()

	return mockEval
}

/*
Scenario: Comparing JSON‑like values for equality
Given pairs of JSON‑compatible values (strings, numbers, booleans, null, arrays, maps)
When equality is checked with JSON‑aware rules (numeric string equals number, int equals float)
Then the result matches expected equality according to JSON semantics

Related spec scenarios: RS.EXT.1, RS.EXT.2
*/
func TestEqualJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    any
		b    any
		want bool
	}{
		{
			name: "equal strings",
			a:    "hello",
			b:    "hello",
			want: true,
		},
		{
			name: "different strings",
			a:    "hello",
			b:    "world",
			want: false,
		},
		{
			name: "equal numbers",
			a:    42.0,
			b:    42.0,
			want: true,
		},
		{
			name: "different numbers",
			a:    42.0,
			b:    43.0,
			want: false,
		},
		{
			name: "string equals number (numeric string)",
			a:    "42",
			b:    42.0,
			want: true,
		},
		{
			name: "number equals string (numeric string)",
			a:    42.0,
			b:    "42",
			want: true,
		},
		{
			name: "non-numeric string vs number",
			a:    "hello",
			b:    42.0,
			want: false,
		},
		{
			name: "int vs float",
			a:    42,
			b:    42.0,
			want: true,
		},
		{
			name: "bool true",
			a:    true,
			b:    true,
			want: true,
		},
		{
			name: "bool false vs true",
			a:    false,
			b:    true,
			want: false,
		},
		{
			name: "null values",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "array equal",
			a:    []any{1, 2, 3},
			b:    []any{1, 2, 3},
			want: true,
		},
		{
			name: "array different",
			a:    []any{1, 2, 3},
			b:    []any{1, 2},
			want: false,
		},
		{
			name: "map equal",
			a:    map[string]any{"key": "value"},
			b:    map[string]any{"key": "value"},
			want: true,
		},
		{
			name: "map different",
			a:    map[string]any{"key": "value"},
			b:    map[string]any{"key": "other"},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := equalJSON(tt.a, tt.b)
			assert.Equal(t, tt.want, got, "equalJSON(%v, %v)", tt.a, tt.b)
		})
	}
}

/*
Scenario: Evaluating literal parameter matches against request sources
Given a ParamsMatch map with literal values and runtime data sources
When EvaluateParamsMatch is called
Then it returns true when all literals match source values, false otherwise, respecting JSON equality

Related spec scenarios: RS.EXT.1
*/
func TestEvaluateParamsMatchLiteral(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pm      ParamsMatch
		sources map[string]runtime.DataSource
		want    bool
		wantErr bool
	}{
		{
			name: "single match",
			pm: ParamsMatch{
				"{$request.query.id}": "123",
			},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{
					QueryParams: map[string][]string{"id": {"123"}},
				},
			},
			want: true,
		},
		{
			name: "single mismatch",
			pm: ParamsMatch{
				"{$request.query.id}": "123",
			},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{
					QueryParams: map[string][]string{"id": {"456"}},
				},
			},
			want: false,
		},
		{
			name: "multiple conditions all match",
			pm: ParamsMatch{
				"{$request.query.id}":   "123",
				"{$request.query.name}": "test",
			},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{
					QueryParams: map[string][]string{
						"id":   {"123"},
						"name": {"test"},
					},
				},
			},
			want: true,
		},
		{
			name: "multiple conditions one mismatch",
			pm: ParamsMatch{
				"{$request.query.id}":   "123",
				"{$request.query.name}": "test",
			},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{
					QueryParams: map[string][]string{
						"id":   {"123"},
						"name": {"wrong"},
					},
				},
			},
			want: false,
		},
		{
			name: "numeric string matches number",
			pm: ParamsMatch{
				"{$request.query.id}": 42.0,
			},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{
					QueryParams: map[string][]string{"id": {"42"}},
				},
			},
			want: true,
		},
		{
			name: "non-existent query param",
			pm: ParamsMatch{
				"{$request.query.missing}": "value",
			},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{
					QueryParams: map[string][]string{},
				},
			},
			want: false,
		},
		{
			name: "empty params match",
			pm:   ParamsMatch{},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eval := newMockRuntimeEvaluatorFromSources(t, tt.sources)

			got, err := EvaluateParamsMatch(tt.pm, eval)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

/*
Scenario: Evaluating JSON schema parameter matches against request sources
Given a ParamsMatch map containing JSON schemas and runtime data sources
When EvaluateParamsMatch is called
Then it returns true when source values satisfy schemas, false otherwise, invalid schemas produce errors

Related spec scenarios: RS.EXT.2
*/
func TestEvaluateParamsMatchSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pm      ParamsMatch
		sources map[string]runtime.DataSource
		want    bool
		wantErr bool
	}{
		{
			name: "schema matches",
			pm: ParamsMatch{
				"{$request.query.id}": map[string]any{
					"type":    "string",
					"pattern": "^[0-9]{3}$",
				},
			},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{
					QueryParams: map[string][]string{"id": {"123"}},
				},
			},
			want: true,
		},
		{
			name: "schema does not match",
			pm: ParamsMatch{
				"{$request.query.id}": map[string]any{
					"type":    "string",
					"pattern": "^[0-9]{3}$",
				},
			},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{
					QueryParams: map[string][]string{"id": {"abc"}},
				},
			},
			want: false,
		},
		{
			name: "invalid schema",
			pm: ParamsMatch{
				"{$request.query.id}": map[string]any{
					"type": "invalid-type",
				},
			},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{
					QueryParams: map[string][]string{"id": {"123"}},
				},
			},
			wantErr: true,
		},
		{
			name: "mixed literal and schema",
			pm: ParamsMatch{
				"{$request.query.id}": "123",
				"{$request.query.name}": map[string]any{
					"type":      "string",
					"minLength": 3,
				},
			},
			sources: map[string]runtime.DataSource{
				"request": &runtime.RequestSource{
					QueryParams: map[string][]string{
						"id":   {"123"},
						"name": {"test"},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eval := newMockRuntimeEvaluatorFromSources(t, tt.sources)

			got, err := EvaluateParamsMatch(tt.pm, eval)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

/*
Scenario: Matching values against JSON schemas
Given a value and a JSON schema (string pattern, numeric range, array type, etc.)
When matchesJSONSchema is called
Then it returns true if the value satisfies the schema, false otherwise, invalid schemas produce errors

Related spec scenarios: RS.EXT.2
*/
func TestMatchesJSONSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		schema  map[string]any
		want    bool
		wantErr bool
	}{
		{
			name:  "string matches pattern",
			value: "123",
			schema: map[string]any{
				"type":    "string",
				"pattern": "^[0-9]{3}$",
			},
			want: true,
		},
		{
			name:  "string does not match pattern",
			value: "abc",
			schema: map[string]any{
				"type":    "string",
				"pattern": "^[0-9]{3}$",
			},
			want: false,
		},
		{
			name:  "number matches range",
			value: 42.0,
			schema: map[string]any{
				"type":    "number",
				"minimum": 0,
				"maximum": 100,
			},
			want: true,
		},
		{
			name:  "number outside range",
			value: 150.0,
			schema: map[string]any{
				"type":    "number",
				"minimum": 0,
				"maximum": 100,
			},
			want: false,
		},
		{
			name:  "array matches",
			value: []any{1, 2, 3},
			schema: map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "number"},
			},
			want: true,
		},
		{
			name:  "invalid schema type",
			value: "test",
			schema: map[string]any{
				"type": "invalid-type",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := matchesJSONSchema(tt.value, tt.schema)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
