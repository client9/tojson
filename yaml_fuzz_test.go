package tojson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// The fuzz targets here are property tests. The interesting one is
// FuzzYAMLLayout, which builds a YAML document and the JSON it must convert to
// at the same time, so it can check the result without a reference parser.

// yamlFuzzMaxDepth bounds generated nesting so a document stays small.
const yamlFuzzMaxDepth = 4

// node kinds a generated value can take
const (
	nodeScalar = iota
	nodeSeq
	nodeMap
)

// fuzzScalars pairs a YAML scalar with the JSON it converts to.
var fuzzScalars = []struct{ yaml, json string }{
	{"alpha", `"alpha"`},
	{"some words here", `"some words here"`},
	{`"quoted: value"`, `"quoted: value"`},
	{`'single #1'`, `"single #1"`},
	{"42", "42"},
	{"-7", "-7"},
	{"1.5", "1.5"},
	{"true", "true"},
	{"false", "false"},
	{"null", "null"},
}

// yamlLayout renders a YAML document and its expected JSON side by side. Every
// choice it makes comes from the fuzzer's bytes, so a failure is reproducible
// from the corpus entry alone. Once those bytes run out every choice is zero,
// which ends the document.
type yamlLayout struct {
	src  []byte
	pos  int
	yaml bytes.Buffer
	want bytes.Buffer
}

// pick returns a value in [0,n).
func (g *yamlLayout) pick(n int) int {
	if n <= 1 || g.pos >= len(g.src) {
		return 0
	}
	b := g.src[g.pos]
	g.pos++
	return int(b) % n
}

func (g *yamlLayout) pad(n int) {
	for i := 0; i < n; i++ {
		g.yaml.WriteByte(' ')
	}
}

func (g *yamlLayout) kind(depth int) int {
	if depth >= yamlFuzzMaxDepth {
		return nodeScalar
	}
	switch g.pick(4) {
	case 1:
		return nodeSeq
	case 2:
		return nodeMap
	}
	return nodeScalar
}

// node writes a value of the given kind. indent is the column the value starts
// at; when inline is false, node writes that indentation itself.
func (g *yamlLayout) node(kind, indent int, inline bool, depth int) {
	switch kind {
	case nodeSeq:
		g.seq(indent, inline, depth)
	case nodeMap:
		g.mapping(indent, inline, depth)
	default:
		g.scalar(indent, indent+2, inline)
	}
}

// scalar writes a plain scalar, or now and then a literal block scalar whose
// content sits two columns deeper than the value itself. contIndent is the
// column a continuation line of this value must start at.
func (g *yamlLayout) scalar(indent, contIndent int, inline bool) {
	if !inline {
		g.pad(indent)
	}
	if g.pick(8) == 0 {
		g.yaml.WriteString("|\n")
		g.pad(indent + 2)
		g.yaml.WriteString("line one\n")
		g.pad(indent + 2)
		g.yaml.WriteString("line two\n")
		g.want.WriteString(`"line one\nline two\n"`)
		return
	}
	s := fuzzScalars[g.pick(len(fuzzScalars))]
	// only a plain scalar continues across lines
	plain := s.yaml[0] != '"' && s.yaml[0] != '\''
	if words := strings.Fields(s.yaml); plain && len(words) > 1 &&
		strings.Join(words, " ") == s.yaml && g.pick(3) == 0 {
		// spread the words over several lines; folding must put them back
		g.yaml.WriteString(words[0])
		for _, w := range words[1:] {
			if g.pick(2) == 0 {
				g.yaml.WriteByte('\n')
				g.pad(contIndent)
			} else {
				g.yaml.WriteByte(' ')
			}
			g.yaml.WriteString(w)
		}
		g.yaml.WriteByte('\n')
		g.want.WriteString(s.json)
		return
	}
	g.yaml.WriteString(s.yaml)
	g.yaml.WriteByte('\n')
	g.want.WriteString(s.json)
}

