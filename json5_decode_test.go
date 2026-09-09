package tojson

import (
	"testing"
	"time"
)

type testcase struct {
	in  string
	out string
}

// Valid JSON.. Decode should return return the input unchanged
func TestDecodeIdentity(t *testing.T) {
	cases := []string{
		"",
		"null",
		"true",
		"false",
		"123",
		"-123",
		"0.5",
		"-0.5",
		"\"abc\"",
		"\"abc\"",
		"{}",
		"[]",
		"[1,2,3]",
		"{\"foo\":\"bar\"}",
		"{\"foo\":\"bar\",\"rock\":\"roll\"}",
		"[{}]",
		"{\"foo\":\"bar\",\"rock\":[]}",
		"{\"foo\":\"bar\",\"rock\":{}}",
	}
	for _, tt := range cases {
		out, err := FromJSONVariant([]byte(tt))
		if err != nil {
			t.Errorf("Got unexpected error: %v", err)
		}
		got := string(out)
		if tt != got {
			t.Errorf("Expected %q got %q", tt, got)
		}
	}
}

// tests non-JSON with leading and trailing commas,
// and other degenerate forms
func TestDecodeComma(t *testing.T) {
	cases := []testcase{
		{
			"[1,2,3,]",
			"[1,2,3]",
		},
		{
			"[,1,2,3,]",
			"[1,2,3]",
		},
		{
			"{\"foo\":1,}",
			"{\"foo\":1}",
		},
		{
			"[,]", // degenerate case
			"[]",
		},
		{
			"[,1,]", // degenerate case
			"[1]",
		},
		{
			"{,}", // degenerate case
			"{}",
		},
	}
	for _, tt := range cases {
		out, err := FromJSONVariant([]byte(tt.in))
		if err != nil {
			t.Errorf("Got unexpected error: %v", err)
		}
		got := string(out)
		if tt.out != got {
			t.Errorf("Expected %q got %q", tt.out, got)
		}
	}
}
func TestDecodeComments(t *testing.T) {
	cases := []testcase{
		{
			`[1,2,
			// single line comment
			3,]`,
			"[1,2,3]",
		},
		{
			`[1,2,
			# single line comment
			3,]`,
			"[1,2,3]",
		},
		{
			`[1,2,
			/* multi 
			line
			comment
			*/
			3,]`,
			"[1,2,3]",
		},
	}
	for _, tt := range cases {
		out, err := FromJSONVariant([]byte(tt.in))
		if err != nil {
			t.Errorf("Got error: %v", err)
		}
		got := string(out)
		if tt.out != got {
			t.Errorf("Expected %q got %q", tt.out, got)
		}
	}
}
func TestDecodeNumbers(t *testing.T) {
	cases := []testcase{
		{
			"+123",
			"123",
		},
		{
			"+1.5",
			"1.5",
		},
		{
			"01",
			"1",
		},
		{
			"01.5",
			"1.5",
		},
		{
			".5",
			"0.5",
		},
		{
			"5.",
			"5.0",
		},
		{
			"0xFF",
			"255",
		},
		// large integer passes through without evaluation
		{"+99999999999999999", "99999999999999999"},
		// hex: full uint64 range
		{"0xFFFFFFFFFFFFFFFF", "18446744073709551615"},
	}

	for _, tt := range cases {
		out, err := FromJSONVariant([]byte(tt.in))
		if err != nil {
			t.Errorf("Got error: %v", err)
		}
		got := string(out)
		if tt.out != got {
			t.Errorf("Expected %q got %q", tt.out, got)
		}
	}
}

func TestDecodeNumberErrors(t *testing.T) {
	cases := []string{
		// hex overflow: 2^64, exceeds uint64
		"0x10000000000000000",
		// hex overflow in object value
		`{"x":0x10000000000000000}`,
		// hex overflow in array
		`[0x10000000000000000]`,
	}
	for _, in := range cases {
		_, err := FromJSONVariant([]byte(in))
		if err == nil {
			t.Errorf("FromJSONVariant(%q): expected error, got nil", in)
		}
	}
}

