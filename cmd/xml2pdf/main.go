// Command xml2pdf renders an XML document description into a PDF.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sandeepkumarmishra354/xml-to-pdf/pkg/xml2pdf"
)

var version = "dev"

type setFlags map[string]string

func (s setFlags) String() string { return "" }

func (s setFlags) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(k) == "" {
		return fmt.Errorf("expected key=value, got %q", v)
	}
	s[strings.TrimSpace(k)] = val
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "xml2pdf:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("xml2pdf", flag.ContinueOnError)
	out := fs.String("o", "", "output PDF file (default: input name with .pdf, \"-\" for stdout)")
	data := fs.String("data", "", "JSON file with values for ${...} references")
	baseDir := fs.String("base-dir", "", "directory for relative image paths (default: the input file's directory, or the working directory for stdin)")
	sets := setFlags{}
	fs.Var(sets, "set", "set a variable, key=value (repeatable; overrides --data and constants)")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: xml2pdf [flags] input.xml\n\nRenders an XML document to PDF. Use \"-\" as input to read stdin.\n\nFlags:\n")
		fs.PrintDefaults()
	}

	// Allow flags before and after the input file.
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if *showVersion {
		fmt.Fprintln(stdout, "xml2pdf", version)
		return nil
	}
	if len(positional) != 1 {
		fs.Usage()
		return fmt.Errorf("expected exactly one input file")
	}
	in := positional[0]

	opts := xml2pdf.Options{Vars: sets, BaseDir: *baseDir}
	if *data != "" {
		d, err := xml2pdf.LoadData(*data)
		if err != nil {
			return err
		}
		opts.Data = d
	}

	if in == "-" {
		var buf bytes.Buffer
		if err := xml2pdf.Render(stdin, &buf, opts); err != nil {
			return err
		}
		if *out == "" || *out == "-" {
			_, err := stdout.Write(buf.Bytes())
			return err
		}
		return os.WriteFile(*out, buf.Bytes(), 0o644)
	}

	if *out == "-" {
		f, err := os.Open(in)
		if err != nil {
			return err
		}
		defer f.Close()
		if opts.BaseDir == "" {
			opts.BaseDir = filepath.Dir(in)
		}
		return xml2pdf.Render(f, stdout, opts)
	}
	if *out == "" {
		*out = strings.TrimSuffix(in, filepath.Ext(in)) + ".pdf"
	}
	return xml2pdf.RenderFile(in, *out, opts)
}
