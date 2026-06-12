package tojson

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode/utf8"
)

// parseUnicodeEscape decodes a 4-hex-digit YAML/TOML \uNNNN escape sequence.
func parseUnicodeEscape(hex4 []byte) (rune, error) {
	var r rune
	for _, c := range hex4 {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		default:
			return 0, fmt.Errorf("invalid hex digit %q", c)
		}
	}
	return r, nil
}

// scalarStringNode encodes s as a JSON string and returns it as a scalar jnode.
func scalarStringNode(s []byte) *jnode {
	return &jnode{raw: appendString(make([]byte, 0, len(s)+2), s)}
}

// parseTOMLValue parses a TOML value from s.
// rawLines/lineIdx are needed for multiline strings.
// Returns pre-encoded JSON bytes, number of additional lines consumed, and error.
func parseTOMLValue(s []byte, rawLines [][]byte, lineIdx int) (*jnode, int, error) {
	s = bytes.TrimSpace(s)
	if len(s) == 0 {
		return nil, 0, fmt.Errorf("expected value")
	}

	if bytes.HasPrefix(s, []byte(`"""`)) {
		str, consumed, err := parseTOMLMultilineBasic(s, rawLines, lineIdx)
		if err != nil {
			return nil, 0, err
		}
		return scalarStringNode(str), consumed, nil
	}

	if s[0] == '"' {
		str, _, err := parseTOMLBasicStringRaw(s)
		if err != nil {
			return nil, 0, err
		}
		return scalarStringNode(str), 0, nil
	}

	if bytes.HasPrefix(s, []byte("'''")) {
		str, consumed, err := parseTOMLMultilineLiteral(s, rawLines, lineIdx)
		if err != nil {
			return nil, 0, err
		}
		return scalarStringNode(str), consumed, nil
	}

	if s[0] == '\'' {
		str, _ := parseTOMLLiteralStringRaw(s)
		return scalarStringNode(str), 0, nil
	}

	if s[0] == '{' {
		node, _, err := parseTOMLInlineTable(s, 0)
		if err != nil {
			return nil, 0, err
		}
		return node, 0, nil
	}

	if s[0] == '[' {
		node, _, consumed, err := parseTOMLInlineArray(s, rawLines, lineIdx, 0)
		if err != nil {
			return nil, 0, err
		}
		return node, consumed, nil
	}

	if bytes.Equal(s, []byte("true")) {
		return nodeTrue, 0, nil
	}
	if bytes.Equal(s, []byte("false")) {
		return nodeFalse, 0, nil
	}

	if bytes.EqualFold(s, []byte("inf")) || bytes.EqualFold(s, []byte("+inf")) || bytes.EqualFold(s, []byte("-inf")) {
		return nil, 0, fmt.Errorf("inf is not representable in JSON")
	}
	if bytes.EqualFold(s, []byte("nan")) || bytes.EqualFold(s, []byte("+nan")) || bytes.EqualFold(s, []byte("-nan")) {
		return nil, 0, fmt.Errorf("nan is not representable in JSON")
	}

	if isTOMLDateTime(s) {
		return scalarStringNode(s), 0, nil
	}

	raw, err := parseTOMLNumber(s)
	if err != nil {
		return nil, 0, err
	}
	return newScalarNode(raw), 0, nil
}

// --------------------------------------------------------------------------
// String parsers
// --------------------------------------------------------------------------

