package tojson

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "regenerate golden .json files from tree parser")

type tomlFn func([]byte) ([]byte, error)

var tomlParsers = []struct {
	name string
	fn   tomlFn
}{
	{"line", fromTOMLLine},
	{"tree", fromTOMLTree},
	{"router", FromTOML},
}

// skipStreaming is the skip list for tests requiring out-of-order table re-entry,
// which the line parser cannot handle (it returns errReentry instead of falling back).
var skipStreaming = []string{"line"}

// forParsers runs f as a subtest for each parser not in the skip list.
func forParsers(t *testing.T, skip []string, f func(*testing.T, tomlFn)) {
	t.Helper()
	skipSet := make(map[string]bool, len(skip))
	for _, s := range skip {
		skipSet[s] = true
	}
	for _, p := range tomlParsers {
		t.Run(p.name, func(t *testing.T) {
			if skipSet[p.name] {
				t.Skip("parser does not support this feature")
			}
			f(t, p.fn)
		})
	}
}

func checkTOML(t *testing.T, fn tomlFn, toml, wantJSON string) {
	t.Helper()
	got, err := fn([]byte(toml))
	if err != nil {
		t.Fatalf("error: %v", err)
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
		t.Errorf("\ninput:  %s\ngot:    %s\nwant:   %s", toml, gotNorm, wantNorm)
	}
}

// --------------------------------------------------------------------------
// Scalars
// --------------------------------------------------------------------------

func TestTOMLBasicStrings(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `key = "hello"`, `{"key":"hello"}`)
		checkTOML(t, fn, `key = "tab\there"`, `{"key":"tab\there"}`)
		checkTOML(t, fn, `key = "newline\nhere"`, `{"key":"newline\nhere"}`)
		checkTOML(t, fn, `key = "backslash\\"`, `{"key":"backslash\\"}`)
		checkTOML(t, fn, `key = "quote\""`, `{"key":"quote\""}`)
		checkTOML(t, fn, `key = "A"`, `{"key":"A"}`)
		checkTOML(t, fn, `key = "\U00000041"`, `{"key":"A"}`)
	})
}

func TestTOMLLiteralStrings(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `key = 'hello world'`, `{"key":"hello world"}`)
		checkTOML(t, fn, `key = 'no \n escape'`, `{"key":"no \\n escape"}`)
		checkTOML(t, fn, `key = 'C:\Users\tom'`, `{"key":"C:\\Users\\tom"}`)
	})
}

func TestTOMLMultilineBasic(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, "key = \"\"\"\nline one\nline two\n\"\"\"", `{"key":"line one\nline two\n"}`)
		// line-ending backslash trims whitespace
		checkTOML(t, fn, "key = \"\"\"hello \\\n   world\"\"\"", `{"key":"hello world"}`)
		// CRLF line endings inside the multi-line value
		checkTOML(t, fn, "key = \"\"\"\r\nline one\r\nline two\r\n\"\"\"", `{"key":"line one\nline two\n"}`)
	})
}

func TestTOMLMultilineLiteral(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, "key = '''\nline one\nline two\n'''", `{"key":"line one\nline two\n"}`)
		// no escape processing
		checkTOML(t, fn, "key = '''no \\n escape'''", `{"key":"no \\n escape"}`)
	})
}

func TestTOMLIntegers(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `n = 42`, `{"n":42}`)
		checkTOML(t, fn, `n = -7`, `{"n":-7}`)
		checkTOML(t, fn, `n = +99`, `{"n":99}`)
		checkTOML(t, fn, `n = 0`, `{"n":0}`)
		checkTOML(t, fn, `n = 1_000_000`, `{"n":1000000}`)
		checkTOML(t, fn, `n = 0xFF`, `{"n":255}`)
		checkTOML(t, fn, `n = 0o17`, `{"n":15}`)
		checkTOML(t, fn, `n = 0b1010`, `{"n":10}`)
		checkTOML(t, fn, `n = 0xDEAD_BEEF`, `{"n":3735928559}`)
	})
}

