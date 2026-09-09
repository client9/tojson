package tojson

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"strings"
	"testing"
)

//go:embed testdata/frontmatter1.yml
var frontmatter1YAML string

func BenchmarkFromYAML(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		raw, err := FromYAML([]byte(frontmatter1YAML))
		if err != nil {
			b.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			b.Fatal(err)
		}
	}
}

// roundtripYAML checks that FromYAML produces valid JSON matching wantJSON.
func roundtripYAML(t *testing.T, yaml, wantJSON string) {
	t.Helper()
	got, err := FromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("FromYAML error: %v", err)
	}
	var v any
	if err := json.Unmarshal(got, &v); err != nil {
		t.Fatalf("invalid JSON %q: %v", got, err)
	}
	gotNorm, _ := json.Marshal(v)
	var wantV any
	if err := json.Unmarshal([]byte(wantJSON), &wantV); err != nil {
		t.Fatalf("bad wantJSON %q: %v", wantJSON, err)
	}
	wantNorm, _ := json.Marshal(wantV)
	if string(gotNorm) != string(wantNorm) {
		t.Errorf("\ninput:  %s\ngot:    %s\nwant:   %s", yaml, gotNorm, wantNorm)
	}
}

func TestYAMLScalars(t *testing.T) {
	roundtripYAML(t, `hello`, `"hello"`)
	roundtripYAML(t, `"hello world"`, `"hello world"`)
	roundtripYAML(t, `'it''s fine'`, `"it's fine"`)
	roundtripYAML(t, `null`, `null`)
	if yamlTildeNull {
		roundtripYAML(t, `~`, `null`)
	} else {
		roundtripYAML(t, `~`, `"~"`)
	}
	roundtripYAML(t, `true`, `true`)
	roundtripYAML(t, `false`, `false`)
	if yamlBoolAliases {
		roundtripYAML(t, `yes`, `true`)
		roundtripYAML(t, `no`, `false`)
	} else {
		roundtripYAML(t, `yes`, `"yes"`)
		roundtripYAML(t, `no`, `"no"`)
	}
	roundtripYAML(t, `42`, `42`)
	roundtripYAML(t, `3.14`, `3.14`)
	roundtripYAML(t, `-7`, `-7`)
	roundtripYAML(t, `1.5e10`, `1.5e10`)
}

func TestYAMLNumberNormalization(t *testing.T) {
	// leading + stripped
	roundtripYAML(t, `+42`, `42`)
	roundtripYAML(t, `+3.14`, `3.14`)
	// leading dot → 0.
	roundtripYAML(t, `.5`, `0.5`)
	roundtripYAML(t, `+.5`, `0.5`)
	roundtripYAML(t, `-.5`, `-0.5`)
	// trailing dot → .0
	roundtripYAML(t, `1.`, `1.0`)
	roundtripYAML(t, `-1.`, `-1.0`)
	// scientific notation: sign optional
	roundtripYAML(t, `1.5e4`, `1.5e4`)
	roundtripYAML(t, `1.5e+4`, `1.5e+4`)
	roundtripYAML(t, `1.5e-4`, `1.5e-4`)
	roundtripYAML(t, `.5e4`, `0.5e4`)
	// signed integers
	roundtripYAML(t, `-1`, `-1`)
	roundtripYAML(t, `+1`, `1`)
	// leading zeros → string (not a number)
	roundtripYAML(t, `0`, `0`)
	roundtripYAML(t, `+0`, `0`)
	roundtripYAML(t, `-0`, `-0`)
	roundtripYAML(t, `00`, `"00"`)
	roundtripYAML(t, `01`, `"01"`)
	roundtripYAML(t, `0012314`, `"0012314"`)
	// leading zero on float is fine
	roundtripYAML(t, `0.5`, `0.5`)
	// large integer exceeding uint64 max — passes through as JSON number
	roundtripYAML(t, `99999999999999999999999999999`, `99999999999999999999999999999`)
}

func TestYAMLSimpleMapping(t *testing.T) {
	roundtripYAML(t, `
name: Alice
age: 30
active: true
`, `{"name":"Alice","age":30,"active":true}`)
}

func TestYAMLSimpleSequence(t *testing.T) {
	roundtripYAML(t, `
- rigid
- better for data interchange
`, `["rigid","better for data interchange"]`)
}

func TestYAMLMappingWithSequence(t *testing.T) {
	roundtripYAML(t, `
sample:
  - rigid
  - better for data interchange
`, `{"sample":["rigid","better for data interchange"]}`)
}

func TestYAMLCompactSequence(t *testing.T) {
	// Block sequence at the same indent as the parent mapping key (YAML compact notation).
	roundtripYAML(t, `
genres:
- mystery
- romance
tags:
- red
- blue
`, `{"genres":["mystery","romance"],"tags":["red","blue"]}`)

	// Mixed: some keys with indented sequences, some compact.
	roundtripYAML(t, `
a:
- 1
- 2
b:
  - 3
  - 4
`, `{"a":[1,2],"b":[3,4]}`)
}

func TestYAMLNestedMapping(t *testing.T) {
	roundtripYAML(t, `
person:
  name: Bob
  address:
    city: "New York"
    zip: "10001"
`, `{"person":{"name":"Bob","address":{"city":"New York","zip":"10001"}}}`)
}

func TestYAMLSequenceOfMappings(t *testing.T) {
	roundtripYAML(t, `
- name: Alice
  age: 30
- name: Bob
  age: 25
`, `[{"name":"Alice","age":30},{"name":"Bob","age":25}]`)
}