// applyTOMLEscape processes a TOML escape sequence. i points to the character
// immediately after the backslash within s. The decoded rune is written to b.
// Returns the number of additional characters consumed beyond s[i], or an error.
func applyTOMLEscape(s []byte, i int, b *bytes.Buffer) (int, error) {
	if i >= len(s) {
		return 0, fmt.Errorf("unexpected end of string after backslash")
	}
	switch s[i] {
	case 'b':
		b.WriteByte('\b')
	case 't':
		b.WriteByte('\t')
	case 'n':
		b.WriteByte('\n')
	case 'f':
		b.WriteByte('\f')
	case 'r':
		b.WriteByte('\r')
	case '"':
		b.WriteByte('"')
	case '\\':
		b.WriteByte('\\')
	case 'u': // \uXXXX
		if i+4 >= len(s) {
			return 0, fmt.Errorf("invalid \\u escape: too short")
		}
		r, err := parseUnicodeEscape(s[i+1 : i+5])
		if err != nil {
			return 0, fmt.Errorf("invalid \\u escape: %w", err)
		}
		// surrogate pair \uHigh\uLow
		if r >= 0xD800 && r <= 0xDBFF && i+10 < len(s) && s[i+5] == '\\' && s[i+6] == 'u' {
			r2, err2 := parseUnicodeEscape(s[i+7 : i+11])
			if err2 == nil && r2 >= 0xDC00 && r2 <= 0xDFFF {
				r = 0x10000 + (r-0xD800)<<10 + (r2 - 0xDC00)
				b.WriteRune(r)
				return 10, nil
			}
		}
		b.WriteRune(r)
		return 4, nil
	case 'U': // \UXXXXXXXX
		if i+8 >= len(s) {
			return 0, fmt.Errorf("invalid \\U escape: too short")
		}
		hi, err1 := parseUnicodeEscape(s[i+1 : i+5])
		if err1 != nil {
			return 0, fmt.Errorf("invalid \\U escape: %w", err1)
		}
		lo, err2 := parseUnicodeEscape(s[i+5 : i+9])
		if err2 != nil {
			return 0, fmt.Errorf("invalid \\U escape: %w", err2)
		}
		r := (rune(hi) << 16) | rune(lo)
		if !utf8.ValidRune(r) {
			return 0, fmt.Errorf("invalid \\U escape: codepoint %U is not valid Unicode", r)
		}
		b.WriteRune(r)
		return 8, nil
	default:
		return 0, fmt.Errorf("invalid escape \\%c", s[i])
	}
	return 0, nil
}

// parseTOMLBasicStringRaw parses a TOML basic (double-quoted) string from s[0]
// using Go string literal rules (strconv.Unquote).
// Returns the decoded bytes and the remainder after the closing quote.
func parseTOMLBasicStringRaw(s []byte) ([]byte, []byte, error) {
	if len(s) < 2 || s[0] != '"' {
		return nil, s, fmt.Errorf("expected double-quoted string")
	}
	end := doubleQuotedEnd(s)
	if end < 0 {
		return nil, s, fmt.Errorf("unterminated basic string")
	}
	str, err := strconv.Unquote(string(s[:end]))
	if err != nil {
		return nil, s, fmt.Errorf("invalid basic string: %w", err)
	}
	return []byte(str), s[end:], nil
}

// parseTOMLLiteralStringRaw parses a TOML literal (single-quoted) string from s[0].
// No escape processing. Returns the raw content and the remainder.
func parseTOMLLiteralStringRaw(s []byte) ([]byte, []byte) {
	if len(s) < 2 || s[0] != '\'' {
		return s, nil
	}
	i := 1
	for i < len(s) {
		if s[i] == '\'' {
			return s[1:i], s[i+1:]
		}
		i++
	}
	return s[1:], nil
}

// parseTOMLMultilineBasic parses a triple-double-quoted multiline basic string.
// s is the portion of the current line starting at the opening """.
// Returns the decoded bytes and the number of additional lines consumed.
func parseTOMLMultilineBasic(s []byte, rawLines [][]byte, lineIdx int) ([]byte, int, error) {
	if !bytes.HasPrefix(s, []byte(`"""`)) {
		return nil, 0, fmt.Errorf("expected \"\"\"")
	}
	// Use 3-index slice to prevent appending into caller's buffer.
	content := s[3:len(s):len(s)]
	extraLines := 0
	for {
		if idx := bytes.Index(content, []byte(`"""`)); idx >= 0 {
			body := content[:idx]
			str, _, err := decodeTOMLMultilineBasic(body)
			if err != nil {
				return nil, 0, err
			}
			if tail := bytes.TrimLeft(content[idx+3:], " \t\r"); len(tail) > 0 && tail[0] != '#' {
				return nil, 0, fmt.Errorf("unexpected content after closing \"\"\"")
			}
			return str, extraLines, nil
		}
		nextIdx := lineIdx + extraLines + 1
		if nextIdx >= len(rawLines) {
			return nil, 0, fmt.Errorf("unterminated multiline basic string")
		}
		content = append(content, '\n')
		content = append(content, rawLines[nextIdx]...)
		extraLines++
	}
}

