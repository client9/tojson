// TOML-to-JSON support. Converts a subset of TOML to JSON without reflection
// or intermediate map[string]any structures.
//
// Supported: key-value pairs, standard tables [header], array-of-tables
// [[header]], inline tables {k=v}, inline arrays [v,v], all scalar types,
// dotted keys, comments.
//
// Not supported: TOML integers larger than int64.

package tojson

import (
	"bytes"
	"errors"
	"fmt"
)

// errReentry signals that the line parser detected an out-of-order section
// and the caller should fall back to tomlConvertTree.
var errReentry = errors.New("toml: out-of-order section")

func tomlConvert(input []byte) ([]byte, error) {
	out, err := fromTOMLLine(input)
	if errors.Is(err, errReentry) {
		return tomlConvertTree(input)
	}
	return out, err
}

// --------------------------------------------------------------------------
// Key path parsing
// --------------------------------------------------------------------------

// parseTOMLKeyPath parses a dotted key (e.g. a."b c".d) from the start of s.
// Returns the decoded key segments and the remainder of s after the last segment.
// buf is caller-provided backing storage (pass yourArray[:0]); avoids a heap alloc for ≤ cap(buf) keys.
func parseTOMLKeyPath(s []byte, buf [][]byte) ([][]byte, []byte, error) {
	keys := buf[:0]
	for {
		s = bytes.TrimLeft(s, " \t")
		if len(s) == 0 {
			break
		}
		var key []byte
		var rest []byte
		var err error
		switch s[0] {
		case '"':
			key, rest, err = parseTOMLBasicStringRaw(s)
			if err != nil {
				return nil, nil, err
			}
		case '\'':
			key, rest = parseTOMLLiteralStringRaw(s)
		default:
			// bare key: [A-Za-z0-9_-]+
			i := 0
			for i < len(s) {
				c := s[i]
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
					(c >= '0' && c <= '9') || c == '_' || c == '-' {
					i++
				} else {
					break
				}
			}
			if i == 0 {
				break
			}
			key = s[:i]
			rest = s[i:]
		}
		if len(key) == 0 && len(keys) == 0 {
			break
		}
		keys = append(keys, key)
		rest = bytes.TrimLeft(rest, " \t")
		if len(rest) == 0 || rest[0] != '.' {
			return keys, rest, nil
		}
		s = rest[1:] // consume the dot
	}
	if len(keys) == 0 {
		return nil, s, fmt.Errorf("empty key")
	}
	return keys, s, nil
}