// An unterminated block comment used to leave the tokenizer stuck on the same
// bytes forever. It is now a parse error.
func TestDecodeUnterminatedBlockComment(t *testing.T) {
	cases := []string{
		"/*",
		"/* no end",
		"{}/*x",
		"[1] /* nope",
		"{\n  \"k\": 1,\n  /* dangling\n}",
		"/* outer /* inner",
	}
	for _, in := range cases {
		done := make(chan error, 1)
		go func() {
			_, err := FromJSONVariant([]byte(in))
			done <- err
		}()
		select {
		case err := <-done:
			pe := requireParseError(t, err)
			if pe.Message != "unterminated block comment" {
				t.Errorf("FromJSONVariant(%q): message = %q, want %q", in, pe.Message, "unterminated block comment")
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("FromJSONVariant(%q) did not return", in)
		}
	}
}

func TestDecodeNumberPassthrough(t *testing.T) {
	// large floats pass through as-is — no float64 evaluation
	cases := []testcase{
		{"+1e309", "1e309"},
		{`{"x":+1e309}`, `{"x":1e309}`},
		{`[+1e309]`, `[1e309]`},
		{"+111111111111111111111123199999999999999999.0", "111111111111111111111123199999999999999999.0"},
	}
	for _, tt := range cases {
		out, err := FromJSONVariant([]byte(tt.in))
		if err != nil {
			t.Errorf("FromJSONVariant(%q): unexpected error: %v", tt.in, err)
			continue
		}
		if got := string(out); got != tt.out {
			t.Errorf("FromJSONVariant(%q): got %q, want %q", tt.in, got, tt.out)
		}
	}
}

func TestDecodeStrings(t *testing.T) {
	cases := []testcase{
		{
			"\"foo\"",
			"\"foo\"",
		},
		{
			"'foo'",
			"\"foo\"",
		},
		{
			"`foo`",
			"\"foo\"",
		},
		{
			"`foo\nbar`",
			"\"foo\\nbar\"",
		},
	}
	for _, tt := range cases {
		out, err := FromJSONVariant([]byte(tt.in))
		if err != nil {
			t.Errorf("Got error: %v", err)
		}
		got := string(out)
		if tt.out != got {
			t.Errorf("Expected %s got %s", tt.out, got)
		}
	}
}

func TestDecodeArrayValueTypes(t *testing.T) {
	cases := []testcase{
		{"[1.5]", "[1.5]"},
		{"[0xFF]", "[255]"},
		{"[[1,2]]", "[[1,2]]"},
		{`[{"x":1}]`, `[{"x":1}]`},
		{"[1.5,0xFF,[1],{}]", "[1.5,255,[1],{}]"},
	}
	for _, tt := range cases {
		out, err := FromJSONVariant([]byte(tt.in))
		if err != nil {
			t.Errorf("Decode(%q): unexpected error: %v", tt.in, err)
		}
		got := string(out)
		if tt.out != got {
			t.Errorf("Decode(%q): expected %q, got %q", tt.in, tt.out, got)
		}
	}
}

func TestDecodeImplicitComma(t *testing.T) {
	cases := []testcase{
		{`{"a":1 "b":2}`, `{"a":1,"b":2}`}, // adjacent object entries
		{"[1 2 3]", "[1,2,3]"},             // adjacent array values
		{"[[1][2]]", "[[1],[2]]"},          // adjacent containers in array
	}
	for _, tt := range cases {
		out, err := FromJSONVariant([]byte(tt.in))
		if err != nil {
			t.Errorf("Decode(%q): unexpected error: %v", tt.in, err)
		}
		got := string(out)
		if tt.out != got {
			t.Errorf("Decode(%q): expected %q, got %q", tt.in, tt.out, got)
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := []string{
		"[}",    // array closed with object brace
		"{]",    // object closed with array bracket
		`{"a"}`, // object key without colon
		"[:]",   // colon in array value position
	}
	for _, in := range cases {
		_, err := FromJSONVariant([]byte(in))
		if err == nil {
			t.Errorf("Decode(%q): expected error, got nil", in)
		}
	}
}

func TestDecodeNaNAndInfinity(t *testing.T) {
	cases := []string{
		"NaN",
		"+NaN",
		"-NaN",
		`{"x":NaN}`,
		`[1,NaN,2]`,
		"Infinity",
		"+Infinity",
		"-Infinity",
		`{"x":Infinity}`,
		`[1,Infinity,2]`,
	}
	for _, in := range cases {
		_, err := FromJSONVariant([]byte(in))
		if err == nil {
			t.Errorf("Decode(%q): expected error, got nil", in)
		}
	}
}

func TestJSON5ParseError(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		line   int
		column int
	}{
		// unterminated string: error at the line the string started
		{"unterminated string line 1", `"unclosed`, 1, 1},
		{"unterminated string line 3", "{\n  \"k\": \"v\",\n  \"bad\": \"unclosed\n}", 3, 10},
		// unescaped newline in double-quoted string: error at the offending line
		{"unescaped newline line 2", "{\n  \"bad\": \"has\nnewline\"}", 2, 10},
		// NaN: error at the line containing NaN
		{"NaN line 2", "[\n  NaN\n]", 2, 3},
		// hex overflow: error at the line containing the literal
		{"hex overflow line 2", "[\n  0x10000000000000000\n]", 2, 3},
		// unterminated block comment: error where the comment opened
		{"unterminated comment line 1", "/* never closed", 1, 1},
		{"unterminated comment line 2", "[1] /* never\nclosed", 1, 5},
		{"unterminated comment line 3", "{\n  \"k\": 1\n} /* open", 3, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FromJSONVariant([]byte(tc.input))
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