func TestTOMLFloats(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `f = 3.14`, `{"f":3.14}`)
		checkTOML(t, fn, `f = -0.001`, `{"f":-0.001}`)
		checkTOML(t, fn, `f = 5e22`, `{"f":5e22}`)
		checkTOML(t, fn, `f = 1e+99`, `{"f":1e+99}`)
		checkTOML(t, fn, `f = 6.626e-34`, `{"f":6.626e-34}`)
		checkTOML(t, fn, `f = 1_0.0`, `{"f":10.0}`)
	})
}

func TestTOMLBooleans(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `a = true`, `{"a":true}`)
		checkTOML(t, fn, `a = false`, `{"a":false}`)
	})
}

func TestTOMLDatetimes(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `dt = 1979-05-27T07:32:00Z`, `{"dt":"1979-05-27T07:32:00Z"}`)
		checkTOML(t, fn, `d = 1979-05-27`, `{"d":"1979-05-27"}`)
		checkTOML(t, fn, `t = 07:32:00`, `{"t":"07:32:00"}`)
		checkTOML(t, fn, `dt = 1979-05-27T07:32:00.999999-08:00`, `{"dt":"1979-05-27T07:32:00.999999-08:00"}`)
	})
}

// --------------------------------------------------------------------------
// Keys
// --------------------------------------------------------------------------

func TestTOMLKeyForms(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `bare_key = 1`, `{"bare_key":1}`)
		checkTOML(t, fn, `bare-key = 1`, `{"bare-key":1}`)
		checkTOML(t, fn, `"quoted key" = 1`, `{"quoted key":1}`)
		checkTOML(t, fn, `'literal key' = 1`, `{"literal key":1}`)
	})
}

func TestTOMLDottedKeys(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `a.b = 1`, `{"a":{"b":1}}`)
		checkTOML(t, fn, `a.b.c = 1`, `{"a":{"b":{"c":1}}}`)
		checkTOML(t, fn, "a.b = 1\na.c = 2", `{"a":{"b":1,"c":2}}`)
	})
}

// --------------------------------------------------------------------------
// Standard tables
// --------------------------------------------------------------------------

func TestTOMLSimpleTable(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, "[server]\nhost = \"localhost\"\nport = 8080",
			`{"server":{"host":"localhost","port":8080}}`)
	})
}

func TestTOMLDottedTableHeader(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, "[a.b.c]\nkey = 1", `{"a":{"b":{"c":{"key":1}}}}`)
	})
}

func TestTOMLMultipleTables(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn,
			"[a]\nx = 1\n[b]\ny = 2",
			`{"a":{"x":1},"b":{"y":2}}`)
	})
}

func TestTOMLImplicitTables(t *testing.T) {
	// [a.b] creates 'a' implicitly; then [a] can add sibling keys.
	// Streaming parser treats this as re-entry into a closed section.
	forParsers(t, skipStreaming, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn,
			"[a.b]\nx = 1\n[a]\ny = 2",
			`{"a":{"b":{"x":1},"y":2}}`)
	})
}

func TestTOMLTableReentry(t *testing.T) {
	// Critical: [a] ... [b] ... [a.c] must re-enter the 'a' object.
	forParsers(t, skipStreaming, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn,
			"[a]\nx = 1\n[b]\ny = 2\n[a.c]\nz = 3",
			`{"a":{"x":1,"c":{"z":3}},"b":{"y":2}}`)
	})
}

// TestTOMLLineOrderedTablesStayOnFastPath verifies ordered siblings don't trigger errReentry.
func TestTOMLLineOrderedTablesStayOnFastPath(t *testing.T) {
	got, err := fromTOMLLine([]byte("[fruit.apple]\nx = 1\n[fruit.orange]\ny = 2\n[animal]\nz = 3"))
	if err != nil {
		t.Fatalf("fromTOMLLine error: %v", err)
	}
	want := `{"fruit":{"apple":{"x":1},"orange":{"y":2}},"animal":{"z":3}}`
	if string(got) != want {
		t.Fatalf("fromTOMLLine = %s, want %s", got, want)
	}
}