func TestYAMLComments(t *testing.T) {
	roundtripYAML(t, `
# top-level comment
name: Alice  # inline comment
age: 30
`, `{"name":"Alice","age":30}`)
}

func TestYAMLFrontmatter(t *testing.T) {
	roundtripYAML(t, `
---
title: Hello
tags:
  - go
  - yaml
`, `{"title":"Hello","tags":["go","yaml"]}`)
}

func TestYAMLQuotedKeys(t *testing.T) {
	roundtripYAML(t, `
"key with spaces": value
'another key': 42
`, `{"key with spaces":"value","another key":42}`)
}

func TestYAMLNullValues(t *testing.T) {
	bVal := `"~"`
	if yamlTildeNull {
		bVal = `null`
	}
	roundtripYAML(t, `
a: null
b: ~
c:
`, `{"a":null,"b":`+bVal+`,"c":null}`)
}

func TestYAMLMixedNested(t *testing.T) {
	roundtripYAML(t, `
title: My Post
tags:
  - go
  - programming
meta:
  draft: false
  views: 0
`, `{"title":"My Post","tags":["go","programming"],"meta":{"draft":false,"views":0}}`)
}

func yamlJSONPassthrough(t *testing.T, input string) {
	t.Helper()
	got, err := FromYAML([]byte(input))
	if err != nil {
		t.Fatalf("FromYAML error: %v", err)
	}
	var gotV, wantV any
	if err := json.Unmarshal(got, &gotV); err != nil {
		t.Fatalf("output is not valid JSON %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(input), &wantV); err != nil {
		t.Fatalf("input is not valid JSON: %v", err)
	}
	gotNorm, _ := json.Marshal(gotV)
	wantNorm, _ := json.Marshal(wantV)
	if string(gotNorm) != string(wantNorm) {
		t.Errorf("\ninput: %s\ngot:   %s\nwant:  %s", input, gotNorm, wantNorm)
	}
}

func TestYAMLJSONPassthrough(t *testing.T) {
	yamlJSONPassthrough(t, `null`)
	yamlJSONPassthrough(t, `true`)
	yamlJSONPassthrough(t, `false`)
	yamlJSONPassthrough(t, `42`)
	yamlJSONPassthrough(t, `-7`)
	yamlJSONPassthrough(t, `3.14`)
	yamlJSONPassthrough(t, `1.5e10`)
	yamlJSONPassthrough(t, `"hello"`)
	yamlJSONPassthrough(t, `"line1\nline2\ttabbed"`)
	yamlJSONPassthrough(t, `"unicode \u0041"`)
	yamlJSONPassthrough(t, `{}`)
	yamlJSONPassthrough(t, `{"a":1}`)
	yamlJSONPassthrough(t, `{"a":1,"b":"hello","c":true,"d":null}`)
	yamlJSONPassthrough(t, `{"key with spaces":"value"}`)
	yamlJSONPassthrough(t, `[]`)
	yamlJSONPassthrough(t, `[1,2,3]`)
	yamlJSONPassthrough(t, `[true,false,null]`)
	yamlJSONPassthrough(t, `["a","b","c"]`)
	yamlJSONPassthrough(t, `{"a":{"b":{"c":42}}}`)
	yamlJSONPassthrough(t, `[[1,2],[3,4]]`)
	yamlJSONPassthrough(t, `{"nums":[1,2,3],"obj":{"x":true}}`)
	yamlJSONPassthrough(t, `[{"name":"Alice","age":30},{"name":"Bob","age":25}]`)
}

func TestYAMLFlowMapping(t *testing.T) {
	roundtripYAML(t, `{a: 1, b: hello}`, `{"a":1,"b":"hello"}`)
	roundtripYAML(t, `key: {x: true, y: null}`, `{"key":{"x":true,"y":null}}`)
	roundtripYAML(t, `{"key one": "val two"}`, `{"key one":"val two"}`)
	roundtripYAML(t, `{a: {b: {c: 42}}}`, `{"a":{"b":{"c":42}}}`)
	roundtripYAML(t, `{}`, `{}`)
	roundtripYAML(t, `
name: Alice
tags: {go: true, yaml: false}
`, `{"name":"Alice","tags":{"go":true,"yaml":false}}`)
	// single-quoted value with '' escape inside a flow mapping (exercises flowDepth i++ branch)
	roundtripYAML(t, `{key: 'it''s fine'}`, `{"key":"it's fine"}`)
}

func TestYAMLFlowSequence(t *testing.T) {
	roundtripYAML(t, `[1, 2, 3]`, `[1,2,3]`)
	roundtripYAML(t, `nums: [1, 2, 3]`, `{"nums":[1,2,3]}`)
	roundtripYAML(t, `[true, null, "hello", 3.14]`, `[true,null,"hello",3.14]`)
	roundtripYAML(t, `[[1, 2], [3, 4]]`, `[[1,2],[3,4]]`)
	roundtripYAML(t, `[]`, `[]`)
	roundtripYAML(t, `[{a: 1}, {b: 2}]`, `[{"a":1},{"b":2}]`)
	roundtripYAML(t, `
- [1, 2]
- [3, 4]
`, `[[1,2],[3,4]]`)
}

func TestYAMLFlowMultiLine(t *testing.T) {
	roundtripYAML(t, "key: {a: 1,\n      b: 2}", `{"key":{"a":1,"b":2}}`)
	roundtripYAML(t, "nums: [1,\n       2,\n       3]", `{"nums":[1,2,3]}`)
}

func TestYAMLParseError(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		line   int
		column int
	}{
		// mapping value: unterminated string
		{"mapping unterminated line 1", `key: "unterminated`, 1, 6},
		{"mapping unterminated line 3", "a: 1\nb: 2\nc: \"bad", 3, 4},
		// sequence item: unterminated string (after "- ")
		{"sequence unterminated", `- "bad`, 1, 3},
		// nested mapping value
		{"nested mapping unterminated", "foo:\n  bar: \"bad", 2, 8},
		// flow value: unterminated mapping
		{"flow unterminated mapping", "key: {a: 1", 1, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FromYAML([]byte(tc.input))
			pe := requireParseError(t, err)
			if pe.Line != tc.line {
				t.Errorf("expected line %d, got %d (msg: %s)", tc.line, pe.Line, pe.Message)
			}
			if pe.Column != tc.column {
				t.Errorf("expected column %d, got %d (msg: %s)", tc.column, pe.Column, pe.Message)
			}
		})
	}
}

