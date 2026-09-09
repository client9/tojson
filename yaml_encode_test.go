package tojson

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToYAML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"scalar", `42`, "42\n"},
		{"emptyObject", `{}`, "{}\n"},
		{"emptyArray", `[]`, "[]\n"},
		{"flatMap", `{"a":1,"b":"two","c":true,"d":null}`,
			"a: 1\nb: two\nc: true\nd: null\n"},
		{"nestedMap", `{"a":{"b":{"c":1}}}`,
			"a:\n  b:\n    c: 1\n"},
		{"seqOfScalars", `[1,"two",false]`,
			"- 1\n- two\n- false\n"},
		{"seqUnderKey", `{"list":[1,2]}`,
			"list:\n  - 1\n  - 2\n"},
		{"seqOfMaps", `[{"a":1,"b":2},{"a":3}]`,
			"- a: 1\n  b: 2\n- a: 3\n"},
		{"seqOfSeqs", `[[1,2],[3]]`,
			"-\n  - 1\n  - 2\n-\n  - 3\n"},
		{"emptyNested", `{"a":{},"b":[]}`,
			"a: {}\nb: []\n"},
		{"emptyInSeq", `[{},[],1]`,
			"- {}\n- []\n- 1\n"},
		{"quotedWhenAmbiguous", `{"a":"true","b":"123","c":"","d":"yes"}`,
			"a: \"true\"\nb: \"123\"\nc: \"\"\nd: \"yes\"\n"},
		{"quotedIndicators", `{"a":"- x","b":"x: y","c":"#c","d":" pad "}`,
			"a: \"- x\"\nb: \"x: y\"\nc: \"#c\"\nd: \" pad \"\n"},
		{"numericKey", `{"1":"a","true":"b"}`,
			"\"1\": a\n\"true\": b\n"},
		{"blockScalar", `{"text":"line one\nline two\n"}`,
			"text: |\n  line one\n  line two\n"},
		{"blockScalarStrip", `{"text":"line one\nline two"}`,
			"text: |-\n  line one\n  line two\n"},
		{"blockScalarBlankLine", `{"text":"a\n\nb\n"}`,
			"text: |\n  a\n\n  b\n"},
		{"blockScalarInSeq", `["a\nb"]`,
			"- |-\n  a\n  b\n"},
		{"blockRejectedIndent", `{"t":"a\n  b\n"}`,
			"t: \"a\\n  b\\n\"\n"},
		{"blockRejectedTrailingNewlines", `{"t":"a\n\n"}`,
			"t: \"a\\n\\n\"\n"},
		{"unicode", `{"k":"héllo","emoji":"🎉"}`,
			"k: héllo\nemoji: 🎉\n"},
		{"escapes", `{"k":"tab\there"}`,
			"k: \"tab\\there\"\n"},
		{"json5", "{unquoted: 'single', hex: 0xff, trailing: [1,2,],}",
			"unquoted: single\nhex: 255\ntrailing:\n  - 1\n  - 2\n"},
		{"comments", "{// hi\na: 1 /* mid */, b: 2}",
			"a: 1\nb: 2\n"},
		{"bignum", `{"num":1e309,"i":123456789012345678901234567890}`,
			"num: 1e309\ni: 123456789012345678901234567890\n"},
		{"infinity", `{"a":Infinity,"b":-Infinity,"c":NaN}`,
			"a: .inf\nb: -.inf\nc: .nan\n"},
		{"yaml11Keywords", `{"n":1,"y":2,"off":3}`,
			"\"n\": 1\n\"y\": 2\n\"off\": 3\n"},
		{"deepMix", `{"a":[{"b":[1,{"c":"d"}]}]}`,
			"a:\n  - b:\n      - 1\n      - c: d\n"},
		{"empty", ``, ""},
		{"dateLike", `{"d":"2020-01-02","t":"12:30:00"}`,
			"d: \"2020-01-02\"\nt: \"12:30:00\"\n"},
		{"dashWord", `{"a":"-x","b":"a-b"}`,
			"a: -x\nb: a-b\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToYAML([]byte(tc.in))
			if err != nil {
				t.Fatalf("ToYAML(%q) error = %v", tc.in, err)
			}
			if string(got) != tc.want {
				t.Errorf("ToYAML(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestToYAMLRoundTrip converts JSON to YAML, reads it back with FromYAML, and
// requires the result to be the same document.
func TestToYAMLRoundTrip(t *testing.T) {
	inputs := []string{
		`{"a":1,"b":[1,2,3],"c":{"d":"e"}}`,
		`[{"name":"one","tags":["x","y"],"on":true},{"name":"two","tags":[]}]`,
		`{"text":"line one\nline two\n","note":"x: y","empty":"","n":null}`,
		`{"deep":{"a":{"b":{"c":[{"d":[[1,2],[3]]}]}}}}`,
		`{"nums":[0,-1,1.5,1e10,-2.5e-3],"strs":["true","0","null","-","#"]}`,
		`{"unicode":"héllo 🎉","quote":"say \"hi\"","tab":"a\tb"}`,
		`[]`,
		`{}`,
		`{"a":{},"b":[],"c":[[]],"d":[{}]}`,
	}
	for _, in := range inputs {
		y, err := ToYAML([]byte(in))
		if err != nil {
			t.Fatalf("ToYAML(%s) error = %v", in, err)
		}
		back, err := FromYAML(y)
		if err != nil {
			t.Fatalf("FromYAML of\n%s\nerror = %v", y, err)
		}
		if !sameJSON(t, []byte(in), back) {
			t.Errorf("round trip mismatch\n  in:   %s\n  yaml: %s\n  back: %s", in, y, back)
		}
	}
}

func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	av, err := decodeJSON(a)
	if err != nil {
		t.Fatalf("decoding %s: %v", a, err)
	}
	bv, err := decodeJSON(b)
	if err != nil {
		t.Fatalf("decoding %s: %v", b, err)
	}
	ab, _ := json.Marshal(av)
	bb, _ := json.Marshal(bv)
	return bytes.Equal(ab, bb)
}

// decodeJSON decodes with UseNumber so that numbers this package passes
// through untouched, such as 1e309, survive the comparison instead of
// overflowing float64.
func decodeJSON(b []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func TestToYAMLErrors(t *testing.T) {
	cases := []string{
		`{"a":`,
		`{"a":0x10000000000000000}`,
		`{"a":"unterminated}`,
		`[`,
		`{`,
		`{"a"`,
		`{"a" 1}`,
		`[1,2`,
		`{"a":1} extra`,
		`{[1]:2}`,
	}
	for _, in := range cases {
		if out, err := ToYAML([]byte(in)); err == nil {
			t.Errorf("ToYAML(%q) = %q, want error", in, out)
		}
	}
}

func TestToYAMLWideIndent(t *testing.T) {
	// nest deep enough to exercise the writeIndent loop past one span of spaces
	const levels = 24
	in := ""
	for i := 0; i < levels; i++ {
		in += `{"k":`
	}
	in += `"v"`
	for i := 0; i < levels; i++ {
		in += "}"
	}
	got, err := ToYAML([]byte(in))
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}
	want := ""
	for i := 0; i < levels; i++ {
		want += strings.Repeat(" ", i*2) + "k:"
		if i == levels-1 {
			want += " v\n"
		} else {
			want += "\n"
		}
	}
	if string(got) != want {
		t.Errorf("ToYAML() =\n%s\nwant\n%s", got, want)
	}
}

func TestToYAMLBlockScalarDeepIndent(t *testing.T) {
	in := `{"aaaaaaaaaa":{"bbbbbbbbbb":{"cccccccccc":{"dddddddddd":{"e":"one\ntwo\n"}}}}}`
	got, err := ToYAML([]byte(in))
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}
	back, err := FromYAML(got)
	if err != nil {
		t.Fatalf("FromYAML() error = %v\n%s", err, got)
	}
	if !sameJSON(t, []byte(in), back) {
		t.Errorf("round trip mismatch: %s", back)
	}
}

func TestToYAMLDeepNesting(t *testing.T) {
	src := bytes.Repeat([]byte("["), yamlMaxDepth+5)
	src = append(src, bytes.Repeat([]byte("]"), yamlMaxDepth+5)...)
	if _, err := ToYAML(src); err == nil {
		t.Error("ToYAML(deeply nested) = nil error, want error")
	}
}

// TestToYAMLCorpus converts every sample file in the repository to JSON, then
// to YAML, then back with FromYAML, and requires the document to survive.
func TestToYAMLCorpus(t *testing.T) {
	var files []string
	for _, root := range []string{"samples", "testdata"} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}

	checked := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		var doc []byte
		switch filepath.Ext(f) {
		case ".json", ".json5", ".jsonc", ".hjson", ".txt", ".js":
			doc, err = FromJSONVariant(src)
		case ".yaml", ".yml":
			doc, err = FromYAML(src)
		case ".toml":
			doc, err = FromTOML(src)
		default:
			continue
		}
		// the corpus deliberately contains inputs that do not convert
		if err != nil || len(doc) == 0 {
			continue
		}
		if !json.Valid(doc) {
			continue // the source converter produced non-JSON (signed hex)
		}
		checked++

		y, err := ToYAML(doc)
		if err != nil {
			t.Errorf("%s: ToYAML: %v", f, err)
			continue
		}
		back, err := FromYAML(y)
		if err != nil {
			t.Errorf("%s: FromYAML of generated YAML: %v\n%s", f, err, y)
			continue
		}
		if !sameJSON(t, doc, back) {
			t.Errorf("%s: round trip mismatch\n want: %s\n got:  %s\nyaml:\n%s", f, doc, back, y)
		}
	}
	if checked == 0 {
		t.Fatal("no sample files were checked")
	}
}