func decodeTOMLMultilineBasic(s []byte) ([]byte, int, error) {
	if bytes.HasPrefix(s, []byte("\n")) {
		s = s[1:]
	} else if bytes.HasPrefix(s, []byte("\r\n")) {
		s = s[2:]
	}
	var b bytes.Buffer
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			next := s[i+1]
			if next == '\n' || next == '\r' || next == ' ' || next == '\t' {
				i++
				for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
					i++
				}
				continue
			}
			i++
			extra, err := applyTOMLEscape(s, i, &b)
			if err != nil {
				return nil, 0, err
			}
			i += extra
		} else {
			b.WriteByte(c)
		}
		i++
	}
	return b.Bytes(), 0, nil
}

// parseTOMLMultilineLiteral parses a triple-single-quoted multiline literal string.
func parseTOMLMultilineLiteral(s []byte, rawLines [][]byte, lineIdx int) ([]byte, int, error) {
	if !bytes.HasPrefix(s, []byte("'''")) {
		return nil, 0, fmt.Errorf("expected '''")
	}
	content := s[3:len(s):len(s)]
	extraLines := 0
	for {
		if idx := bytes.Index(content, []byte("'''")); idx >= 0 {
			body := content[:idx]
			if bytes.HasPrefix(body, []byte("\n")) {
				body = body[1:]
			} else if bytes.HasPrefix(body, []byte("\r\n")) {
				body = body[2:]
			}
			if tail := bytes.TrimLeft(content[idx+3:], " \t\r"); len(tail) > 0 && tail[0] != '#' {
				return nil, 0, fmt.Errorf("unexpected content after closing '''")
			}
			return body, extraLines, nil
		}
		nextIdx := lineIdx + extraLines + 1
		if nextIdx >= len(rawLines) {
			return nil, 0, fmt.Errorf("unterminated multiline literal string")
		}
		content = append(content, '\n')
		content = append(content, rawLines[nextIdx]...)
		extraLines++
	}
}

// --------------------------------------------------------------------------
// Number parser
// --------------------------------------------------------------------------

// parseTOMLNumber parses a TOML number, returning JSON-ready bytes.
// Works entirely in []byte to avoid string conversion allocations.
func parseTOMLNumber(s []byte) ([]byte, error) {
	if len(s) == 0 {
		return nil, fmt.Errorf("invalid number")
	}

	neg := s[0] == '-'
	hasSign := neg || s[0] == '+'
	body := s
	if hasSign {
		body = s[1:]
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("invalid number: %s", s)
	}

	// Radix-prefixed integers: no sign allowed.
	if !hasSign {
		if (body[0] == '0') && len(body) > 1 {
			switch body[1] {
			case 'x', 'X':
				digits := stripUnderscoresBytes(body[2:])
				if len(digits) == 0 {
					return nil, fmt.Errorf("invalid hex number: %s", s)
				}
				v, err := strconv.ParseInt(string(digits), 16, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid hex number %s: %v", s, err)
				}
				return strconv.AppendInt(nil, v, 10), nil
			case 'o', 'O':
				digits := stripUnderscoresBytes(body[2:])
				v, err := strconv.ParseInt(string(digits), 8, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid octal number %s: %v", s, err)
				}
				return strconv.AppendInt(nil, v, 10), nil
			case 'b', 'B':
				digits := stripUnderscoresBytes(body[2:])
				v, err := strconv.ParseInt(string(digits), 2, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid binary number %s: %v", s, err)
				}
				return strconv.AppendInt(nil, v, 10), nil
			}
		}
	}

	stripped, err := stripUnderscoresBytesValidated(body)
	if err != nil {
		return nil, fmt.Errorf("invalid number %s: %v", s, err)
	}
	wasStripped := len(stripped) != len(body)
	body = stripped

	if len(body) > 1 && body[0] == '0' && body[1] >= '0' && body[1] <= '9' {
		return nil, fmt.Errorf("leading zeros not allowed in integer: %s", s)
	}

	isFloat := bytes.ContainsAny(body, ".eE")

	// Build result without unnecessary allocation:
	// - no sign or '+': return body (sub-slice of s, or stripped copy)
	// - '-' with no stripping: return s (already starts with '-')
	// - '-' with stripping: need to prepend '-' to new allocation
	var result []byte
	if neg {
		if wasStripped {
			result = make([]byte, 1+len(body))
			result[0] = '-'
			copy(result[1:], body)
		} else {
			result = s // original includes the '-'
		}
	} else {
		result = body // '+' stripped or no sign; body points into s or is new alloc
	}

	if isFloat {
		if _, err := strconv.ParseFloat(string(result), 64); err != nil {
			return nil, fmt.Errorf("invalid float %s: %v", s, err)
		}
		return result, nil
	}

	if _, err := strconv.ParseInt(string(result), 10, 64); err != nil {
		if _, err2 := strconv.ParseUint(string(result), 10, 64); err2 != nil {
			return nil, fmt.Errorf("invalid integer %s: %v", s, err)
		}
	}
	return result, nil
}