func TestYAMLEscapes(t *testing.T) {
	roundtripYAML(t, `msg: "line1\nline2\ttabbed"`, `{"msg":"line1\nline2\ttabbed"}`)
}

func TestYAMLLiteralBlockScalar(t *testing.T) {
	roundtripYAML(t, `
key: |
  line one
  line two
`, `{"key":"line one\nline two\n"}`)

	roundtripYAML(t, `
key: |-
  line one
  line two
`, `{"key":"line one\nline two"}`)

	roundtripYAML(t, "key: |+\n  line one\n  line two\n\n", `{"key":"line one\nline two\n\n"}`)

	roundtripYAML(t, `
a: |
  hello
  world
b: 42
`, `{"a":"hello\nworld\n","b":42}`)

	roundtripYAML(t, `
outer:
  inner: |
    indented
    content
`, `{"outer":{"inner":"indented\ncontent\n"}}`)

	roundtripYAML(t, `
- |
  first
- |
  second
`, `["first\n","second\n"]`)
}

// Block scalars as values of a mapping key nested inside a sequence item
// ("- key: v" / "  value: |-"), which is parsed by parseInlineMap.
func TestYAMLBlockScalarInSequenceItemMap(t *testing.T) {
	roundtripYAML(t, `
- key: a
  value: |-
    line one
    line two
  after: 1
`, `[{"key":"a","value":"line one\nline two","after":1}]`)

	roundtripYAML(t, `
- key: a
  value: |
    line one
  after: 1
`, `[{"key":"a","value":"line one\n","after":1}]`)

	roundtripYAML(t, `
- key: a
  value: >-
    foo
    bar
`, `[{"key":"a","value":"foo bar"}]`)

	// block scalar as the first key of the inline map
	roundtripYAML(t, `
- value: |-
    only
`, `[{"value":"only"}]`)

	// indented content that itself looks like YAML structure
	roundtripYAML(t, `
footnotes:
  - key: robert1778-baptism
    value: |-
      $link[b-1778 "Robert Galbreath"]{
          Robert lawful son to Samuel Galbraeth + Janet McNair
      }
`, `{"footnotes":[{"key":"robert1778-baptism","value":"$link[b-1778 \"Robert Galbreath\"]{\n    Robert lawful son to Samuel Galbraeth + Janet McNair\n}"}]}`)
}

func TestYAMLFoldedBlockScalar(t *testing.T) {
	roundtripYAML(t, `
key: >
  foo bar
  baz
`, `{"key":"foo bar baz\n"}`)

	roundtripYAML(t, `
key: >
  paragraph one

  paragraph two
`, `{"key":"paragraph one\nparagraph two\n"}`)

	roundtripYAML(t, `
key: >-
  foo
  bar
`, `{"key":"foo bar"}`)
}

// Explicit indentation indicators (YAML 1.2 section 8.1.1.1). The indicator
// counts from the parent node's indentation, and lets content keep leading
// whitespace that auto-detection would swallow.
func TestYAMLBlockScalarExplicitIndent(t *testing.T) {
	roundtripYAML(t, "k: |2\n    a\n  b\n", `{"k":"  a\nb\n"}`)
	roundtripYAML(t, "k: |1\n  a\n", `{"k":" a\n"}`)
	roundtripYAML(t, "k: >2\n    a\n  b\n", `{"k":"  a\nb\n"}`)

	// both orderings of the two indicators
	roundtripYAML(t, "k: |-2\n    a\n", `{"k":"  a"}`)
	roundtripYAML(t, "k: |2-\n    a\n", `{"k":"  a"}`)

	// relative to the parent node, not the document
	roundtripYAML(t, "a:\n  k: |2\n      x\n    y\n", `{"a":{"k":"  x\ny\n"}}`)
	roundtripYAML(t, "a:\n  k: |1\n     x\n", `{"a":{"k":"  x\n"}}`)
}

