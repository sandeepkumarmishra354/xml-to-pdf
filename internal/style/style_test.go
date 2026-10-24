package style

import "testing"

func props(t *testing.T, attrs map[string]string) Props {
	t.Helper()
	p, err := ParseProps(func(k string) (string, bool) { v, ok := attrs[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseColor(t *testing.T) {
	cases := map[string]Color{
		"#fff":         {255, 255, 255},
		"#1a2B3c":      {0x1a, 0x2b, 0x3c},
		"rgb(1, 2, 3)": {1, 2, 3},
		"Red":          {255, 0, 0},
		" lightgray ":  {211, 211, 211},
	}
	for in, want := range cases {
		got, err := ParseColor(in)
		if err != nil || got != want {
			t.Errorf("ParseColor(%q) = %v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"#12", "rgb(1,2)", "rgb(1,2,300)", "nope"} {
		if _, err := ParseColor(bad); err == nil {
			t.Errorf("ParseColor(%q) expected error", bad)
		}
	}
}

func TestResolveCascade(t *testing.T) {
	r := NewRegistry()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(r.Add(&Def{ID: "base", Props: props(t, map[string]string{"font": "times", "color": "#333"})}))
	must(r.Add(&Def{ID: "h1", Extends: "base", Props: props(t, map[string]string{"size": "20", "bold": "true", "margin-bottom": "6"})}))
	must(r.Add(&Def{ID: "red", Props: props(t, map[string]string{"color": "red"})}))
	must(r.Flatten())

	parent := Default
	parent.MarginBottom = 99 // must not inherit
	c, err := r.Resolve(parent, props(t, map[string]string{"style": "h1 red", "italic": "yes"}))
	must(err)
	want := Computed{Font: "times", Size: 20, Bold: true, Italic: true, Color: Color{255, 0, 0}, Align: "left", LineHeight: 1.2, MarginBottom: 6}
	if c != want {
		t.Errorf("got %+v\nwant %+v", c, want)
	}
	if c.FontStyle() != "BI" {
		t.Errorf("FontStyle = %q", c.FontStyle())
	}

	if _, err := r.Resolve(Default, Props{Refs: []string{"missing"}}); err == nil {
		t.Error("expected unknown style error")
	}
}

func TestFlattenErrors(t *testing.T) {
	r := NewRegistry()
	_ = r.Add(&Def{ID: "a", Extends: "b"})
	_ = r.Add(&Def{ID: "b", Extends: "a"})
	if err := r.Flatten(); err == nil {
		t.Error("expected cycle error")
	}
	r = NewRegistry()
	_ = r.Add(&Def{ID: "a", Extends: "zzz"})
	if err := r.Flatten(); err == nil {
		t.Error("expected unknown parent error")
	}
	if err := r.Add(&Def{ID: "a"}); err == nil {
		t.Error("expected duplicate error")
	}
}

func TestParsePropsErrors(t *testing.T) {
	for _, attrs := range []map[string]string{
		{"font": "comic sans"},
		{"size": "-1"},
		{"bold": "maybe"},
		{"align": "middle"},
		{"line-height": "x"},
		{"color": "#zzzzzz"},
	} {
		if _, err := ParseProps(func(k string) (string, bool) { v, ok := attrs[k]; return v, ok }); err == nil {
			t.Errorf("expected error for %v", attrs)
		}
	}
}
