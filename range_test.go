package httpreader

import (
	"testing"
)

func TestHttpRangeString(t *testing.T) {
	tests := []struct {
		rangeInput httpRange
		expected   string
	}{
		{httpRange{-1, 100}, "-100"},
		{httpRange{0, -1}, "0-"},
		{httpRange{0, 100}, "0-100"},
		{httpRange{10, 20}, "10-20"},
		{httpRange{5, 5}, "5-5"},
		{httpRange{100, 50}, ""},
		{httpRange{0, 0}, "0-0"},
		{httpRange{-1, -1}, ""},
	}

	for _, test := range tests {
		result := test.rangeInput.String()
		if result != test.expected {
			t.Errorf("httpRange(%v).String() = %q; expected %q", test.rangeInput, result, test.expected)
		}
	}
}
