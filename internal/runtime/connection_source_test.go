package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Connection context exposure for per-connection matching
Given a ConnectionSource with id, channel and upgrade-time metadata
When the source is queried by path
Then {$connection.id}, {$connection.channel}, {$connection.query.<key>} and
{$connection.header.<key>} resolve from the connection context

Related spec scenarios: RS.EXT.27
*/
func TestConnectionSource_Get(t *testing.T) {
	t.Parallel()

	src := &ConnectionSource{
		ID:      "conn-1",
		Channel: "/alerts",
		Query:   map[string][]string{"region": {"eu"}, "mode": {"a", "b"}},
		Headers: map[string][]string{"x-tenant": {"acme"}, "x-org": {"acme"}, "x-echo": {"one", "two"}},
	}

	tests := []struct {
		name string
		path string
		want any
		ok   bool
	}{
		{name: "connection id", path: "id", want: "conn-1", ok: true},
		{name: "connection channel", path: "channel", want: "/alerts", ok: true},
		{name: "query single value", path: "query.region", want: "eu", ok: true},
		{name: "query multiple values", path: "query.mode", want: []string{"a", "b"}, ok: true},
		{name: "header single value", path: "header.x-tenant", want: "acme", ok: true},
		{name: "header multiple values", path: "header.x-echo", want: []string{"one", "two"}, ok: true},
		{name: "missing query key", path: "query.missing", ok: false},
		{name: "missing header key", path: "header.missing", ok: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v, ok := src.Get(tt.path)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, v)
			}
		})
	}
}

/*
Scenario: Connection expressions evaluate through the evaluator
Given an evaluator with a connection source registered as "connection"
When an expression is evaluated
Then the connection value is returned

Related spec scenarios: RS.EXT.27
*/
func TestEvaluator_ConnectionExpression(t *testing.T) {
	t.Parallel()

	eval := NewEvaluator()
	eval.AddSource("connection", &ConnectionSource{ID: "conn-1", Channel: "/alerts"})

	val, err := eval.Evaluate("{$connection.id}")
	require.NoError(t, err)
	assert.Equal(t, "conn-1", val)
}