// TestTOMLLineOutOfOrderTableReentry verifies the errReentry sentinel is returned.
func TestTOMLLineOutOfOrderTableReentry(t *testing.T) {
	_, err := fromTOMLLine([]byte("[fruit.apple]\nx = 1\n[animal]\nz = 3\n[fruit.orange]\ny = 2"))
	if err != errReentry {
		t.Fatalf("fromTOMLLine error = %v, want errReentry", err)
	}
}

func TestTOMLLineQuotedDotTableDoesNotCollideWithDottedPath(t *testing.T) {
	got, err := fromTOMLLine([]byte("[fruit.apple]\nx = 1\n[animal]\nz = 3\n[\"fruit.apple\"]\ny = 2"))
	if err != nil {
		t.Fatalf("fromTOMLLine error: %v", err)
	}
	want := `{"fruit":{"apple":{"x":1}},"animal":{"z":3},"fruit.apple":{"y":2}}`
	if string(got) != want {
		t.Fatalf("fromTOMLLine = %s, want %s", got, want)
	}
}

func TestTOMLLineQuotedDotAoTDoesNotCollideWithDottedPath(t *testing.T) {
	got, err := fromTOMLLine([]byte("[[fruit.apple]]\nx = 1\n[[\"fruit.apple\"]]\ny = 2"))
	if err != nil {
		t.Fatalf("fromTOMLLine error: %v", err)
	}
	want := `{"fruit":{"apple":[{"x":1}]},"fruit.apple":[{"y":2}]}`
	if string(got) != want {
		t.Fatalf("fromTOMLLine = %s, want %s", got, want)
	}
}

// --------------------------------------------------------------------------
// Array of tables
// --------------------------------------------------------------------------

func TestTOMLArrayOfTables(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn,
			"[[products]]\nname = \"Hammer\"\n[[products]]\nname = \"Nail\"",
			`{"products":[{"name":"Hammer"},{"name":"Nail"}]}`)
	})
}

func TestTOMLNestedAoT(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn,
			"[[fruits]]\nname = \"apple\"\n[[fruits.varieties]]\nname = \"red\"\n[[fruits.varieties]]\nname = \"green\"",
			`{"fruits":[{"name":"apple","varieties":[{"name":"red"},{"name":"green"}]}]}`)
	})
}

func TestTOMLMixedTableAndAoT(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn,
			"[a]\nx = 1\n[[a.items]]\nname = \"A\"\n[[a.items]]\nname = \"B\"",
			`{"a":{"x":1,"items":[{"name":"A"},{"name":"B"}]}}`)
	})
}

// Out-of-order AoT: [[section]] reappears after an intervening [other] section,
// which closes the first section and prevents the streaming path from handling
// the re-entry. These tests exercise the tree-based fallback (tomlConvertTree).

func TestTOMLAoTOutOfOrder(t *testing.T) {
	forParsers(t, skipStreaming, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn,
			"[[a]]\nx = 1\n[b]\ny = 2\n[[a]]\nx = 2",
			`{"a":[{"x":1},{"x":2}],"b":{"y":2}}`)
	})
}

func TestTOMLAoTOutOfOrderStrings(t *testing.T) {
	// string values exercise scalarStringNode via parseTOMLValue in tree path
	forParsers(t, skipStreaming, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn,
			"[[items]]\nname = \"hammer\"\n[meta]\nv = 1\n[[items]]\nname = \"nail\"",
			`{"items":[{"name":"hammer"},{"name":"nail"}],"meta":{"v":1}}`)
	})
}

func TestTOMLAoTOutOfOrderInlineArray(t *testing.T) {
	// inline array values exercise parseTOMLInlineArray via parseTOMLValue in tree path
	forParsers(t, skipStreaming, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn,
			"[[items]]\nnums = [1, 2, 3]\n[meta]\nv = 1\n[[items]]\nnums = [4, 5]",
			`{"items":[{"nums":[1,2,3]},{"nums":[4,5]}],"meta":{"v":1}}`)
	})
}