// stripUnderscoresBytes removes underscores without validation (for radix-prefixed numbers).
// Returns s unchanged (same backing array) if no underscores present.
func stripUnderscoresBytes(s []byte) []byte {
	if bytes.IndexByte(s, '_') < 0 {
		return s
	}
	out := make([]byte, 0, len(s))
	for _, c := range s {
		if c != '_' {
			out = append(out, c)
		}
	}
	return out
}

// stripUnderscoresBytesValidated removes underscores from a decimal/float number,
// validating that they are not at the start, end, or adjacent.
// Returns s unchanged (same backing array) if no underscores present.
func stripUnderscoresBytesValidated(s []byte) ([]byte, error) {
	if bytes.IndexByte(s, '_') < 0 {
		return s, nil
	}
	out := make([]byte, 0, len(s))
	for i := range s {
		c := s[i]
		if c == '_' {
			if i == 0 || i == len(s)-1 {
				return nil, fmt.Errorf("underscore at start or end of number")
			}
			if s[i-1] == '_' {
				return nil, fmt.Errorf("adjacent underscores in number")
			}
			if s[i-1] == '.' || (i+1 < len(s) && s[i+1] == '.') {
				return nil, fmt.Errorf("underscore adjacent to decimal point")
			}
			if s[i-1] == 'e' || s[i-1] == 'E' || (i+1 < len(s) && (s[i+1] == 'e' || s[i+1] == 'E')) {
				return nil, fmt.Errorf("underscore adjacent to exponent")
			}
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// --------------------------------------------------------------------------
// Datetime detection
// --------------------------------------------------------------------------

func isTOMLDateTime(s []byte) bool {
	if len(s) >= 5 && s[2] == ':' && isDigits(s[0:2]) {
		return true
	}
	if len(s) >= 10 && s[4] == '-' && isDigits(s[0:4]) {
		return true
	}
	return false
}

func isDigits(s []byte) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// --------------------------------------------------------------------------
// Inline table parser
// --------------------------------------------------------------------------

// parseTOMLInlineTable parses {k = v, ...} starting at s[pos].
// Returns the built jnode, position after the closing '}', and any error.
func parseTOMLInlineTable(s []byte, pos int) (*jnode, int, error) {
	if pos >= len(s) || s[pos] != '{' {
		return nil, pos, fmt.Errorf("expected '{'")
	}
	pos++ // consume '{'
	node := newObjectNode()
	pos = flowSkipWS(s, pos)

	if pos < len(s) && s[pos] == '}' {
		return node, pos + 1, nil
	}

	var pathBuf [4][]byte
	first := true
	for pos < len(s) {
		if !first {
			if pos >= len(s) || s[pos] != ',' {
				return nil, pos, fmt.Errorf("expected ',' or '}' in inline table")
			}
			pos = flowSkipWS(s, pos+1)
			if pos < len(s) && s[pos] == '}' {
				return nil, pos, fmt.Errorf("trailing comma not allowed in inline table")
			}
		}
		first = false

		path, rest, err := parseTOMLKeyPath(s[pos:], pathBuf[:0])
		if err != nil {
			return nil, pos, err
		}
		pos += len(s[pos:]) - len(rest)
		pos = flowSkipWS(s, pos)
		if pos >= len(s) || s[pos] != '=' {
			return nil, pos, fmt.Errorf("expected '=' in inline table")
		}
		pos = flowSkipWS(s, pos+1)

		valNode, newPos, err := parseTOMLInlineValue(s, pos)
		if err != nil {
			return nil, pos, err
		}
		pos = flowSkipWS(s, newPos)

		target := node
		for i := 0; i < len(path)-1; i++ {
			pair := target.findPair(path[i])
			if pair == nil {
				next := newObjectNode()
				target.obj = append(target.obj, &jpair{key: path[i], val: next})
				target = next
			} else if pair.val.obj != nil {
				target = pair.val
			} else {
				return nil, pos, fmt.Errorf("duplicate key %q in inline table", path[i])
			}
		}
		lastKey := path[len(path)-1]
		if target.findPair(lastKey) != nil {
			return nil, pos, fmt.Errorf("duplicate key %q in inline table", lastKey)
		}
		target.obj = append(target.obj, &jpair{key: lastKey, val: valNode})

		if pos < len(s) && s[pos] == '}' {
			return node, pos + 1, nil
		}
	}
	return nil, pos, fmt.Errorf("unterminated inline table")
}

// parseTOMLInlineValue parses a single value inside an inline collection (no multiline).
func parseTOMLInlineValue(s []byte, pos int) (*jnode, int, error) {
	pos = flowSkipWS(s, pos)
	if pos >= len(s) {
		return nil, pos, fmt.Errorf("expected value")
	}
	rest := s[pos:]
	end := tomlValueEnd(rest)
	node, _, err := parseTOMLValue(rest[:end], nil, 0)
	if err != nil {
		return nil, pos, err
	}
	return node, pos + end, nil
}

// tomlValueEnd returns the number of bytes in s consumed by the first TOML value.
func tomlValueEnd(s []byte) int {
	s = bytes.TrimLeft(s, " \t")
	if len(s) == 0 {
		return 0
	}
	switch {
	case bytes.HasPrefix(s, []byte(`"""`)):
		i := 3
		for i < len(s) {
			if bytes.HasPrefix(s[i:], []byte(`"""`)) {
				return i + 3
			}
			if s[i] == '\\' {
				i += 2
			} else {
				i++
			}
		}
		return len(s)
	case s[0] == '"':
		i := 1
		for i < len(s) {
			if s[i] == '\\' {
				i += 2
			} else if s[i] == '"' {
				return i + 1
			} else {
				i++
			}
		}
		return len(s)
	case bytes.HasPrefix(s, []byte("'''")):
		i := 3
		for i < len(s) {
			if bytes.HasPrefix(s[i:], []byte("'''")) {
				return i + 3
			}
			i++
		}
		return len(s)
	case s[0] == '\'':
		i := 1
		for i < len(s) {
			if s[i] == '\'' {
				return i + 1
			}
			i++
		}
		return len(s)
	case s[0] == '{':
		depth := 0
		inDouble, inSingle := false, false
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
				if c == '\'' {
					inSingle = false
				}
			case c == '"':
				inDouble = true
			case c == '\'':
				inSingle = true
			case c == '{':
				depth++
			case c == '}':
				depth--
				if depth == 0 {
					return i + 1
				}
			}
		}
		return len(s)
	case s[0] == '[':
		depth := 0
		inDouble, inSingle := false, false
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
				if c == '\'' {
					inSingle = false
				}
			case c == '"':
				inDouble = true
			case c == '\'':
				inSingle = true
			case c == '[':
				depth++
			case c == ']':
				depth--
				if depth == 0 {
					return i + 1
				}
			}
		}
		return len(s)
	default:
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c == ',' || c == '}' || c == ']' || c == ' ' || c == '\t' || c == '\r' || c == '\n' {
				return i
			}
		}
		return len(s)
	}
}

