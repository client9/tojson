package tojson

import "errors"

// FromJSONVariant converts JSON and common JSON-derived variants to standard JSON.
// It handles JSON5/HuJSON/JWCC/JSONC/HanSON features such as trailing/leading
// commas, line and block comments, unquoted keys, single-quoted and backtick
// strings, and hex literals.
func FromJSONVariant(src []byte) ([]byte, error) {
	d := &decoder{}
	d.out = &d.buf
	d.stack = d.stackbuf[:0]
	d.buf.Grow(len(src))
	err := d.Translate(src)
	return d.buf.Bytes(), err
}

// FromYAML converts a YAML subset to standard JSON.
// The output can be passed directly to encoding/json.Unmarshal using only json struct tags.
// Anchors/aliases, tags, and complex keys are not supported.
func FromYAML(src []byte) ([]byte, error) {
	return yamlConvert(src)
}

// FromTOML converts TOML to standard JSON.
// The output can be passed directly to encoding/json.Unmarshal using only json struct tags.
func FromTOML(src []byte) ([]byte, error) {
	return tomlConvert(src)
}

// MultilineStyle selects how ToYAML writes a string that contains newlines.
type MultilineStyle int

const (
	// BlockLiteral prefers a literal block scalar ("|"), and falls back to a
	// double-quoted scalar for the strings a block cannot reproduce exactly,
	// such as one with trailing whitespace on a line.
	BlockLiteral MultilineStyle = iota

	// Quoted always writes a double-quoted scalar, carrying newlines as \n
	// the way JSON does.
	Quoted
)

// YAMLStyle controls the shape of ToYAML output. Its zero value is the
// default: two spaces per level, literal blocks for multi-line strings, and
// sequences indented under their key.
//
// Style never changes the document. Whatever the settings, reading the output
// back yields the same data.
type YAMLStyle struct {
	// Indent is the number of spaces per nesting level. Zero means two.
	// YAML forbids tabs in indentation, so this is a count of spaces rather
	// than the string that encoding/json takes.
	Indent int

	// Multiline selects how a string containing newlines is written.
	Multiline MultilineStyle

	// CompactSequence puts a sequence at the indentation of the mapping key
	// that introduces it, rather than one level deeper:
	//
	//	tags:          tags:
	//	  - a            vs   - a
	//
	// It applies only to a sequence that is a mapping value. A sequence
	// nested directly in another sequence always gets its own indented lines,
	// where the compact form would read as a sibling item.
	CompactSequence bool
}

var (
	errNegativeIndent   = errors.New("YAMLStyle.Indent must not be negative")
	errUnknownMultiline = errors.New("YAMLStyle.Multiline is not a known style")
)

// ToYAML converts JSON, and the same JSON variants FromJSONVariant accepts,
// to block-style YAML with the default style. Strings are written as plain
// scalars where that round-trips, as literal blocks ("|") where they contain
// newlines, and as double-quoted scalars otherwise. Numbers pass through
// without evaluation.
//
// An object key longer than 1024 characters returns a ParseError: YAML bounds
// a mapping key written without the "? " indicator at that length, and the
// explicit form is not something FromYAML reads back.
func ToYAML(src []byte) ([]byte, error) {
	return yamlEncode(src, YAMLStyle{})
}

// ToYAMLStyle is ToYAML with the output shape given by style.
// To indent JSON output instead, pass the bytes any From* function returns to
// json.Indent, which re-indents them without reflection.
func ToYAMLStyle(src []byte, style YAMLStyle) ([]byte, error) {
	return yamlEncode(src, style)
}
