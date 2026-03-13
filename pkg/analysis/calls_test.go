package analysis

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArgsMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params []Interval
		args   []Interval
		want   bool
	}{
		// Matching cases
		{"identical single", []Interval{NewInterval(1, 5)}, []Interval{NewInterval(1, 5)}, true},
		{"identical multi", []Interval{NewInterval(1, 5), NewInterval(-3, 3)}, []Interval{NewInterval(1, 5), NewInterval(-3, 3)}, true},
		{"both empty", []Interval{}, []Interval{}, true},
		{"both nil", nil, nil, true},
		{"both top", []Interval{Top()}, []Interval{Top()}, true},
		{"both bottom", []Interval{Bottom()}, []Interval{Bottom()}, true},

		// Non-matching cases
		{"different length", []Interval{NewInterval(1, 5)}, []Interval{NewInterval(1, 5), NewInterval(2, 3)}, false},
		{"nil vs non-nil", nil, []Interval{NewInterval(1, 5)}, false},
		{"non-nil vs nil", []Interval{NewInterval(1, 5)}, nil, false},
		{"different intervals", []Interval{NewInterval(1, 5)}, []Interval{NewInterval(1, 6)}, false},
		{"top vs concrete", []Interval{Top()}, []Interval{NewInterval(1, 5)}, false},
		{"concrete vs bottom", []Interval{NewInterval(1, 5)}, []Interval{Bottom()}, false},
		{"first matches second differs", []Interval{NewInterval(1, 5), NewInterval(10, 20)}, []Interval{NewInterval(1, 5), NewInterval(10, 21)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &FunctionSummary{Params: tt.params}
			require.Equal(t, tt.want, s.ArgsMatch(tt.args))
		})
	}
}
