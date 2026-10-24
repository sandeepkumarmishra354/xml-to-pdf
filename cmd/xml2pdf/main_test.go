package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const tmpl = `<document><page><text>${greeting}, ${who.name} (${status})</text></page></document>`

// render runs the CLI on tmpl and returns the PDF's text, or "" when
// pdftotext is unavailable (the render itself must still succeed).
func render(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "doc.xml")
	out := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(in, []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run(append([]string{in, "-o", out}, args...), strings.NewReader(stdin), &bytes.Buffer{})
	if err != nil {
		return "", err
	}
	if _, lookErr := exec.LookPath("pdftotext"); lookErr != nil {
		return "", nil
	}
	txt, err := exec.Command("pdftotext", "-enc", "UTF-8", out, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(txt)), nil
}

func TestDataSources(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "data.json")
	if err := os.WriteFile(file, []byte(`{"greeting": "Hello", "who": {"name": "File", "id": 1}, "status": "DRAFT"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"file only", "", []string{"--data", file}, "Hello, File (DRAFT)"},
		{"inline only", "", []string{"--json", `{"greeting":"Hi","who":{"name":"Inline"},"status":"NEW"}`}, "Hi, Inline (NEW)"},
		{"inline overrides file, deep merge", "", []string{"--data", file, "--json", `{"who":{"name":"Alice"},"status":"PAID"}`}, "Hello, Alice (PAID)"},
		{"later file overrides earlier inline", "", []string{"--json", `{"status":"PAID"}`, "--data", file}, "Hello, File (DRAFT)"},
		{"repeated inline", "", []string{"--data", file, "--json", `{"status":"A"}`, "--json", `{"status":"B"}`}, "Hello, File (B)"},
		{"stdin", `{"greeting":"Yo","who":{"name":"Stdin"},"status":"X"}`, []string{"--data", "-"}, "Yo, Stdin (X)"},
		{"--set beats everything", "", []string{"--data", file, "--json", `{"status":"PAID"}`, "--set", "status=SET"}, "Hello, File (SET)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := render(t, c.stdin, c.args...)
			if err != nil {
				t.Fatal(err)
			}
			if got != "" && got != c.want {
				t.Errorf("text = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDataSourceErrors(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"invalid inline", "", []string{"--json", `{bad`}, "--json #1: invalid JSON"},
		{"second inline invalid", "", []string{"--json", `{}`, "--json", `[]`}, "--json #2: data must be a JSON object"},
		{"stdin twice", `{}`, []string{"--data", "-", "--data", "-"}, "stdin can only be used once"},
		{"empty stdin", "", []string{"--data", "-"}, "--data - (stdin): no JSON data"},
		{"missing file", "", []string{"--data", "does-not-exist.json"}, "does-not-exist.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := render(t, c.stdin, c.args...)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want containing %q", err, c.want)
			}
		})
	}
}

func TestStdinInputAndDataConflict(t *testing.T) {
	err := run([]string{"-", "--data", "-"}, strings.NewReader(tmpl), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "stdin can only be used once") {
		t.Errorf("err = %v", err)
	}
}
