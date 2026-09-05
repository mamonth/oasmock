package extensions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Extracting the x-mock-interval timing extension
Given a message example declaring x-mock-interval as a positive millisecond count
When the interval is extracted
Then it resolves to the declared millisecond value and marks the example periodic

Related spec scenarios: RS.EXT.22
*/
func TestValueInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ext    map[string]any
		want   int
		wantOK bool
	}{
		{name: "positive interval", ext: map[string]any{"x-mock-interval": float64(1000)}, want: 1000, wantOK: true},
		{name: "integer interval", ext: map[string]any{"x-mock-interval": 500}, want: 500, wantOK: true},
		{name: "absent interval", ext: map[string]any{}, want: 0, wantOK: false},
		{name: "zero interval invalid", ext: map[string]any{"x-mock-interval": 0}, want: 0, wantOK: false},
		{name: "negative interval invalid", ext: map[string]any{"x-mock-interval": -10}, want: 0, wantOK: false},
		{name: "fractional interval invalid", ext: map[string]any{"x-mock-interval": 1.5}, want: 0, wantOK: false},
		{name: "non-numeric invalid", ext: map[string]any{"x-mock-interval": "soon"}, want: 0, wantOK: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ev := NewExampleValue(map[string]any{}, nil, tt.ext)
			got, ok := ValueInterval(ev)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

/*
Scenario: Extracting the x-mock-delay timing extension
Given a message example declaring x-mock-delay as a millisecond count
When the delay is extracted
Then it resolves to the declared value (defaulting to zero when absent)

Related spec scenarios: RS.EXT.23
*/
func TestValueDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ext    map[string]any
		want   int
		wantOK bool
	}{
		{name: "declared delay", ext: map[string]any{"x-mock-delay": float64(150)}, want: 150, wantOK: true},
		{name: "integer delay", ext: map[string]any{"x-mock-delay": 200}, want: 200, wantOK: true},
		{name: "zero delay", ext: map[string]any{"x-mock-delay": 0}, want: 0, wantOK: true},
		{name: "absent delay", ext: map[string]any{}, want: 0, wantOK: false},
		{name: "fractional delay invalid", ext: map[string]any{"x-mock-delay": 1.5}, want: 0, wantOK: false},
		{name: "non-numeric invalid", ext: map[string]any{"x-mock-delay": "soon"}, want: 0, wantOK: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ev := NewExampleValue(map[string]any{}, nil, tt.ext)
			got, ok := ValueDelay(ev)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

/*
Scenario: Interval and delay parse from OpenAPI examples too
Given an OpenAPI example carrying x-mock-interval and x-mock-delay
When the generic extractors run
Then the values resolve identically to the map-backed accessors

Related spec scenarios: RS.EXT.22, RS.EXT.23
*/
func TestValueIntervalDelay_OpenAPI(t *testing.T) {
	t.Parallel()

	ev := exampleSource{
		ext: map[string]any{"x-mock-interval": 1000, "x-mock-delay": 150},
	}

	interval, ok := ValueInterval(ev)
	require.True(t, ok)
	assert.Equal(t, 1000, interval)

	delay, ok := ValueDelay(ev)
	require.True(t, ok)
	assert.Equal(t, 150, delay)
}

// exampleSource is a minimal ExampleValue implementation exercising the
// extractors through the interface without importing openapi3.
type exampleSource struct {
	ext map[string]any
}

func (e exampleSource) Get(key string) (any, bool) {
	v, ok := e.ext[key]
	return v, ok
}

func (e exampleSource) Payload() any            { return nil }
func (e exampleSource) Headers() map[string]any { return nil }
