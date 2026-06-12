package tojson

import (
	"bytes"
	"fmt"
	"strconv"
)

// --------------------------------------------------------------------------
// YAML parser options
// --------------------------------------------------------------------------

// yamlTabWidth is the number of spaces a tab character counts as when
// measuring indentation. Set to <= 0 to forbid tabs in YAML input entirely.
const yamlTabWidth = 2

// yamlBoolAliases controls whether YAML 1.1 boolean aliases are recognised.
// When true, yes/no/on/off (and their case variants) map to true/false.
// When false, only true/false (and their case variants) are treated as booleans.
const yamlBoolAliases = false

// yamlTildeNull controls whether bare ~ is treated as null.
const yamlTildeNull = false

// writeScalar converts a YAML scalar to its JSON representation.
func writeScalar(s []byte, buf *bytes.Buffer) error {
	s = bytes.TrimSpace(s)
	switch string(s) {
	case "", "null", "Null", "NULL":
		buf.WriteString("null")
		return nil
	}
	if yamlTildeNull && string(s) == "~" {
		buf.WriteString("null")
		return nil
	}
	switch string(s) {
	case "true", "True", "TRUE":
		buf.WriteString("true")
		return nil
	case "false", "False", "FALSE":
		buf.WriteString("false")
		return nil
	}
	if yamlBoolAliases {
		switch string(s) {
		case "yes", "Yes", "YES", "on", "On", "ON":
			buf.WriteString("true")
			return nil
		case "no", "No", "NO", "off", "Off", "OFF":
			buf.WriteString("false")
			return nil
		}
	}

	if len(s) > 0 && s[0] == '"' {
		// Decode using Go string literal rules, then re-encode as JSON.
		// This handles \n \t \uNNNN \xNN etc.; YAML-specific escapes like
		// \e \N \L \P are outside the minimal YAML spec and not supported.
		str, err := strconv.Unquote(string(s))
		if err != nil {
			return fmt.Errorf("invalid double-quoted string: %w", err)
		}
		writeJSONString([]byte(str), buf)
		return nil
	}
	if len(s) > 0 && s[0] == '\'' {
		str := parseSingleQuoted(s)
		writeJSONString(str, buf)
		return nil
	}

	if isYAMLNumber(s) {
		writeNormalizedNumber(buf, s)
		return nil
	}

	writeJSONString(s, buf)
	return nil
}

// writeJSONString writes s as a properly escaped JSON string.
// Uses AvailableBuffer so that when buf has spare capacity no allocation is needed.
func writeJSONString(s []byte, buf *bytes.Buffer) {
	buf.Write(appendString(buf.AvailableBuffer(), s))
}

// --------------------------------------------------------------------------
// Quoted string parsers
// --------------------------------------------------------------------------

// doubleQuotedEnd returns the index just past the closing '"' in s,
// or -1 if the string is unterminated. s must start with '"'.
// Only used to locate the boundary; decoding is done by strconv.Unquote.
func doubleQuotedEnd(s []byte) int {
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++ // skip the escaped character
			continue
		}
		if s[i] == '"' {
			return i + 1
		}
	}
	return -1
}

// parseDoubleQuotedRaw decodes a double-quoted string at the start of s using
// Go string literal rules (strconv.Unquote) and returns the decoded content
// and the remainder of s after the closing '"'.
func parseDoubleQuotedRaw(s []byte) ([]byte, []byte, error) {
	end := doubleQuotedEnd(s)
	if end < 0 {
		return nil, s, fmt.Errorf("unterminated double-quoted string")
	}
	str, err := strconv.Unquote(string(s[:end]))
	if err != nil {
		return nil, s, fmt.Errorf("invalid double-quoted string: %w", err)
	}
	return []byte(str), s[end:], nil
}

func parseSingleQuoted(s []byte) []byte {
	str, _ := parseSingleQuotedRaw(s)
	return str
}

// --------------------------------------------------------------------------
// Line classification helpers
// --------------------------------------------------------------------------

func isSeqItem(content []byte) bool {
	return bytes.Equal(content, []byte("-")) || bytes.HasPrefix(content, []byte("- "))
}

// isMapKey returns true if content looks like a YAML mapping key line.
func isMapKey(content []byte) bool {
	if isSeqItem(content) {
		return false
	}
	if len(content) == 0 {
		return false
	}
	if content[0] == '{' || content[0] == '[' {
		return false
	}
	switch content[0] {
	case '"':
		// find the closing quote, then check for the required ': ' separator
		end := doubleQuotedEnd(content)
		return end >= 0 && end < len(content) && content[end] == ':'
	case '\'':
		i := 1
		for i < len(content) {
			if content[i] == '\'' {
				if i+1 < len(content) && content[i+1] == '\'' {
					i += 2
					continue
				}
				return i+1 < len(content) && content[i+1] == ':'
			}
			i++
		}
		return false
	}
	return bytes.Contains(content, []byte(": ")) || (len(content) > 0 && content[len(content)-1] == ':')
}

