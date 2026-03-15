package runtime

import (
	"testing"
)

/*
Scenario: Benchmark evaluating simple path expression
Given an evaluator with request source containing path parameter
When Evaluate is called with simple path variable repeatedly
Then measure performance of simple variable resolution

Related spec scenarios: RS.MSC.13
*/
func BenchmarkEvaluateSimplePath(b *testing.B) {
	eval := NewEvaluator()
	eval.AddSource("request", &RequestSource{
		PathParams: map[string]string{"id": "123"},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eval.Evaluate("{$request.path.id}")
	}
}

/*
Scenario: Benchmark evaluating nested path expression
Given an evaluator with request source containing deeply nested body
When Evaluate is called with nested path variable repeatedly
Then measure performance of deep property resolution

Related spec scenarios: RS.MSC.16
*/
func BenchmarkEvaluateNestedPath(b *testing.B) {
	eval := NewEvaluator()
	eval.AddSource("request", &RequestSource{
		Body: map[string]any{
			"user": map[string]any{
				"profile": map[string]any{
					"name": "John",
				},
			},
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eval.Evaluate("{$request.body.user.profile.name}")
	}
}

/*
Scenario: Benchmark evaluating expression with default modifier
Given an evaluator with request source missing a query parameter
When Evaluate is called with default modifier repeatedly
Then measure performance of default fallback resolution

Related spec scenarios: RS.EXT.14
*/
func BenchmarkEvaluateWithDefaultModifier(b *testing.B) {
	eval := NewEvaluator()
	eval.AddSource("request", &RequestSource{
		QueryParams: map[string][]string{"other": {"value"}},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eval.Evaluate("{$request.query.missing|default:fallback}")
	}
}

/*
Scenario: Benchmark evaluating expression with getByPath modifier
Given an evaluator with state source containing nested data
When Evaluate is called with getByPath modifier repeatedly
Then measure performance of modifier‑based property resolution

Related spec scenarios: RS.EXT.15
*/
func BenchmarkEvaluateWithGetByPathModifier(b *testing.B) {
	eval := NewEvaluator()
	eval.AddSource("state", &StateSource{
		Data: map[string]any{
			"config": map[string]any{
				"nested": map[string]any{
					"value": "result",
				},
			},
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eval.Evaluate("{$state.config|getByPath:nested.value}")
	}
}

/*
Scenario: Benchmark evaluating expression with escaped dot
Given an evaluator with request source containing cookie key with dot
When Evaluate is called with escaped‑dot path repeatedly
Then measure performance of escaped dot resolution

Related spec scenarios: RS.EXT.17
*/
func BenchmarkEvaluateEscapedDot(b *testing.B) {
	eval := NewEvaluator()
	eval.AddSource("request", &RequestSource{
		Cookies: map[string]string{"dot.key": "value"},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eval.Evaluate("{$request.cookie.dot\\.key}")
	}
}

/*
Scenario: Benchmark evaluating multiple expressions across different sources
Given an evaluator with request, state, and environment sources
When Evaluate is called with multiple distinct expressions repeatedly
Then measure performance of variable resolution across multiple data sources

Related spec scenarios: RS.MSC.13, RS.MSC.14, RS.MSC.15, RS.MSC.16, RS.MSC.17, RS.MSC.18, RS.MSC.19
*/
func BenchmarkEvaluateMultipleSources(b *testing.B) {
	eval := NewEvaluator()
	eval.AddSource("request", &RequestSource{
		PathParams:  map[string]string{"id": "123"},
		QueryParams: map[string][]string{"page": {"1"}},
		Headers:     map[string][]string{"content-type": {"application/json"}},
		Cookies:     map[string]string{"session": "abc"},
		Body:        map[string]any{"action": "test"},
	})
	eval.AddSource("state", &StateSource{
		Data: map[string]any{"counter": 5},
	})
	eval.AddSource("env", &EnvSource{
		Env: map[string]string{"PORT": "8080"},
	})

	expressions := []string{
		"{$request.path.id}",
		"{$request.query.page}",
		"{$request.header.content-type}",
		"{$request.cookie.session}",
		"{$request.body.action}",
		"{$state.counter}",
		"{$env.PORT}",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, expr := range expressions {
			_, _ = eval.Evaluate(expr)
		}
	}
}
