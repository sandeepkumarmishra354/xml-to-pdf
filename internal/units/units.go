// Package units parses length values such as "12", "10mm", "1.5in" or "50%".
// All absolute lengths are converted to PDF points (1/72 inch).
package units

import (
	"fmt"
	"strconv"
	"strings"
)

// Conversion factors to points.
const (
	PtPerIn = 72.0
	PtPerCm = 72.0 / 2.54
	PtPerMm = 72.0 / 25.4
)

// Length is an absolute length in points or a percentage of a reference length.
type Length struct {
	Value float64 // points, or percent (0-100) when Pct is set
	Pct   bool
}

// Pt returns an absolute Length.
func Pt(v float64) Length { return Length{Value: v} }

// Resolve converts the length to points, using ref as 100% for percentages.
func (l Length) Resolve(ref float64) float64 {
	if l.Pct {
		return l.Value * ref / 100
	}
	return l.Value
}

// Parse parses a length. Bare numbers are points; supported suffixes are
// pt, mm, cm, in and %.
func Parse(s string) (Length, error) {
	t := strings.TrimSpace(strings.ToLower(s))
	if t == "" {
		return Length{}, fmt.Errorf("empty length")
	}
	if strings.HasSuffix(t, "%") {
		v, err := parseNum(strings.TrimSuffix(t, "%"), s)
		return Length{Value: v, Pct: true}, err
	}
	factor := 1.0
	for _, u := range []struct {
		suffix string
		factor float64
	}{{"pt", 1}, {"mm", PtPerMm}, {"cm", PtPerCm}, {"in", PtPerIn}} {
		if strings.HasSuffix(t, u.suffix) {
			t = strings.TrimSuffix(t, u.suffix)
			factor = u.factor
			break
		}
	}
	v, err := parseNum(t, s)
	return Length{Value: v * factor}, err
}

// ParseAbs parses a length that must not be a percentage and returns points.
func ParseAbs(s string) (float64, error) {
	l, err := Parse(s)
	if err != nil {
		return 0, err
	}
	if l.Pct {
		return 0, fmt.Errorf("percentage not allowed in %q", s)
	}
	return l.Value, nil
}

// ParseBox parses 1-4 space separated absolute lengths using CSS shorthand
// order (top right bottom left) and returns [top, right, bottom, left].
func ParseBox(s string) ([4]float64, error) {
	var out [4]float64
	parts := strings.Fields(s)
	vals := make([]float64, len(parts))
	for i, p := range parts {
		v, err := ParseAbs(p)
		if err != nil {
			return out, err
		}
		vals[i] = v
	}
	switch len(vals) {
	case 1:
		out = [4]float64{vals[0], vals[0], vals[0], vals[0]}
	case 2:
		out = [4]float64{vals[0], vals[1], vals[0], vals[1]}
	case 3:
		out = [4]float64{vals[0], vals[1], vals[2], vals[1]}
	case 4:
		out = [4]float64{vals[0], vals[1], vals[2], vals[3]}
	default:
		return out, fmt.Errorf("expected 1 to 4 values, got %q", s)
	}
	return out, nil
}

func parseNum(t, orig string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid length %q", orig)
	}
	return v, nil
}
