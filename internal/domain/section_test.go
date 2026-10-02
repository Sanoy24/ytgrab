package domain

import (
	"math"
	"testing"
)

func TestSection(t *testing.T) {
	for _, s := range []Section{{-1, 5}, {5, 5}, {5, 5.5}, {10, 3}, {0, 90000}, {math.NaN(), 5}} {
		if s.Valid() {
			t.Errorf("%+v should be invalid", s)
		}
	}
	for _, c := range []struct {
		s     Section
		label string
	}{
		{Section{5, 15}, "5s-15s"},
		{Section{65, 150.4}, "1m05s-2m30s"},
		{Section{3600, 3730}, "1h00m00s-1h02m10s"},
	} {
		if !c.s.Valid() || c.s.Label() != c.label {
			t.Errorf("%+v: valid %v, label %q, want %q", c.s, c.s.Valid(), c.s.Label(), c.label)
		}
	}
}