func (g *yamlLayout) seq(indent int, inline bool, depth int) {
	n := 1 + g.pick(3)
	g.want.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			g.want.WriteByte(',')
		}
		if i > 0 || !inline {
			g.pad(indent)
		}
		// An item with nothing after its dash is null: the next line is
		// either a sibling at this indentation or the end of the sequence.
		if g.pick(6) == 0 {
			g.yaml.WriteString("-\n")
			g.want.WriteString("null")
			continue
		}
		k := g.kind(depth + 1)
		if k != nodeScalar && g.pick(2) == 0 {
			// container on the lines below the dash
			g.yaml.WriteString("-\n")
			g.node(k, indent+2, false, depth+1)
			continue
		}
		// value on the dash's own line, sometimes past extra spaces
		gap := " "
		if g.pick(4) == 0 {
			gap = "   "
		}
		g.yaml.WriteByte('-')
		g.yaml.WriteString(gap)
		g.node(k, indent+1+len(gap), true, depth+1)
	}
	g.want.WriteByte(']')
}

func (g *yamlLayout) mapping(indent int, inline bool, depth int) {
	n := 1 + g.pick(3)
	g.want.WriteByte('{')
	for i := 0; i < n; i++ {
		if i > 0 {
			g.want.WriteByte(',')
		}
		if i > 0 || !inline {
			g.pad(indent)
		}
		key := fmt.Sprintf("k%d", i)
		g.yaml.WriteString(key)
		g.yaml.WriteByte(':')
		g.want.WriteString(`"` + key + `":`)

		// a key with nothing after it, and no deeper block, is null
		if g.pick(8) == 0 {
			g.yaml.WriteByte('\n')
			g.want.WriteString("null")
			continue
		}
		k := g.kind(depth + 1)
		if k == nodeScalar {
			g.yaml.WriteByte(' ')
			// a continuation must be indented past the key
			g.scalar(indent+len(key)+2, indent+2, true)
			continue
		}
		g.yaml.WriteByte('\n')
		child := indent + 2
		// a sequence may also sit at its key's own indentation
		if k == nodeSeq && g.pick(2) == 0 {
			child = indent
		}
		g.node(k, child, false, depth+1)
	}
	g.want.WriteByte('}')
}

func (g *yamlLayout) build() {
	g.node(g.kind(0), 0, false, 0)
}

// FuzzYAMLLayout generates a YAML document alongside the JSON it must produce,
// then checks FromYAML against it. The generator picks freely among the
// layouts YAML allows for the same data: a nested collection on the dash's own
// line or the lines below it, a sequence at its key's indentation or deeper,
// extra spaces after a dash, empty items, and block scalars.
func FuzzYAMLLayout(f *testing.F) {
	for _, seed := range [][]byte{
		{1, 1, 1},
		{1, 2, 1, 3, 5, 7, 11, 13},
		{2, 1, 2, 0, 4, 1, 6, 3, 8, 5, 10, 7},
		{1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8},
		{2, 2, 1, 5, 2, 1, 5, 2, 1, 5, 2, 1, 5, 2, 1, 5, 2, 1},
		{9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 11, 13, 17, 19, 23, 29, 31, 37, 41},
		{1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		g := &yamlLayout{src: src}
		g.build()

		got, err := FromYAML(g.yaml.Bytes())
		if err != nil {
			t.Fatalf("FromYAML error: %v\ninput:\n%s", err, g.yaml.String())
		}
		if !sameJSON(t, g.want.Bytes(), got) {
			t.Errorf("mismatch\n want: %s\n got:  %s\ninput:\n%s",
				g.want.String(), got, g.yaml.String())
		}
	})
}

// FuzzFromYAML checks that arbitrary input never panics and that a success
// always yields parseable JSON.
func FuzzFromYAML(f *testing.F) {
	f.Add(frontmatter1YAML)
	for _, seed := range []string{
		"a: 1\nb:\n  - 1\n  - 2\n",
		"- - 1\n  - 2\n",
		"- |\n  text\n",
		"{a: 1, b: [1, 2]}\n",
		"-\n- 1\n",
		"a: >\n  folded\n  text\n",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		out, err := FromYAML([]byte(src))
		if err != nil {
			return
		}
		if !json.Valid(out) {
			t.Errorf("FromYAML(%q) = %q, which is not JSON", src, out)
		}
	})
}

// FuzzJSONVariant checks that arbitrary input never panics and that a success
// always yields parseable JSON.
func FuzzJSONVariant(f *testing.F) {
	for _, seed := range []string{
		`{"a":1,"b":[1,2],"c":null}`,
		"{unquoted: 'v', /* c */ hex: 0x2a, trailing: [1,2,],}",
		"// lead\n{a: 1} # trail\n",
		`{"n":1e309,"m":-0.5,"e":1E-3}`,
		"[1 2 3]",
		"`backtick`",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		out, err := FromJSONVariant([]byte(src))
		if err != nil || len(out) == 0 {
			return // empty input converts to empty output, by design
		}
		if !json.Valid(out) {
			t.Errorf("FromJSONVariant(%q) = %q, which is not JSON", src, out)
		}
	})
}