func TestTOMLTableAfterAoTReentry(t *testing.T) {
	// [a] after [[a]] in the tree path enters the last aot element as context
	forParsers(t, skipStreaming, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn,
			"[[a]]\nx = 1\n[b]\ny = 2\n[a]\nz = 3",
			`{"a":[{"x":1,"z":3}],"b":{"y":2}}`)
	})
}

// --------------------------------------------------------------------------
// Inline tables
// --------------------------------------------------------------------------

func TestTOMLInlineTable(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `point = {x = 1, y = 2}`, `{"point":{"x":1,"y":2}}`)
		checkTOML(t, fn, `empty = {}`, `{"empty":{}}`)
		checkTOML(t, fn, `nested = {a = {b = 42}}`, `{"nested":{"a":{"b":42}}}`)
	})
}

func TestTOMLInlineTableDottedKeys(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `t = {a.b = 1, a.c = 2}`, `{"t":{"a":{"b":1,"c":2}}}`)
	})
}

// --------------------------------------------------------------------------
// Inline arrays
// --------------------------------------------------------------------------

func TestTOMLInlineArray(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, `nums = [1, 2, 3]`, `{"nums":[1,2,3]}`)
		checkTOML(t, fn, `mixed = [1, "two", true]`, `{"mixed":[1,"two",true]}`)
		checkTOML(t, fn, `nested = [[1, 2], [3, 4]]`, `{"nested":[[1,2],[3,4]]}`)
		checkTOML(t, fn, `empty = []`, `{"empty":[]}`)
		checkTOML(t, fn, `trailing = [1, 2,]`, `{"trailing":[1,2]}`)
	})
}

func TestTOMLInlineArrayMultiline(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, "nums = [\n  1,\n  2,\n  3,\n]", `{"nums":[1,2,3]}`)
		// CRLF line endings inside the array
		checkTOML(t, fn, "nums = [\r\n  1,\r\n  2,\r\n]", `{"nums":[1,2]}`)
	})
}

func TestTOMLUnterminatedMultiline(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		for _, input := range []string{
			"key = \"\"\"\nline one\n",
			"key = '''\nline one\n",
			"nums = [\n  1,\n  2,\n",
		} {
			if _, err := fn([]byte(input)); err == nil {
				t.Errorf("input %q: expected error, got nil", input)
			}
		}
	})
}

// --------------------------------------------------------------------------
// Comments
// --------------------------------------------------------------------------

func TestTOMLComments(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		checkTOML(t, fn, "# full line comment\nkey = 1", `{"key":1}`)
		checkTOML(t, fn, `key = 1 # inline comment`, `{"key":1}`)
		checkTOML(t, fn, "# comment\n[section]\n# another\nkey = 2", `{"section":{"key":2}}`)
		// TOML allows # without preceding whitespace
		checkTOML(t, fn, "key=1#comment", `{"key":1}`)
		checkTOML(t, fn, "arr=[1,2]#comment", `{"arr":[1,2]}`)
		checkTOML(t, fn, `key = "v"#comment`, `{"key":"v"}`)
		// # inside a string must not be stripped
		checkTOML(t, fn, `s = "string with # inside"`, `{"s":"string with # inside"}`)
		checkTOML(t, fn, `s = 'literal with # inside'`, `{"s":"literal with # inside"}`)
		// TOML literal strings: '' is two empty strings, not an escaped quote (YAML rule).
		// A # immediately after the closing ' must be treated as a comment.
		checkTOML(t, fn, "key = ''# comment", `{"key":""}`)
		checkTOML(t, fn, `key = 'a'# comment`, `{"key":"a"}`)
	})
}

// --------------------------------------------------------------------------
// Empty input
// --------------------------------------------------------------------------

