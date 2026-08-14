package api

import "testing"

func TestParseIntParam(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		def  int
		max  int
		want int
	}{
		{"empty falls back to default", "", 25, 100, 25},
		{"invalid falls back to default", "abc", 25, 100, 25},
		{"zero falls back to default", "0", 25, 100, 25},
		{"negative falls back to default", "-5", 25, 100, 25},
		{"valid value passes through", "10", 25, 100, 10},
		{"clamped to max", "9999", 25, 100, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseIntParam(tc.raw, tc.def, tc.max); got != tc.want {
				t.Errorf("parseIntParam(%q, %d, %d) = %d, want %d", tc.raw, tc.def, tc.max, got, tc.want)
			}
		})
	}
}

func TestRound1(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{22.5, 22.5},
		{22.549, 22.5},
		{22.551, 22.6},
		{0, 0},
		{100, 100},
	}
	for _, tc := range cases {
		if got := round1(tc.in); got != tc.want {
			t.Errorf("round1(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