// FuzzToYAMLRoundTrip checks that a document converted to YAML reads back as
// the same document.
func FuzzToYAMLRoundTrip(f *testing.F) {
	for _, seed := range []string{
		`{"a":1,"b":[1,2,{"c":"d"}]}`,
		`[[1,2],[],{},null,true]`,
		`{"text":"line one\nline two\n","k":"x: y"}`,
		"{unquoted: 'v', hex: 0x2a, trailing: [1,2,],}",
		`"top level string"`,
		`{"nested":{"deep":{"deeper":[{"a":[]}]}}}`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		doc, err := FromJSONVariant([]byte(src))
		if err != nil || len(doc) == 0 {
			return
		}
		if !json.Valid(doc) {
			return // input the JSON path does not turn into valid JSON
		}
		// A carriage return inside a string does not survive the trip: the
		// converters normalize CRLF to LF while re-encoding, so the value
		// comes back a byte shorter. That is longstanding behaviour, and the
		// upstream conformance tests for it are skipped in json5_test.go.
		if v, err := decodeJSON(doc); err == nil && valueHasCR(v) {
			return
		}
		// FromYAML decodes double-quoted strings with Go string literal
		// rules, which reject surrogate escapes. That limitation is spelled
		// out in docs/yaml-minimal.md.
		if hasSurrogateEscape(doc) {
			return
		}
		y, err := ToYAML(doc)
		if err != nil {
			t.Fatalf("ToYAML(%s) error: %v", doc, err)
		}
		back, err := FromYAML(y)
		if err != nil {
			t.Fatalf("FromYAML error: %v\nyaml:\n%s", err, y)
		}
		if !sameJSON(t, doc, back) {
			t.Errorf("round trip mismatch\n want: %s\n got:  %s\nyaml:\n%s", doc, back, y)
		}
	})
}

// valueHasCR reports whether any string in a decoded JSON document holds a
// carriage return.
func valueHasCR(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.ContainsRune(t, '\r')
	case []any:
		for _, x := range t {
			if valueHasCR(x) {
				return true
			}
		}
	case map[string]any:
		for k, x := range t {
			if strings.ContainsRune(k, '\r') || valueHasCR(x) {
				return true
			}
		}
	}
	return false
}

// hasSurrogateEscape reports whether b holds a \uD800-\uDFFF escape.
func hasSurrogateEscape(b []byte) bool {
	for i := 0; i+3 < len(b); i++ {
		if b[i] != '\\' || b[i+1] != 'u' {
			continue
		}
		if b[i+2] != 'd' && b[i+2] != 'D' {
			continue
		}
		if v := hexVal(b[i+3]); v >= 8 {
			return true
		}
	}
	return false
}

// TestYAMLLayoutGenerator keeps the generator honest under plain "go test":
// a spread of inputs must produce documents that convert as predicted.
func TestYAMLLayoutGenerator(t *testing.T) {
	seen := map[string]bool{}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 5000; i++ {
		src := make([]byte, r.Intn(48))
		for j := range src {
			src[j] = byte(r.Intn(256))
		}
		g := &yamlLayout{src: src}
		g.build()
		seen[g.yaml.String()] = true

		got, err := FromYAML(g.yaml.Bytes())
		if err != nil {
			t.Fatalf("FromYAML error: %v\ninput:\n%s", err, g.yaml.String())
		}
		if !sameJSON(t, g.want.Bytes(), got) {
			t.Fatalf("mismatch\n want: %s\n got:  %s\ninput:\n%s",
				g.want.String(), got, g.yaml.String())
		}
	}
	if len(seen) < 100 {
		t.Errorf("generator produced only %d distinct documents", len(seen))
	}
	// the generator must reach the layouts this file exists to cover
	var all strings.Builder
	for doc := range seen {
		all.WriteString(doc)
	}
	for _, want := range []string{"- - ", "-   ", "-\n", "|\n", "words\n"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("generator never produced %q", want)
		}
	}
}
