package units

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		pct  bool
	}{
		{"12", 12, false},
		{"12pt", 12, false},
		{" 1in ", 72, false},
		{"25.4mm", 72, false},
		{"2.54cm", 72, false},
		{"50%", 50, true},
		{"-3", -3, false},
	}
	for _, c := range cases {
		l, err := Parse(c.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.in, err)
		}
		if !near(l.Value, c.want) || l.Pct != c.pct {
			t.Errorf("Parse(%q) = %+v, want %v pct=%v", c.in, l, c.want, c.pct)
		}
	}
	for _, bad := range []string{"", "abc", "10px", "mm"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) expected error", bad)
		}
	}
}

func TestResolve(t *testing.T) {
	if got := (Length{Value: 50, Pct: true}).Resolve(200); got != 100 {
		t.Errorf("got %v", got)
	}
	if got := Pt(10).Resolve(200); got != 10 {
		t.Errorf("got %v", got)
	}
}

func TestParseBox(t *testing.T) {
	cases := map[string][4]float64{
		"10":          {10, 10, 10, 10},
		"10 20":       {10, 20, 10, 20},
		"10 20 30":    {10, 20, 30, 20},
		"10 20 30 40": {10, 20, 30, 40},
	}
	for in, want := range cases {
		got, err := ParseBox(in)
		if err != nil || got != want {
			t.Errorf("ParseBox(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseBox("10%"); err == nil {
		t.Error("expected error for percentage")
	}
	if _, err := ParseBox("1 2 3 4 5"); err == nil {
		t.Error("expected error for 5 values")
	}
}
