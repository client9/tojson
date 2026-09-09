// Package tojson converts YAML, TOML, and JSON variants to standard JSON bytes.
//
// The package converts source bytes to JSON, then they can be unmarshalled
// using standard json struct tags.
//
//	raw, err := tojson.FromYAML(src)
//	if err != nil { ... }
//	if err = json.Unmarshal(raw, &cfg); err != nil { ... }
//
// The package exposes five top-level functions:
//
//	tojson.FromJSONVariant(src []byte) ([]byte, error)
//	tojson.FromYAML(src []byte) ([]byte, error)
//	tojson.FromTOML(src []byte) ([]byte, error)
//	tojson.FromFrontMatter(src []byte) (meta []byte, body []byte, err error)
//	tojson.ToYAML(src []byte) ([]byte, error)
//
// ToYAML runs the other direction, converting JSON and the same JSON variants
// FromJSONVariant accepts into block-style YAML. Combined with the From*
// functions it converts between any two supported formats:
//
//	raw, err := tojson.FromTOML(src)
//	if err != nil { ... }
//	out, err := tojson.ToYAML(raw)
//
// ToYAMLStyle takes a YAMLStyle to set the indent width, whether multi-line
// strings use literal blocks or JSON-style quoting, and whether sequences are
// indented under their key. Style never changes the document.
//
// To indent JSON output, pass the bytes a From* function returns to
// json.Indent, which re-indents them without reflection.
//
// FromYAML intentionally supports a practical YAML subset for config files and
// front matter, not the full YAML specification.
//
// FromFrontMatter handles documents that embed metadata in a front matter block
// before the main content, as used by Hugo, Jekyll, and similar static site
// generators. It detects the format from the opening sentinel and returns the
// metadata as JSON and the body separately:
//
//	meta, body, err := tojson.FromFrontMatter(src)
//	if err != nil { ... }
//	if meta != nil {
//		if err := json.Unmarshal(meta, &article); err != nil { ... }
//	}
//	// use body (markdown, etc.)
//
// Parse failures are returned as *ParseError, which carries a 1-based line and
// column number and can be inspected with errors.As:
//
//	var pe *tojson.ParseError
//	if errors.As(err, &pe) {
//		fmt.Printf("line %d, column %d: %s\n", pe.Line, pe.Column, pe.Message)
//	}
package tojson