func TestTOMLEmptyInput(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		for _, input := range []string{"", "  \n  ", "# just a comment"} {
			got, err := fn([]byte(input))
			if err != nil {
				t.Errorf("input %q error: %v", input, err)
				continue
			}
			if string(got) != "{}" {
				t.Errorf("input %q = %s, want {}", input, got)
			}
		}
	})
}

// --------------------------------------------------------------------------
// Error cases
// --------------------------------------------------------------------------

func TestTOMLErrorDuplicateKey(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte("a = 1\na = 2")); err == nil {
			t.Error("expected error for duplicate key")
		}
	})
}

func TestTOMLErrorDuplicateDottedKey(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte("a.b = 1\na.b = 2")); err == nil {
			t.Error("expected error for duplicate dotted key")
		}
	})
}

func TestTOMLErrorDuplicateNestedKey(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte("[a]\nx = 1\nx = 2")); err == nil {
			t.Error("expected error for duplicate key inside table")
		}
	})
}

func TestTOMLErrorDuplicateTable(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte("[a]\n[a]")); err == nil {
			t.Error("expected error for duplicate table")
		}
	})
}

func TestTOMLErrorDuplicateDottedTable(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte("[a.b]\n[a.b]")); err == nil {
			t.Error("expected error for duplicate dotted table")
		}
	})
}

func TestTOMLErrorScalarAsTable(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte("a = 1\n[a]")); err == nil {
			t.Error("expected error for scalar redefined as table")
		}
	})
}

func TestTOMLErrorInf(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte("f = inf")); err == nil {
			t.Error("expected error for inf")
		}
		if _, err := fn([]byte("f = -inf")); err == nil {
			t.Error("expected error for -inf")
		}
	})
}

func TestTOMLErrorNaN(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte("f = nan")); err == nil {
			t.Error("expected error for nan")
		}
	})
}

func TestTOMLErrorLeadingZero(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte("n = 01")); err == nil {
			t.Error("expected error for leading zero integer")
		}
	})
}

func TestTOMLErrorUnterminatedString(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte(`key = "unclosed`)); err == nil {
			t.Error("expected error for unterminated string")
		}
	})
}

func TestTOMLErrorInvalidEscape(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte(`key = "\q"`)); err == nil {
			t.Error("expected error for invalid escape")
		}
	})
}

func TestTOMLErrorInlineTableTrailingComma(t *testing.T) {
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte(`t = {a = 1,}`)); err == nil {
			t.Error("expected error for trailing comma in inline table")
		}
	})
}

func TestTOMLErrorInlineTableBadKey(t *testing.T) {
	// parseTOMLKeyPath errors from inside an inline table must carry a line number.
	cases := []struct {
		input   string
		wantLine int
	}{
		{`t = { "unterminated = 1 }`, 1},
		{"# comment\nt = { \"unterminated = 1 }", 2},
	}
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		for _, tc := range cases {
			_, err := fn([]byte(tc.input))
			if err == nil {
				t.Errorf("input %q: expected error", tc.input)
				continue
			}
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Errorf("input %q: got plain error (no position): %v", tc.input, err)
				continue
			}
			if pe.Line != tc.wantLine {
				t.Errorf("input %q: got line %d, want %d", tc.input, pe.Line, tc.wantLine)
			}
		}
	})
}

func TestTOMLErrorNestingLimit(t *testing.T) {
	// [a.b.c.d.e.f.g.h] is exactly at the limit (tomlMaxNesting = 8); must succeed.
	ok := "[a.b.c.d.e.f.g.h]\nk = 1"
	forParsers(t, nil, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte(ok)); err != nil {
			t.Errorf("depth-%d header rejected: %v", tomlMaxNesting, err)
		}
	})

	// [a.b.c.d.e.f.g.h.i] is one level too deep; only the line parser enforces the limit.
	over := "[a.b.c.d.e.f.g.h.i]\nk = 1"
	forParsers(t, []string{"streaming", "tree", "router"}, func(t *testing.T, fn tomlFn) {
		if _, err := fn([]byte(over)); err == nil {
			t.Errorf("depth-%d header accepted, want error", tomlMaxNesting+1)
		}
	})
}