// The indicator exists for one situation: content whose first line begins with
// a space. Auto-detection reads that line's indentation as the block's, so the
// leading spaces are lost and a later, shallower line falls outside the block.
func TestYAMLBlockScalarIndicatorMotivation(t *testing.T) {
	// without the indicator the second line belongs to nothing
	if _, err := FromYAML([]byte("k: |\n      indented\n  flush\n")); err == nil {
		t.Error("auto-detected block with a shallower later line: want error")
	}
	// with it, the leading spaces are data
	roundtripYAML(t, "k: |2\n      indented\n  flush\n",
		`{"k":"    indented\nflush\n"}`)

	// an indicator deeper than the content leaves the block empty, so the
	// content line belongs to nothing
	for _, in := range []string{"k: |4\n  a\n", "k: |4\n  a\nj: 1\n"} {
		if _, err := FromYAML([]byte(in)); err == nil {
			t.Errorf("FromYAML(%q): want error", in)
		}
	}
}

func TestYAMLBlockScalarIndicatorDigits(t *testing.T) {
	cases := []struct{ in, want string }{
		{"k: |1\n  a\n", `{"k":" a\n"}`},
		{"k: |2\n   a\n", `{"k":" a\n"}`},
		{"k: |3\n     a\n", `{"k":"  a\n"}`},
		{"k: |5\n       a\n", `{"k":"  a\n"}`},
		{"k: |9\n           a\n", `{"k":"  a\n"}`},
	}
	for _, tc := range cases {
		roundtripYAML(t, tc.in, tc.want)
	}
}

// The indicator combines with every chomping mode, in either order, in both
// block styles.
func TestYAMLBlockScalarIndicatorWithChomping(t *testing.T) {
	cases := []struct{ in, want string }{
		{"k: |2\n    a\n", `{"k":"  a\n"}`},
		{"k: |2-\n    a\n", `{"k":"  a"}`},
		{"k: |-2\n    a\n", `{"k":"  a"}`},
		{"k: |2+\n    a\n\n", `{"k":"  a\n\n"}`},
		{"k: |+2\n    a\n\n", `{"k":"  a\n\n"}`},
		{"k: >2\n    a\n", `{"k":"  a\n"}`},
		{"k: >2-\n    a\n", `{"k":"  a"}`},
		{"k: >-2\n    a\n", `{"k":"  a"}`},
	}
	for _, tc := range cases {
		roundtripYAML(t, tc.in, tc.want)
	}
}

// The indicator works wherever a block scalar can appear.
func TestYAMLBlockScalarIndicatorPositions(t *testing.T) {
	// sequence item
	roundtripYAML(t, "- |2\n    x\n  y\n", `["  x\ny\n"]`)
	// mapping opened on a sequence item line
	roundtripYAML(t, "- k: |2\n      x\n    y\n", `[{"k":"  x\ny\n"}]`)
	// nested mapping
	roundtripYAML(t, "a:\n  b:\n    k: |2\n        x\n      y\n",
		`{"a":{"b":{"k":"  x\ny\n"}}}`)
	// blank lines inside an indicated block
	roundtripYAML(t, "k: |2\n    a\n\n    b\n", `{"k":"  a\n\n  b\n"}`)
	roundtripYAML(t, "k: |2\n\n    a\n", `{"k":"\n  a\n"}`)
}

// A block scalar that is the whole document has no parent node to count from,
// and the indicator is taken as the content's own column: "|2" puts it at
// column two. Checked against gopkg.in/yaml.v3, which reads these the same way.
func TestYAMLBlockScalarIndicatorAtRoot(t *testing.T) {
	roundtripYAML(t, "|2\n    a\n  b\n", `"  a\nb\n"`)
	roundtripYAML(t, "|1\n  a\n", `" a\n"`)
	roundtripYAML(t, "|2\n  a\n", `"a\n"`)
}

// A header that is not a well-formed block scalar indicator is not a block
// scalar. The value is read as a plain scalar instead, which is lenient: YAML
// itself does not allow a plain scalar to open with "|" or ">".
func TestYAMLBlockScalarMalformedHeader(t *testing.T) {
	cases := []struct{ in, want string }{
		{"k: |0\n  a\n", `{"k":"|0 a"}`},   // zero is not a valid indicator
		{"k: |10\n  a\n", `{"k":"|10 a"}`}, // the indicator is one digit
		{"k: |22\n  a\n", `{"k":"|22 a"}`}, // one indentation indicator only
		{"k: |--\n  a\n", `{"k":"|-- a"}`}, // one chomping indicator only
		{"k: |2-3\n  a\n", `{"k":"|2-3 a"}`},
		{"k: |-2-\n  a\n", `{"k":"|-2- a"}`},
	}
	for _, tc := range cases {
		roundtripYAML(t, tc.in, tc.want)
	}
}

func TestDetectBlockScalar(t *testing.T) {
	cases := []struct {
		in     string
		style  byte
		chomp  byte
		indent int
		ok     bool
	}{
		{"|", '|', 0, 0, true},
		{">", '>', 0, 0, true},
		{"|-", '|', '-', 0, true},
		{"|+", '|', '+', 0, true},
		{"|2", '|', 0, 2, true},
		{"|9", '|', 0, 9, true},
		{"|2-", '|', '-', 2, true},
		{"|-2", '|', '-', 2, true},
		{"|2+", '|', '+', 2, true},
		{"|+2", '|', '+', 2, true},
		{">2-", '>', '-', 2, true},
		{"  |2  ", '|', 0, 2, true}, // surrounding space is trimmed
		{"|2\t", '|', 0, 2, true},

		// not block scalar headers
		{"|0", 0, 0, 0, false},
		{"|10", 0, 0, 0, false},
		{"|22", 0, 0, 0, false},
		{"|--", 0, 0, 0, false},
		{"|2-3", 0, 0, 0, false},
		{"|-2-", 0, 0, 0, false},
		{"|x", 0, 0, 0, false},
		{"|2 x", 0, 0, 0, false},
		{"", 0, 0, 0, false},
		{"a", 0, 0, 0, false},
		{"plain value", 0, 0, 0, false},
	}
	for _, tc := range cases {
		style, chomp, indent, ok := detectBlockScalar([]byte(tc.in))
		if ok != tc.ok || style != tc.style || chomp != tc.chomp || indent != tc.indent {
			t.Errorf("detectBlockScalar(%q) = %q, %q, %d, %v; want %q, %q, %d, %v",
				tc.in, style, chomp, indent, ok, tc.style, tc.chomp, tc.indent, tc.ok)
		}
	}
}