// --------------------------------------------------------------------------
// Direct-write value helpers (streaming path — no jnode allocation)
// --------------------------------------------------------------------------

// writeTOMLValue writes the JSON representation of a TOML value directly to buf.
// Returns extra lines consumed (for multiline strings) and any error.
func writeTOMLValue(s []byte, rawLines [][]byte, lineIdx int, buf *bytes.Buffer) (int, error) {
	s = bytes.TrimSpace(s)
	if len(s) == 0 {
		return 0, fmt.Errorf("expected value")
	}

	if bytes.HasPrefix(s, []byte(`"""`)) {
		str, consumed, err := parseTOMLMultilineBasic(s, rawLines, lineIdx)
		if err != nil {
			return 0, err
		}
		writeJSONString(str, buf)
		return consumed, nil
	}
	if s[0] == '"' {
		str, _, err := parseTOMLBasicStringRaw(s)
		if err != nil {
			return 0, err
		}
		writeJSONString(str, buf)
		return 0, nil
	}
	if bytes.HasPrefix(s, []byte("'''")) {
		str, consumed, err := parseTOMLMultilineLiteral(s, rawLines, lineIdx)
		if err != nil {
			return 0, err
		}
		writeJSONString(str, buf)
		return consumed, nil
	}
	if s[0] == '\'' {
		str, _ := parseTOMLLiteralStringRaw(s)
		writeJSONString(str, buf)
		return 0, nil
	}
	if s[0] == '{' {
		node, _, err := parseTOMLInlineTable(s, 0)
		if err != nil {
			return 0, err
		}
		serializeNode(node, buf)
		return 0, nil
	}
	if s[0] == '[' {
		return writeTOMLInlineArray(s, rawLines, lineIdx, buf)
	}
	if bytes.Equal(s, []byte("true")) {
		buf.WriteString("true")
		return 0, nil
	}
	if bytes.Equal(s, []byte("false")) {
		buf.WriteString("false")
		return 0, nil
	}
	if bytes.EqualFold(s, []byte("inf")) || bytes.EqualFold(s, []byte("+inf")) || bytes.EqualFold(s, []byte("-inf")) {
		return 0, fmt.Errorf("inf is not representable in JSON")
	}
	if bytes.EqualFold(s, []byte("nan")) || bytes.EqualFold(s, []byte("+nan")) || bytes.EqualFold(s, []byte("-nan")) {
		return 0, fmt.Errorf("nan is not representable in JSON")
	}
	if isTOMLDateTime(s) {
		writeJSONString(s, buf)
		return 0, nil
	}
	// fast path: valid JSON number as-is (no + prefix, no leading zeros, no underscores/radix)
	if isYAMLNumber(s) && s[0] != '+' {
		digits := s
		if digits[0] == '-' {
			digits = digits[1:]
		}
		if len(digits) < 2 || digits[0] != '0' || digits[1] < '0' || digits[1] > '9' {
			buf.Write(s)
			return 0, nil
		}
	}
	// strip leading + and retry (parseTOMLNumber handles validation)
	if len(s) > 1 && s[0] == '+' && isYAMLNumber(s[1:]) {
		s2 := s[1:]
		digits := s2
		if len(digits) < 2 || digits[0] != '0' || digits[1] < '0' || digits[1] > '9' {
			buf.Write(s2)
			return 0, nil
		}
	}
	raw, err := parseTOMLNumber(s)
	if err != nil {
		return 0, err
	}
	buf.Write(raw)
	return 0, nil
}