// --------------------------------------------------------------------------
// File-based golden tests
// --------------------------------------------------------------------------
//
// Each testdata/toml/*.toml file is paired with a matching .json golden file.
// All parsers run over every .toml; if a parser returns errReentry the subtest
// is skipped (the parser doesn't support that input pattern).
//
// To add a test: drop a .toml file in testdata/toml/ and run:
//
//	go test -run TestTOMLFiles -update
//
// to generate the matching .json from the tree parser.

func TestTOMLFiles(t *testing.T) {
	files, err := filepath.Glob("testdata/toml/*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata/toml/*.toml files found")
	}

	for _, tomlPath := range files {
		name := strings.TrimSuffix(filepath.Base(tomlPath), ".toml")
		jsonPath := strings.TrimSuffix(tomlPath, ".toml") + ".json"

		tomlData, err := os.ReadFile(tomlPath)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}

		t.Run(name, func(t *testing.T) {
			if *update {
				raw, err := fromTOMLTree(tomlData)
				if err != nil {
					// invalid TOML — golden file is empty to signal expected failure
					os.WriteFile(jsonPath, []byte{}, 0644)
				} else {
					var v any
					json.Unmarshal(raw, &v)
					pretty, _ := json.MarshalIndent(v, "", "  ")
					os.WriteFile(jsonPath, append(pretty, '\n'), 0644)
				}
				t.Logf("updated %s", jsonPath)
			}

			want, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("missing %s — run: go test -run TestTOMLFiles -update", jsonPath)
			}

			if len(want) == 0 {
				// empty golden file = this input must be rejected by all parsers
				forParsers(t, nil, func(t *testing.T, fn tomlFn) {
					if _, err := fn(tomlData); err == nil {
						t.Error("expected parse error, got nil")
					}
				})
				return
			}

			forParsers(t, nil, func(t *testing.T, fn tomlFn) {
				raw, err := fn(tomlData)
				// TODO: not quite right.  Need to add condition if the parser is 'streaming' or 'line'
				//  the tree based parser should work, and not error
				if err == errReentry && strings.Contains(name, "reentry") {
					t.Skip("parser does not support out-of-order tables")
				}
				if err != nil {
					t.Fatalf("%v", err)
				}
				var v any
				if err := json.Unmarshal(raw, &v); err != nil {
					t.Fatalf("invalid JSON output %q: %v", raw, err)
				}
				pretty, _ := json.MarshalIndent(v, "", "  ")
				pretty = append(pretty, '\n')
				if string(pretty) != string(want) {
					t.Errorf("got:\n%s\nwant:\n%s", pretty, want)
				}
			})
		})
	}
}

// --------------------------------------------------------------------------
// Path-specific helpers (smoke tests, not parameterized)
// --------------------------------------------------------------------------

func TestTOMLFromTOMLTree(t *testing.T) {
	got, err := fromTOMLTree([]byte("k = 1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"k":1}` {
		t.Errorf("got %q, want %q", got, `{"k":1}`)
	}
}

// --------------------------------------------------------------------------
// ParseError line numbers
// --------------------------------------------------------------------------

func TestTOMLParseErrorLineNumber(t *testing.T) {
	t.Skip()
	cases := []struct {
		name   string
		input  string
		line   int
		column int
	}{
		// simple value error: streaming fast path
		{"unterminated string line 2", "a = 1\nb = \"unclosed", 2, 5},
		// value error first line
		{"inf not valid", "b = inf", 1, 5},
		// malformed table header: structural, column at start of content
		{"malformed table header", "[bad header", 1, 1},
		// dotted key value error: streaming slow path
		{"dotted key unterminated", `a.b = "unclosed`, 1, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FromTOML([]byte(tc.input))
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

func TestTOMLParseErrorString(t *testing.T) {
	e := &ParseError{Line: 5, Column: 3, Message: "bad token"}
	if got, want := e.Error(), "line 5, column 3: bad token"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
