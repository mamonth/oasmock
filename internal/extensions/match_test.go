package extensions

import (
	"testing"

	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRequestEvaluatorFromSources builds an evaluator from real data sources.
// Using the production runtime.Evaluator (rather than a mock that mirrors the
// expression string construction) keeps these tests honest: a change to how
// expressions resolve surfaces as a failed assertion, not a silently
// re-matched mock expectation.
func newRequestEvaluatorFromSources(t *testing.T, sources map[string]runtime.DataSource) runtime.Evaluator {
	t.Helper()
	eval := runtime.NewEvaluator()
	for name, source := range sources {
		eval.AddSource(name, source)
	}
	return eval
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
			eval := newRequestEvaluatorFromSources(t, tt.sources)

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
			eval := newRequestEvaluatorFromSources(t, tt.sources)

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

/*
Scenario: Detecting connection references in a match
Given a match whose condition key or string value references {$connection.*}
When MatchReferencesConnection runs
Then it reports true for key-side and value-side references and false otherwise

Related spec scenarios: RS.EXT.24, RS.EXT.27
*/
func TestMatchReferencesConnection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		match map[string]any
		want  bool
	}{
		{name: "connection id key", match: map[string]any{"{$connection.id}": "conn-1"}, want: true},
		{name: "connection header key", match: map[string]any{"{$connection.header.x-tid}": "abc"}, want: true},
		{name: "connection ref as value", match: map[string]any{"{$event.connectionId}": "{$connection.id}"}, want: true},
		{name: "empty match", match: map[string]any{}, want: false},
		{name: "event only", match: map[string]any{"{$event.name}": "orderCreated"}, want: false},
		{name: "reply path only", match: map[string]any{"{$request.query.kind}": "alerts"}, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, MatchReferencesConnection(tt.match))
		})
	}
}
