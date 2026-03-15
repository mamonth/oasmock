package extensions

import (
	"testing"

	"github.com/mamonth/oasmock/internal/runtime"
)

var _ runtime.DataSource = (*runtime.RequestSource)(nil)

type benchmarkEvaluator struct {
	evalMap map[string]any
	errMap  map[string]error
}

func (b *benchmarkEvaluator) AddSource(name string, source runtime.DataSource) {}

func (b *benchmarkEvaluator) Evaluate(expr string) (any, error) {
	if err, ok := b.errMap[expr]; ok {
		return nil, err
	}
	if val, ok := b.evalMap[expr]; ok {
		return val, nil
	}
	return nil, nil
}

func newMockEvaluatorForBenchmark() *benchmarkEvaluator {
	return &benchmarkEvaluator{
		evalMap: map[string]any{
			"{$request.query.id}": "123",
		},
		errMap: map[string]error{},
	}
}

/*
Scenario: Benchmark evaluating params match with literal value
Given a params match with literal value and evaluator with matching request source
When EvaluateParamsMatch is called repeatedly
Then measure performance of literal match evaluation

Related spec scenarios: RS.EXT.1
*/
func BenchmarkEvaluateParamsMatchLiteral(b *testing.B) {
	pm := ParamsMatch{
		"{$request.query.id}": "123",
	}
	eval := newMockEvaluatorForBenchmark()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EvaluateParamsMatch(pm, eval) //nolint:errcheck
	}
}

/*
Scenario: Benchmark evaluating params match with JSON schema
Given a params match with JSON schema and evaluator with matching request source
When EvaluateParamsMatch is called repeatedly
Then measure performance of schema‑based match evaluation

Related spec scenarios: RS.EXT.2
*/
func BenchmarkEvaluateParamsMatchSchema(b *testing.B) {
	pm := ParamsMatch{
		"{$request.query.id}": map[string]any{
			"type":    "string",
			"pattern": "^[0-9]{3}$",
		},
	}
	eval := newMockEvaluatorForBenchmark()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EvaluateParamsMatch(pm, eval) //nolint:errcheck
	}
}

/*
Scenario: Benchmark matching simple JSON schema
Given a simple string‑type JSON schema and matching value
When matchesJSONSchema is called repeatedly
Then measure performance of simple schema validation

Related spec scenarios: RS.EXT.2
*/
func BenchmarkMatchesJSONSchemaSimple(b *testing.B) {
	schema := map[string]any{
		"type":    "string",
		"pattern": "^[0-9]{3}$",
	}
	value := "123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matchesJSONSchema(value, schema) //nolint:errcheck
	}
}

/*
Scenario: Benchmark matching complex JSON schema
Given a complex object‑type JSON schema and matching value
When matchesJSONSchema is called repeatedly
Then measure performance of complex schema validation

Related spec scenarios: RS.EXT.2
*/
func BenchmarkMatchesJSONSchemaComplex(b *testing.B) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":    "integer",
				"minimum": 1,
			},
			"name": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
		"required": []any{"id", "name"},
	}
	value := map[string]any{
		"id":   123,
		"name": "test",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matchesJSONSchema(value, schema) //nolint:errcheck
	}
}