// tomlFlowSkipWS advances pos past ASCII whitespace, newlines, and TOML
// '#' line comments (which run to the end of the current line). Used inside
// multi-line inline arrays, where comments are permitted between values,
// element separators, and the closing bracket.
func tomlFlowSkipWS(s []byte, pos int) int {
	for pos < len(s) {
		switch s[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
		case '#':
			pos++
			for pos < len(s) && s[pos] != '\n' {
				pos++
			}
		default:
			return pos
		}
	}
	return pos
}

// writeTOMLInlineArray writes [v, v, ...] starting at s[0] directly to buf.
func writeTOMLInlineArray(s []byte, rawLines [][]byte, lineIdx int, buf *bytes.Buffer) (int, error) {
	// 3-index slice prevents appending into caller's buffer.
	s = s[:len(s):len(s)]
	pos := 1 // consume '['
	extraLines := 0
	buf.WriteByte('[')
	count := 0

	for {
		for {
			pos = tomlFlowSkipWS(s, pos)
			if pos < len(s) {
				break
			}
			nextIdx := lineIdx + extraLines + 1
			if rawLines == nil || nextIdx >= len(rawLines) {
				return extraLines, fmt.Errorf("unterminated inline array")
			}
			s = append(s, '\n')
			s = append(s, rawLines[nextIdx]...)
			extraLines++
		}

		if s[pos] == ']' {
			buf.WriteByte(']')
			return extraLines, nil
		}

		if count > 0 {
			if s[pos] != ',' {
				return extraLines, fmt.Errorf("expected ',' or ']' in array")
			}
			pos = tomlFlowSkipWS(s, pos+1)
			for {
				pos = tomlFlowSkipWS(s, pos)
				if pos < len(s) {
					break
				}
				nextIdx := lineIdx + extraLines + 1
				if rawLines == nil || nextIdx >= len(rawLines) {
					return extraLines, fmt.Errorf("unterminated inline array")
				}
				s = append(s, '\n')
				s = append(s, rawLines[nextIdx]...)
				extraLines++
			}
			if s[pos] == ']' {
				buf.WriteByte(']')
				return extraLines, nil
			}
			buf.WriteByte(',')
		}

		rest := bytes.TrimLeft(s[pos:], " \t")
		lead := len(s[pos:]) - len(rest)
		valEnd := tomlValueEnd(rest)
		consumed, err := writeTOMLValue(rest[:valEnd], rawLines, lineIdx+extraLines, buf)
		if err != nil {
			return extraLines, err
		}
		extraLines += consumed
		pos = pos + lead + valEnd
		count++
	}
}

