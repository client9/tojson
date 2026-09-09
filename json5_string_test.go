package tojson

import (
	"encoding/json"
	"testing"
)

func TestAppendRecodeString(t *testing.T) {
	cases := []testcase{
		{"abc", `"abc"`},
		{"1\n2", `"1\n2"`},
		{"1\t2", `"1\t2"`},
		{"\\x41", `"A"`},
		{"\\x0a", `"\n"`},
		{"\\x00", `"\u0000"`},
		{"\\xFF", `"\u00ff"`},
		{"\\n", `"\n"`},
		{"\\t", `"\t"`},
		{"\\b", `"\b"`},
		{"\\f", `"\f"`},
		{"\\r", `"\r"`},
		{"\\\\", `"\\"`},
		{"\\\"", `"\""`},
		{"\\a", `"\u0007"`},
		{"\\v", `"\u000b"`},
		{"\\u0041", `"\u0041"`},
		{"\r\n", `"\n"`},
		{"\r", `"\r"`},
		{"\u2028", `"\u2028"`},
		{"\u2029", `"\u2029"`},
		{"\xff", `"\ufffd"`},
	}
	for _, tt := range cases {
		dst := appendRecodeString(nil, []byte(tt.in))
		got := string(dst)
		if tt.out != got {
			t.Errorf("appendRecodeString(%q): expected %s, got %s", tt.in, tt.out, got)
		}
	}
}

// A backslash that is itself escaped does not escape what follows, so a string
// may end with one. The tokenizer used to run off the end of such a string.
func TestEscapedTrailingBackslash(t *testing.T) {
	cases := []struct{ in, out string }{
		{`"\\"`, `"\\"`},
		{`"\\\\"`, `"\\\\"`},
		{`"C:\\dir\\"`, `"C:\\dir\\"`},
		{`{"a":"x\\"}`, `{"a":"x\\"}`},
		{`{"k\\":""}`, `{"k\\":""}`},
		{`["a\\","b"]`, `["a\\","b"]`},
		{`"a\\b"`, `"a\\b"`},
		{`"a\\\"b"`, `"a\\\"b"`},
	}
	for _, tc := range cases {
		out, err := FromJSONVariant([]byte(tc.in))
		if err != nil {
			t.Errorf("FromJSONVariant(%s): unexpected error: %v", tc.in, err)
			continue
		}
		if got := string(out); got != tc.out {
			t.Errorf("FromJSONVariant(%s) = %s, want %s", tc.in, got, tc.out)
		}
		if !json.Valid(out) {
			t.Errorf("FromJSONVariant(%s) = %s, which is not JSON", tc.in, out)
		}
	}
}
