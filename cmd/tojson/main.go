// tojson converts YAML/TOML/JSON variants and front matter from a file or
// stdin to JSON on stdout.
//
// Usage:
//
//	tojson file.yaml          # format inferred from extension
//	tojson file.md            # front matter extracted, meta JSON printed
//	cat file.yaml | tojson -f yaml
//	tojson -pretty file.yaml  # pretty-printed JSON
//	tojson -compact file.yaml # explicit compact JSON
//	tojson -raw file.yaml     # raw output from conversion, no post-processing
//
// To convert to YAML instead, pipe the JSON into the toyaml command from
// github.com/client9/toyaml.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/client9/tojson"
)

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "tojson: "+format+"\n", args...)
	os.Exit(1)
}

func convert(format string, input []byte) ([]byte, error) {
	switch format {
	case "yaml", "yml":
		return tojson.FromYAML(input)
	case "toml":
		return tojson.FromTOML(input)
	case "json5", "json", "jsonc", "hjson", "hson":
		return tojson.FromJSONVariant(input)
	case "md", "markdown", "frontmatter":
		meta, _, err := tojson.FromFrontMatter(input)
		if err != nil {
			return nil, err
		}
		if meta == nil {
			meta = []byte("{}")
		}
		return meta, nil
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
}

// prettyJSON re-indents compact JSON. json.Indent works on the bytes, so no
// reflection is involved, the document keeps its original key order, and
// string contents are left exactly as the converter wrote them.
func prettyJSON(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return src, nil
	}
	var buf bytes.Buffer
	buf.Grow(len(src) + len(src)/4)
	if err := json.Indent(&buf, src, "", "  "); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeOutput(w io.Writer, out []byte, raw bool) error {
	if _, err := w.Write(out); err != nil {
		return err
	}
	if raw {
		return nil
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func main() {
	pretty := flag.Bool("pretty", false, "pretty-print JSON output")
	compact := flag.Bool("compact", false, "compact JSON output (default)")
	raw := flag.Bool("raw", false, "raw output from conversion, no post-processing")
	format := flag.String("f", "", "input format: yaml, toml, json5 (required when reading stdin)")
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *version {
		v := "(devel)"
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			v = info.Main.Version
		}
		fmt.Println(v)
		os.Exit(0)
	}

	modeCount := 0
	for _, b := range []bool{*pretty, *compact, *raw} {
		if b {
			modeCount++
		}
	}
	if modeCount > 1 {
		fatalf("-pretty, -compact, and -raw are mutually exclusive")
	}

	var input []byte
	var err error
	var fmt_ string

	switch flag.NArg() {
	case 0:
		if *format == "" {
			fatalf("-f <format> is required when reading from stdin")
		}
		fmt_ = strings.ToLower(*format)
		input, err = io.ReadAll(os.Stdin)
		if err != nil {
			fatalf("reading stdin: %v", err)
		}
	case 1:
		filename := flag.Arg(0)
		input, err = os.ReadFile(filename)
		if err != nil {
			fatalf("%v", err)
		}
		if *format != "" {
			fmt_ = strings.ToLower(*format)
		} else {
			ext := strings.TrimPrefix(filepath.Ext(filename), ".")
			fmt_ = strings.ToLower(ext)
		}
	default:
		fatalf("usage: tojson [-pretty|-compact|-raw] [-f format] [file]")
	}

	out, err := convert(fmt_, input)
	if err != nil {
		fatalf("%v", err)
	}

	if *pretty {
		out, err = prettyJSON(out)
		if err != nil {
			fatalf("indenting: %v", err)
		}
	}

	if err := writeOutput(os.Stdout, out, *raw); err != nil {
		fatalf("writing stdout: %v", err)
	}
}