// Leading empty lines are content, not padding to be skipped.
func TestYAMLBlockScalarLeadingEmptyLines(t *testing.T) {
	roundtripYAML(t, "k: |\n\n  a\n", `{"k":"\na\n"}`)
	roundtripYAML(t, "k: |\n\n\n  a\n", `{"k":"\n\na\n"}`)
	roundtripYAML(t, "k: >\n\n  a\n", `{"k":"\na\n"}`)
	roundtripYAML(t, "k: |-\n\n  a\n\n", `{"k":"\na"}`)
	roundtripYAML(t, "k: |+\n\n  a\n", `{"k":"\na\n"}`)

	// a block of nothing but empty lines
	roundtripYAML(t, "k: |\n\n\nj: 1\n", `{"k":"","j":1}`)
	roundtripYAML(t, "k: |+\n\n\nj: 1\n", `{"k":"\n\n","j":1}`)
}

// Trailing whitespace on a content line is preserved.
func TestYAMLBlockScalarTrailingWhitespace(t *testing.T) {
	roundtripYAML(t, "k: |\n  a   \n  b\n", `{"k":"a   \nb\n"}`)
	roundtripYAML(t, "k: |-\n  a\t\n", `{"k":"a\t"}`)
	roundtripYAML(t, "k: >\n  a   \n  b\n", `{"k":"a    b\n"}`)

	// an all-whitespace line is empty if it stops at the block indentation,
	// and content if it reaches past it
	roundtripYAML(t, "k: |\n  a\n \n  b\n", `{"k":"a\n\nb\n"}`)
	roundtripYAML(t, "k: |\n  a\n  \n  b\n", `{"k":"a\n\nb\n"}`)
	roundtripYAML(t, "k: |\n  a\n      \n  b\n", `{"k":"a\n    \nb\n"}`)
}

// Folded scalars leave more-indented lines alone (section 8.1.3): breaks on
// either side of one are kept rather than folded to a space.
func TestYAMLFoldedMoreIndentedLines(t *testing.T) {
	roundtripYAML(t, "k: >\n  a\n   more\n  b\n", `{"k":"a\n more\nb\n"}`)
	roundtripYAML(t, "k: >\n  a\n   m1\n   m2\n  b\n  c\n", `{"k":"a\n m1\n m2\nb c\n"}`)
	roundtripYAML(t, "k: >-\n  a\n   b\n", `{"k":"a\n b"}`)

	// an empty line next to a more-indented line keeps every break
	roundtripYAML(t, "k: >\n  a\n\n   more\n\n  b\n", `{"k":"a\n\n more\n\nb\n"}`)

	// ordinary folding is unaffected
	roundtripYAML(t, "k: >\n  a\n\n  b\n", `{"k":"a\nb\n"}`)
	roundtripYAML(t, "k: >\n  a\n\n\n  b\n", `{"k":"a\n\nb\n"}`)
	roundtripYAML(t, "k: >\n   a\n   b\n\n   c\n", `{"k":"a b\nc\n"}`)
}

// A block scalar indicator on its own line measures its content against the
// parent node's indentation, not the indicator line's own.
func TestYAMLBlockScalarIndicatorOnOwnLine(t *testing.T) {
	roundtripYAML(t, "k:\n  |-\n  a\n", `{"k":"a"}`)
	roundtripYAML(t, "k:\n  |-\n    a\n", `{"k":"a"}`)
	roundtripYAML(t, "k:\n  |\n  a\n  b\n", `{"k":"a\nb\n"}`)
}