// splitMapKey splits "key: value" → ("key", "value"), or "key:" → ("key", nil).
func splitMapKey(content []byte) (key, value []byte, err error) {
	switch {
	case len(content) > 0 && content[0] == '"':
		k, rest, err := parseDoubleQuotedRaw(content)
		if err != nil {
			return nil, nil, err
		}
		rest = bytes.TrimPrefix(rest, []byte(":"))
		rest = bytes.TrimPrefix(rest, []byte(" "))
		return k, bytes.TrimSpace(rest), nil
	case len(content) > 0 && content[0] == '\'':
		k, rest := parseSingleQuotedRaw(content)
		rest = bytes.TrimPrefix(rest, []byte(":"))
		rest = bytes.TrimPrefix(rest, []byte(" "))
		return k, bytes.TrimSpace(rest), nil
	}
	if idx, after, ok := bytes.Cut(content, []byte(": ")); ok {
		return idx, bytes.TrimSpace(after), nil
	}
	if len(content) > 0 && content[len(content)-1] == ':' {
		return content[:len(content)-1], nil, nil
	}
	return content, nil, nil
}

// parseSingleQuotedRaw returns (unescaped bytes, remainder after closing quote).
func parseSingleQuotedRaw(s []byte) ([]byte, []byte) {
	if len(s) < 2 || s[0] != '\'' {
		return s, nil
	}
	// Fast path: no '' escape sequences — return a no-alloc sub-slice.
	for i := 1; i < len(s); i++ {
		if s[i] == '\'' {
			if i+1 < len(s) && s[i+1] == '\'' {
				break // has '' escape, fall through to slow path
			}
			return s[1:i], s[i+1:]
		}
	}
	// Slow path: has '' escapes, must decode.
	var b bytes.Buffer
	i := 1
	for i < len(s) {
		if s[i] == '\'' {
			if i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte('\'')
				i += 2
				continue
			}
			return b.Bytes(), s[i+1:]
		}
		b.WriteByte(s[i])
		i++
	}
	return b.Bytes(), nil
}

// --------------------------------------------------------------------------
// Misc helpers
// --------------------------------------------------------------------------

func leadingSpaces(s []byte) int {
	n := 0
	for _, c := range s {
		if c == ' ' {
			n++
		} else if c == '\t' {
			n += 2
		} else {
			break
		}
	}
	return n
}

// yamlLeadingIndent counts the indentation of s using yamlTabWidth for tabs.
// Returns an error if yamlTabWidth < 0 and s contains a leading tab.
func yamlLeadingIndent(s []byte) (int, error) {
	n := 0
	for _, c := range s {
		if c == ' ' {
			n++
		} else if c == '\t' {
			if yamlTabWidth <= 0 {
				return 0, fmt.Errorf("tab character not allowed in YAML indentation")
			}
			n += yamlTabWidth
		} else {
			break
		}
	}
	return n, nil
}

// isYAMLNumber returns true for decimal integers and floats:
//
//	integer: [-+]?(0|[1-9][0-9]*)
//	float:   [-+]?[0-9]*\.[0-9]*([eE][-+]?[0-9]+)?
//
// Leading + is accepted here; writeScalar strips it before writing output.
func isYAMLNumber(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	i := 0
	if s[i] == '-' || s[i] == '+' {
		i++
	}
	if i >= len(s) {
		return false
	}
	hasDigit := false
	if s[i] >= '0' && s[i] <= '9' {
		hasDigit = true
		if s[i] == '0' {
			i++
			// leading zero: valid only as bare 0 or start of float (0.5)
			if i < len(s) && s[i] >= '0' && s[i] <= '9' {
				return false
			}
		} else {
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
		}
	}
	if i < len(s) && s[i] == '.' && !hasDigit {
		// leading dot: .5 is valid
		hasDigit = true
	}
	if !hasDigit {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	return i == len(s)
}

// stripInlineComment removes a # comment from a YAML content line, respecting
// quotes. YAML requires # to be preceded by whitespace; bare # inside a value
// is not a comment.
func stripInlineComment(s []byte) []byte {
	inDouble := false
	inSingle := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inDouble:
			if c == '\\' {
				i++
			} else if c == '"' {
				inDouble = false
			}
		case inSingle:
			if c == '\'' && i+1 < len(s) && s[i+1] == '\'' {
				i++
			} else if c == '\'' {
				inSingle = false
			}
		case c == '"':
			inDouble = true
		case c == '\'':
			inSingle = true
		case c == '#':
			if i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
				return bytes.TrimRight(s[:i], " \t")
			}
		}
	}
	return s
}