// --------------------------------------------------------------------------
// Inline array parser
// --------------------------------------------------------------------------

// parseTOMLInlineArray parses [v, v, ...] starting at s[pos].
func parseTOMLInlineArray(s []byte, rawLines [][]byte, lineIdx int, pos int) (*jnode, int, int, error) {
	if pos >= len(s) || s[pos] != '[' {
		return nil, pos, 0, fmt.Errorf("expected '['")
	}
	// 3-index slice prevents appending into caller's buffer.
	s = s[:len(s):len(s)]
	pos++ // consume '['
	extraLines := 0
	node := &jnode{arr: []*jnode{}}

	for {
		for {
			pos = tomlFlowSkipWS(s, pos)
			if pos < len(s) {
				break
			}
			nextIdx := lineIdx + extraLines + 1
			if rawLines == nil || nextIdx >= len(rawLines) {
				return nil, pos, extraLines, fmt.Errorf("unterminated inline array")
			}
			s = append(s, '\n')
			s = append(s, rawLines[nextIdx]...)
			extraLines++
		}

		if s[pos] == ']' {
			return node, pos + 1, extraLines, nil
		}

		if len(node.arr) > 0 {
			if s[pos] != ',' {
				return nil, pos, extraLines, fmt.Errorf("expected ',' or ']' in array")
			}
			pos = tomlFlowSkipWS(s, pos+1)
			for {
				pos = tomlFlowSkipWS(s, pos)
				if pos < len(s) {
					break
				}
				nextIdx := lineIdx + extraLines + 1
				if rawLines == nil || nextIdx >= len(rawLines) {
					return nil, pos, extraLines, fmt.Errorf("unterminated inline array")
				}
				s = append(s, '\n')
				s = append(s, rawLines[nextIdx]...)
				extraLines++
			}
			if s[pos] == ']' {
				return node, pos + 1, extraLines, nil
			}
		}

		rest := bytes.TrimLeft(s[pos:], " \t")
		lead := len(s[pos:]) - len(rest)
		valEnd := tomlValueEnd(rest)
		valNode, consumed, err := parseTOMLValue(rest[:valEnd], rawLines, lineIdx+extraLines)
		if err != nil {
			return nil, pos, extraLines, err
		}
		extraLines += consumed
		pos = pos + lead + valEnd
		node.arr = append(node.arr, valNode)
	}
}