// Characters that a YAML reader trims or treats as a line break cannot survive
// a plain scalar, so a string holding one is quoted.
func TestToYAMLUnicodeWhitespace(t *testing.T) {
	const (
		nel  = "\u0085" // next line
		nbsp = "\u00a0" // no-break space
		idsp = "\u3000" // ideographic space
	)
	cases := []struct{ in, want string }{
		{`"` + nel + `"`, "\"" + nel + "\"\n"},
		{`"` + nbsp + `"`, "\"" + nbsp + "\"\n"},
		{`"a` + nel + `b"`, "\"a" + nel + "b\"\n"},
		{`"` + idsp + `x"`, "\"" + idsp + "x\"\n"},
		{`{"k":"x` + nbsp + `"}`, "k: \"x" + nbsp + "\"\n"},
		// ordinary non-ASCII text stays plain
		{"\"h\u00e9llo\"", "h\u00e9llo\n"},
		{"\"\u65e5\u672c\u8a9e\"", "\u65e5\u672c\u8a9e\n"},
	}
	for _, tc := range cases {
		got, err := ToYAML([]byte(tc.in))
		if err != nil {
			t.Errorf("ToYAML(%s) error = %v", tc.in, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("ToYAML(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}

	// and the document survives the trip back
	for _, in := range []string{
		`"` + nel + `"`,
		`"` + nbsp + `"`,
		`{"k":"a` + nel + `b"}`,
		`["` + nbsp + `"]`,
		`"a\nb` + nel + `c\n"`,
	} {
		y, err := ToYAML([]byte(in))
		if err != nil {
			t.Fatalf("ToYAML(%s) error = %v", in, err)
		}
		back, err := FromYAML(y)
		if err != nil {
			t.Fatalf("FromYAML(%q) error = %v", y, err)
		}
		doc, err := FromJSONVariant([]byte(in))
		if err != nil {
			t.Fatalf("FromJSONVariant(%s) error = %v", in, err)
		}
		if !sameJSON(t, doc, back) {
			t.Errorf("round trip mismatch for %s\n want: %s\n got:  %s\nyaml: %q", in, doc, back, y)
		}
	}
}

// A block scalar cannot carry whitespace a reader would trim or treat as a
// line break, so a string holding one is written double-quoted instead.
func TestToYAMLBlockScalarExoticSpace(t *testing.T) {
	const emsp = "\u2004" // three-per-em space
	cases := []struct{ in, want string }{
		{`"a\nb\n"`, "|\n  a\n  b\n"},
		{`"\n` + emsp + `"`, `"\n` + emsp + `"` + "\n"},
		{`"a\n` + emsp + `b\n"`, `"a\n` + emsp + `b\n"` + "\n"},
	}
	for _, tc := range cases {
		got, err := ToYAML([]byte(tc.in))
		if err != nil {
			t.Errorf("ToYAML(%s) error = %v", tc.in, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("ToYAML(%s) = %q, want %q", tc.in, got, tc.want)
		}
		back, err := FromYAML(got)
		if err != nil {
			t.Errorf("FromYAML(%q) error = %v", got, err)
			continue
		}
		doc, _ := FromJSONVariant([]byte(tc.in))
		if !sameJSON(t, doc, back) {
			t.Errorf("round trip mismatch for %s: %s", tc.in, back)
		}
	}
}