func TestYAMLParseErrorString(t *testing.T) {
	e := &ParseError{Line: 3, Column: 7, Message: "bad token"}
	if got, want := e.Error(), "line 3, column 7: bad token"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestYAMLFlowSingleQuoted(t *testing.T) {
	roundtripYAML(t, `{key: 'hello world'}`, `{"key":"hello world"}`)
	roundtripYAML(t, `['a', 'b', 'c']`, `["a","b","c"]`)
	roundtripYAML(t, `{'my key': 42}`, `{"my key":42}`)
	roundtripYAML(t, `{'it''s': true}`, `{"it's":true}`)
}

func TestYAMLDoubleQuotedEscapes(t *testing.T) {
	// valid Go string escapes
	roundtripYAML(t, `"\b\f"`, `"\u0008\u000c"`)
	// invalid escapes are errors under Go string literal rules
	for _, bad := range []string{
		`"\/"`,           // \/ is JSON-only, not a Go escape
		`"\q"`,           // \q is not a recognized escape
		`"\u41"`,         // \uNNNN requires exactly 4 hex digits
		`"\uD800\uDC00"`, // surrogate pairs not supported by strconv.Unquote
	} {
		if _, err := FromYAML([]byte(bad)); err == nil {
			t.Errorf("%s should error under Go string literal rules", bad)
		}
	}
}

func TestYAMLControlCharEncoding(t *testing.T) {
	got, err := FromYAML([]byte("v: \"\x01\""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != `{"v":"\u0001"}` {
		t.Errorf("got %s, want {\"v\":\"\\u0001\"}", got)
	}
}

func TestYAMLIsNumberSigns(t *testing.T) {
	if !isYAMLNumber([]byte("+42")) {
		t.Error("isYAMLNumber(+42) should be true")
	}
	roundtripYAML(t, `1.5e+10`, `1.5e+10`)
	roundtripYAML(t, `2.0e-3`, `2.0e-3`)
}

func TestYAMLInlineMapSubValues(t *testing.T) {
	roundtripYAML(t, `
- name: Alice
  addr:
    city: NYC
`, `[{"name":"Alice","addr":{"city":"NYC"}}]`)

	roundtripYAML(t, `
- name: Alice
  tags: [go, yaml]
`, `[{"name":"Alice","tags":["go","yaml"]}]`)
}

func TestYAMLFlowErrors(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"unterminated mapping", `{key: value`},
		{"missing comma in mapping", `{a: {b: 1} {c: 2}}`},
		{"unterminated sequence", `[1, 2, 3`},
		{"missing comma in sequence", `[{a: 1} {b: 2}]`},
		{"unterminated string in flow", `{"key": "bad`},
		{"multiline unterminated mapping", "key: {a: 1,"},
		// unterminated double-quoted key — exercises parseFlowMapping flowParseKey error return
		{"unterminated quoted key in flow mapping", `{"bad key: 1}`},
		// unterminated double-quoted item — exercises parseFlowSequence flowParseItem error return
		{"unterminated quoted item in flow sequence", `["bad item]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FromYAML([]byte(tc.input))
			requireParseError(t, err)
		})
	}
}

func TestYAMLFlowDepthSingleQuoted(t *testing.T) {
	roundtripYAML(t,
		"key: ['it''s [ok]',\n      'fine']",
		`{"key":["it's [ok]","fine"]}`,
	)
	roundtripYAML(t, "key: {a: 1,\n\n     b: 2}", `{"key":{"a":1,"b":2}}`)
}

func TestYAMLConvertEmptyInput(t *testing.T) {
	for _, input := range []string{"", "  \n  \n", "# just a comment\n", "---\n"} {
		got, err := FromYAML([]byte(input))
		if err != nil || string(got) != "null" {
			t.Errorf("FromYAML(%q) = %s, %v; want null, nil", input, got, err)
		}
	}
}

func TestYAMLDoubleQuotedEscapesMore(t *testing.T) {
	// valid Go string escapes
	roundtripYAML(t, `"\r"`, `"\r"`)
	roundtripYAML(t, `"say \"hi\""`, `"say \"hi\""`)
	roundtripYAML(t, `"back\\slash"`, `"back\\slash"`)
	roundtripYAML(t, `"\u004a"`, `"J"`)
	// invalid hex in \uNNNN is an error under Go string literal rules
	if _, err := FromYAML([]byte(`"\uGHIJ"`)); err == nil {
		t.Error(`"\uGHIJ" should error: invalid hex digits in \uNNNN escape`)
	}
}

func TestYAMLSingleQuotedValueWithDoubleQuote(t *testing.T) {
	roundtripYAML(t, `'say "hello"'`, `"say \"hello\""`)
}

func TestYAMLFlowTrailingComma(t *testing.T) {
	roundtripYAML(t, `{a: 1,}`, `{"a":1}`)
	roundtripYAML(t, `[1, 2,]`, `[1,2]`)
}

func TestYAMLBlockScalarTopLevel(t *testing.T) {
	roundtripYAML(t, "|\n  hello\n  world\n", `"hello\nworld\n"`)
}

func TestYAMLBlockScalarEmptyBody(t *testing.T) {
	roundtripYAML(t, "key: |\nnext: value", `{"key":"","next":"value"}`)
}

func TestYAMLSequenceEmptyDash(t *testing.T) {
	roundtripYAML(t, "-\n  name: Alice", `[{"name":"Alice"}]`)
	roundtripYAML(t, "-\n  nested: value\n-\n  nested: other", `[{"nested":"value"},{"nested":"other"}]`)
}

func TestYAMLQuotedMapKeys(t *testing.T) {
	roundtripYAML(t, `"key\n": value`, "{\"key\\n\":\"value\"}")
	roundtripYAML(t, `'it''s key': value`, `{"it's key":"value"}`)
}

func TestYAMLParseErrorPaths(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"top-level unterminated string", `"unterminated`},
		{"mapping value block error", "key:\n  \"unterminated"},
		{"sequence empty-dash block error", "-\n  \"unterminated"},
		{"sequence flow error", `- {key: "bad`},
		{"sequence inline map scalar error", `- name: "bad`},
		{"sequence inline map continuation error", "- name: Alice\n  age: \"bad"},
		{"inline map flow error", "- name: {key: \"bad"},
		{"inline map block error", "- name:\n  \"bad"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FromYAML([]byte(tc.input))
			requireParseError(t, err)
		})
	}
}

// TestParseFlowExprDirect exercises the empty-string and bare-scalar branches of
// parseFlowExpr, which are unreachable via isFlowValue but exist as defensive cases.
func TestParseFlowExprDirect(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "null"},
		{"   ", "null"},
		{"hello", `"hello"`},
		{"42", "42"},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		if err := parseFlowExpr([]byte(tc.in), &buf); err != nil {
			t.Errorf("parseFlowExpr(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got := buf.String(); got != tc.want {
			t.Errorf("parseFlowExpr(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestFlowDepthSingleQuoteEscape exercises the i++ branch in flowDepth for ”
// (YAML single-quote escape). A bracket inside 'it”s}value' must not affect depth.
func TestFlowDepthSingleQuoteEscape(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{`{key: 'it''s fine'}`, 0},
		// bracket inside a '' escape — without i++, the } would close the outer {
		{`{key: 'val''s}inner'}`, 0},
		// unbalanced to verify non-zero result still works
		{`{key: 'val''s fine'`, 1},
		// double-quote \" escape: without i++, the " after \ closes the string early
		// and the trailing } falls outside, miscounting depth
		{"{\"\\\"\"}", 0},
	}
	for _, tc := range cases {
		if got := flowDepth([]byte(tc.s)); got != tc.want {
			t.Errorf("flowDepth(%q): got %d, want %d", tc.s, got, tc.want)
		}
	}
}

// A sequence written on the same line as its parent's dash used to be read as
// the plain string "- 1", and every line after it was silently dropped.
func TestYAMLNestedSequenceOnOneLine(t *testing.T) {
	roundtripYAML(t, "- - 1\n  - 2\n", `[[1,2]]`)
	roundtripYAML(t, "- - 1\n  - 2\n- - 3\n", `[[1,2],[3]]`)
	roundtripYAML(t, "- - 1\n  - 2\n- x\n", `[[1,2],"x"]`)
	roundtripYAML(t, "- - - 1\n", `[[[1]]]`)
	roundtripYAML(t, "- - 1\n  - 2\n  - - 3\n    - 4\n", `[[1,2,[3,4]]]`)

	// under a mapping key, both indented and compact
	roundtripYAML(t, "a:\n  - - 1\n    - 2\n", `{"a":[[1,2]]}`)
	roundtripYAML(t, "a:\n- - 1\n  - 2\n- 3\n", `{"a":[[1,2],3]}`)

	// the nested item may itself hold any value
	roundtripYAML(t, "- - a: 1\n    b: 2\n", `[[{"a":1,"b":2}]]`)
	roundtripYAML(t, "- - [1, 2]\n  - {a: 1}\n", `[[[1,2],{"a":1}]]`)
	roundtripYAML(t, "- - |\n    block\n  - 2\n", `[["block\n",2]]`)
	roundtripYAML(t, "- - null\n  - true\n", `[[null,true]]`)

	// extra spaces after a dash shift the column the nested sequence starts at
	roundtripYAML(t, "-   - 1\n    - 2\n", `[[1,2]]`)
}

// An empty sequence item is null. It used to swallow the items that followed
// it, turning them into a nested sequence.
func TestYAMLEmptySequenceItem(t *testing.T) {
	roundtripYAML(t, "-\n- 1\n", `[null,1]`)
	roundtripYAML(t, "-\n- 1\n- 2\n", `[null,1,2]`)
	roundtripYAML(t, "- 1\n-\n- 2\n", `[1,null,2]`)
	roundtripYAML(t, "- 1\n-\n", `[1,null]`)
	roundtripYAML(t, "- -\n  - 1\n", `[[null,1]]`)
	roundtripYAML(t, "a:\n-\n- 1\n", `{"a":[null,1]}`)

	// a genuinely nested block still belongs to the empty item
	roundtripYAML(t, "-\n  - 1\n", `[[1]]`)
	roundtripYAML(t, "-\n  a: 1\n", `[{"a":1}]`)
}

// A mapping opened on a sequence item line continues at the column its first
// key starts at, which is not always two past the dash.
func TestYAMLInlineMapExtraSpaces(t *testing.T) {
	roundtripYAML(t, "- name: a\n  age: 1\n", `[{"name":"a","age":1}]`)
	roundtripYAML(t, "-   name: a\n    age: 1\n", `[{"name":"a","age":1}]`)
	roundtripYAML(t, "-    name: a\n     age: 1\n     tags:\n       - x\n",
		`[{"name":"a","age":1,"tags":["x"]}]`)
}

func TestYAMLNestedSequenceTooDeep(t *testing.T) {
	deep := strings.Repeat("- ", yamlMaxCompactDepth+2) + "1\n"
	_, err := FromYAML([]byte(deep))
	pe := requireParseError(t, err)
	if pe.Message != errSeqTooDeep.Error() {
		t.Errorf("message = %q, want %q", pe.Message, errSeqTooDeep.Error())
	}

	// just under the limit is still accepted
	ok := strings.Repeat("- ", yamlMaxCompactDepth-1) + "1\n"
	if _, err := FromYAML([]byte(ok)); err != nil {
		t.Errorf("FromYAML at depth %d: %v", yamlMaxCompactDepth-1, err)
	}
}

// A line the parser cannot attach to any block used to be dropped in silence,
// truncating the document. It is now a parse error.
func TestYAMLUnattachedLine(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		line   int
		column int
	}{
		{"over-indented key", "a: 1\n  b: 2\n", 2, 3},
		{"over-indented item", "- 1\n  - 2\n", 2, 3},
		{"continuation after a quoted scalar", "k: \"quoted\"\n  more\n", 2, 3},
		{"continuation past a comment", "desc: one\n  # note\n  two\n", 3, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FromYAML([]byte(tc.input))
			pe := requireParseError(t, err)
			if pe.Message != errTrailingContent.Error() {
				t.Errorf("message = %q, want %q", pe.Message, errTrailingContent.Error())
			}
			if pe.Line != tc.line || pe.Column != tc.column {
				t.Errorf("position = %d:%d, want %d:%d", pe.Line, pe.Column, tc.line, tc.column)
			}
		})
	}
}

// A mapping key opened on a sequence item line, with nothing after the colon,
// is null. It used to absorb the sibling keys that followed it.
func TestYAMLInlineMapEmptyValue(t *testing.T) {
	roundtripYAML(t, "- k1:\n  k2: 2\n", `[{"k1":null,"k2":2}]`)
	roundtripYAML(t, "- k1:\n  k2: 2\n- x\n", `[{"k1":null,"k2":2},"x"]`)
	roundtripYAML(t, "- k1:\n  k2:\n  k3: 3\n", `[{"k1":null,"k2":null,"k3":3}]`)

	// a genuinely deeper block, or a sequence at the key's own indentation,
	// still belongs to the key
	roundtripYAML(t, "- k1:\n    a: 1\n  k2: 2\n", `[{"k1":{"a":1},"k2":2}]`)
	roundtripYAML(t, "- k1:\n  - 1\n  k2: 2\n", `[{"k1":[1],"k2":2}]`)
}

// A scalar whose exponent has no digits, or that is a lone dot, is a string.
// It used to be treated as a number and written through unchanged, leaving the
// output something other than JSON.
func TestYAMLMalformedNumbers(t *testing.T) {
	roundtripYAML(t, "a: 0e\nb: 1e\nc: 1e+\nd: .\ne: -\nf: +\n",
		`{"a":"0e","b":"1e","c":"1e+","d":".","e":"-","f":"+"}`)

	// the numbers around them still convert
	roundtripYAML(t, "a: 0\nb: .5\nc: 0.\nd: 1e5\ne: 1E-3\nf: -0.25\n",
		`{"a":0,"b":0.5,"c":0.0,"d":1e5,"e":1E-3,"f":-0.25}`)
}

// Tabs count as several columns of indentation, so a column offset cannot be
// used to slice a line. Doing that used to panic.
func TestYAMLTabIndentation(t *testing.T) {
	roundtripYAML(t, "\t\t0\n", `0`)
	roundtripYAML(t, "\t0\n", `0`)
	roundtripYAML(t, "a:\n\tb: 1\n", `{"a":{"b":1}}`)
	roundtripYAML(t, "a: |\n\t\tx\n\t\ty\n", `{"a":"x\ny\n"}`)
	roundtripYAML(t, "a:\n\t- 1\n\t- 2\n", `{"a":[1,2]}`)
}

// A plain scalar may continue on the lines below it. Continuations fold into
// the value with single spaces; blank lines between them become newlines.
func TestYAMLMultiLinePlainScalar(t *testing.T) {
	roundtripYAML(t, "desc: one\n  two\n", `{"desc":"one two"}`)
	roundtripYAML(t, "desc: one\n  two\n  three\n", `{"desc":"one two three"}`)
	roundtripYAML(t, "desc: one\n  two\ntitle: x\n", `{"desc":"one two","title":"x"}`)
	roundtripYAML(t, "desc: one\n  two\n\n  three\n", `{"desc":"one two\nthree"}`)

	// a scalar block under a key, and a bare document
	roundtripYAML(t, "a:\n  one\n  two\n", `{"a":"one two"}`)
	roundtripYAML(t, "scalar\nmore\n", `"scalar more"`)

	// sequence items, including one opening a mapping
	roundtripYAML(t, "- one\n  two\n", `["one two"]`)
	roundtripYAML(t, "- one\n  two\n- three\n", `["one two","three"]`)
	roundtripYAML(t, "- name: a\n    cont\n  age: 1\n", `[{"name":"a cont","age":1}]`)

	// the folded text is a string even when its first line looks like a number
	roundtripYAML(t, "desc: 1\n  2\n", `{"desc":"1 2"}`)

	// structure below the value still parses as structure
	roundtripYAML(t, "a: 1\nb:\n  - 1\n  - 2\n", `{"a":1,"b":[1,2]}`)
	roundtripYAML(t, "a: |\n  x\n  y\n", `{"a":"x\ny\n"}`)
	roundtripYAML(t, "a: >\n  folded\n  text\n", `{"a":"folded text\n"}`)
}

// An indicator deeper than the content leaves the block empty. Whether that is
// an error depends on where the content line lands.
func TestYAMLBlockScalarIndicatorOvershoot(t *testing.T) {
	// the line belongs to nothing, so the document is rejected
	if _, err := FromYAML([]byte("k: |4\n  a\n")); err == nil {
		t.Error("overshooting indicator with an orphan line: want error")
	}
	// here the line fits the enclosing mapping, so it is read as part of that
	roundtripYAML(t, "a:\n  k: |9\n  b: 1\n", `{"a":{"k":"","b":1}}`)
	roundtripYAML(t, "k: |9\nj: 1\n", `{"k":"","j":1}`)
	roundtripYAML(t, "- |9\n- x\n", `["","x"]`)
}
