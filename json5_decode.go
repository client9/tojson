package tojson

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
)

var bareInfinity = []byte("Infinity")
var bareNaN = []byte("NaN")

func isNaN(b []byte) bool {
	if len(b) == 3 {
		return bytes.Equal(b, bareNaN)
	}
	if len(b) == 4 && (b[0] == '+' || b[0] == '-') {
		return bytes.Equal(b[1:], bareNaN)
	}
	return false
}

func isInfinity(b []byte) bool {
	if bytes.Equal(b, bareInfinity) {
		return true
	}
	if len(b) > 1 && (b[0] == '+' || b[0] == '-') {
		return bytes.Equal(b[1:], bareInfinity)
	}
	return false
}

type decoder struct {
	tok      tokenizer
	buf      bytes.Buffer // embedded output; FromJSONVariant sets out = &buf
	out      *bytes.Buffer
	stack    []byte
	stackbuf [8]byte // inline backing for stack; avoids a heap alloc at typical nesting depths
	next     stateFunction
	lastRow  int
	lastCol  int
}

type stateFunction func(d *decoder, t token) error

func (d *decoder) Translate(src []byte) error {
	d.tok = tokenizer{data: src}
	d.next = stateValue

	for {
		t, err := d.tok.Next()
		if err == io.EOF {
			if len(d.stack) == 0 {
				return nil
			}
			return &ParseError{Line: d.lastRow + 1, Column: d.lastCol + 1, Message: "got end of file prematurely"}
		}
		if err != nil {
			return err
		}

		// ignore comments
		if t.kind == 'c' {
			continue
		}
		d.lastRow = t.row
		d.lastCol = t.col
		err = d.next(d, t)

		if err != nil {
			return err
		}
	}
}

func stateValue(d *decoder, t token) error {
	switch t.kind {
	case leftBrace:
		return stateObjectStart(d, t)
	case leftBracket:
		return stateArrayStart(d, t)
	case 's':
		writeString(d.out, t.value)
		d.next = stateObjectAfterValue
	case '0':
		if err := writeInt(d.out, t.value); err != nil {
			return atToken(t, err)
		}
		d.next = stateObjectAfterValue
	case '1':
		if err := writeFloat(d.out, t.value); err != nil {
			return atToken(t, err)
		}
		d.next = stateObjectAfterValue
	case '2':
		if err := writeHex(d.out, t.value); err != nil {
			return atToken(t, err)
		}
		d.next = stateObjectAfterValue
	case 'w':
		if isNaN(t.value) || isInfinity(t.value) {
			return atToken(t, fmt.Errorf("%s is not representable in JSON", t.value))
		}
		bareword(d.out, t.value)
		d.next = stateObjectAfterValue
	default:
		return atToken(t, fmt.Errorf("unknown token for value"))
	}
	return nil
}

func stateObjectStart(d *decoder, t token) error {
	d.out.WriteByte('{')
	d.stack = append(d.stack, '{')
	d.next = stateObjectAfterStart
	return nil
}

func stateObjectAfterStart(d *decoder, t token) error {
	switch t.kind {
	case '}':
		return stateObjectEnd(d, t)
	case ',': // degenerate case
		// ignore comma and reparse
		d.next = stateObjectAfterStart
		return nil
	}
	return stateObjectKey(d, t)
}

func stateObjectKey(d *decoder, t token) error {
	switch t.kind {
	case 's':
		writeString(d.out, t.value)
		d.next = stateObjectAfterKey
	case 'w', '0', '1', '2':
		// whatever it is, it's always quoted
		writeQuoted(d.out, t.value)
		d.next = stateObjectAfterKey
	default:
		return atToken(t, fmt.Errorf("invalid token at object key: %s", t))
	}
	return nil
}

func stateObjectAfterKey(d *decoder, t token) error {
	if t.kind == ':' {
		d.out.Write(t.value)
		d.next = stateObjectValue
		return nil
	}

	return atToken(t, fmt.Errorf("invalid token after object key"))
}

func stateObjectValue(d *decoder, t token) error {
	switch t.kind {
	case 's':
		writeString(d.out, t.value)
		d.next = stateObjectAfterValue
	case 'w':
		if isNaN(t.value) || isInfinity(t.value) {
			return atToken(t, fmt.Errorf("%s is not representable in JSON", t.value))
		}
		bareword(d.out, t.value)
		d.next = stateObjectAfterValue
	case '0':
		if err := writeInt(d.out, t.value); err != nil {
			return atToken(t, err)
		}
		d.next = stateObjectAfterValue
	case '1':
		if err := writeFloat(d.out, t.value); err != nil {
			return atToken(t, err)
		}
		d.next = stateObjectAfterValue
	case '2':
		if err := writeHex(d.out, t.value); err != nil {
			return atToken(t, err)
		}
		d.next = stateObjectAfterValue
	case '{':
		return stateObjectStart(d, t)
	case '[':
		return stateArrayStart(d, t)
	default:
		return atToken(t, fmt.Errorf("unknown token for object value: %s", t))
	}
	return nil
}
func stateObjectAfterValue(d *decoder, t token) error {
	switch t.kind {
	case '}':
		return stateObjectEnd(d, t)
	case ',':
		return stateComma(d, t)
		// MIDDLE COMMA
	case 'w', 's', '0', '1', '2':
		// e.g. { "key": 1 "key2": 2 }  ==> { "key": 1, "key2": 2 }
		d.out.WriteByte(',')
		return stateObjectKey(d, t)
	default:
		return atToken(t, fmt.Errorf("unknown token after object value: %s", t))
	}
}

func stateComma(d *decoder, t token) error {
	// check if next token is "}"

	t2, err := d.tok.Next()
	if err != nil {
		return err
	}

	if t2.kind == 'c' {
		// if comment, reparse
		return stateComma(d, t)
	}
	if t2.kind == '}' {
		// Skip writing comma
		return stateObjectEnd(d, t2)
	}
	if t2.kind == ']' {
		return stateArrayEnd(d, t2)
	}

	d.out.Write(t.value)

	if d.stack[len(d.stack)-1] == '{' {
		// write comma, and expect a key
		return stateObjectKey(d, t2)
	}

	// it's an array value
	return stateArrayValue(d, t2)
}

func stateObjectEnd(d *decoder, t token) error {
	if len(d.stack) == 0 || d.stack[len(d.stack)-1] != '{' {
		return atToken(t, fmt.Errorf("unmatched object end, level=%d, stack=%q", len(d.stack), string(d.stack)))
	}
	d.out.WriteByte('}')
	d.stack = d.stack[:len(d.stack)-1]
	d.next = stateAfterContainer
	return nil
}

func stateAfterContainer(d *decoder, t token) error {

	switch t.kind {
	case rightBrace:
		return stateObjectEnd(d, t)
	case rightBracket:
		return stateArrayEnd(d, t)
	case ',':
		return stateComma(d, t)

	// MIDDLE COMMA
	case leftBrace, leftBracket, 'w', 's', '0', '1', '2':
		if len(d.stack) == 0 {
			return atToken(t, fmt.Errorf("unexpected token after top-level value: %s", t))
		}
		d.out.WriteByte(',')
		if d.stack[len(d.stack)-1] == leftBrace {
			// write comma, and expect a key
			return stateObjectKey(d, t)
		} else {
			// it's an array value
			return stateArrayValue(d, t)
		}

	default:
		return atToken(t, fmt.Errorf("unknown token after end of object or array: %s", t))
	}
}

func stateArrayStart(d *decoder, t token) error {
	d.out.WriteByte('[')
	d.stack = append(d.stack, '[')
	d.next = stateArrayAfterStart
	return nil
}

func stateArrayAfterStart(d *decoder, t token) error {
	switch t.kind {
	case ']':
		return stateArrayEnd(d, t)
	case ',': // degenerate case
		// ignore comma and reparse
		d.next = stateArrayAfterStart
		return nil
	}
	return stateArrayValue(d, t)
}

func stateArrayEnd(d *decoder, t token) error {
	if len(d.stack) == 0 || d.stack[len(d.stack)-1] != '[' {
		return atToken(t, fmt.Errorf("unmatched array end"))
	}
	d.out.WriteByte(']')
	d.stack = d.stack[:len(d.stack)-1]
	d.next = stateAfterContainer
	return nil
}

func stateArrayValue(d *decoder, t token) error {
	switch t.kind {
	case 's':
		writeString(d.out, t.value)
		d.next = stateArrayAfterValue
	case 'w':
		if isNaN(t.value) || isInfinity(t.value) {
			return atToken(t, fmt.Errorf("%s is not representable in JSON", t.value))
		}
		bareword(d.out, t.value)
		d.next = stateArrayAfterValue
	case '0':
		if err := writeInt(d.out, t.value); err != nil {
			return atToken(t, err)
		}
		d.next = stateArrayAfterValue
	case '1':
		if err := writeFloat(d.out, t.value); err != nil {
			return atToken(t, err)
		}
		d.next = stateArrayAfterValue
	case '2':
		if err := writeHex(d.out, t.value); err != nil {
			return atToken(t, err)
		}
		d.next = stateArrayAfterValue
	case '{':
		return stateObjectStart(d, t)
	case '[':
		return stateArrayStart(d, t)
	default:
		return atToken(t, fmt.Errorf("unknown token for array value: %s", t))
	}
	return nil
}

func stateArrayAfterValue(d *decoder, t token) error {
	switch t.kind {
	case ']':
		return stateArrayEnd(d, t)
	case ',':
		return stateComma(d, t)

		// MIDDLE COMMA
	case leftBrace, leftBracket, 'w', 's', '0', '1', '2':
		// e.g. [ 1 2 3 ] ==> [ 1,2,3 ]
		//      [ "foo" "bar" ] ==> [ "foo", "bar" ]
		d.out.WriteByte(',')
		return stateArrayValue(d, t)
	}
	return atToken(t, fmt.Errorf("unknown token after array value: %s", t))
}

func isNull(b []byte) bool {
	if len(b) != 4 {
		return false
	}
	return b[0] == 'n' &&
		b[1] == 'u' &&
		b[2] == 'l' &&
		b[3] == 'l'
}
func isTrue(b []byte) bool {
	if len(b) != 4 {
		return false
	}
	return b[0] == 't' &&
		b[1] == 'r' &&
		b[2] == 'u' &&
		b[3] == 'e'
}
func isFalse(b []byte) bool {
	if len(b) != 5 {
		return false
	}
	return b[0] == 'f' &&
		b[1] == 'a' &&
		b[2] == 'l' &&
		b[3] == 's' &&
		b[4] == 'e'
}

func writeInt(out *bytes.Buffer, b []byte) error {
	if len(b) == 0 {
		return nil
	}
	writeNormalizedNumber(out, b)
	return nil
}

// Unoptimized since it's a rare feature
func writeHex(out *bytes.Buffer, b []byte) error {
	// slice off "0x" or "0X"
	s := string(b[2:])
	num, err := strconv.ParseUint(s, 16, 64)
	if err == nil {
		out.WriteString(strconv.FormatUint(num, 10))
		return nil
	}
	return fmt.Errorf("hex literal %s overflows uint64", b)
}

func writeFloat(out *bytes.Buffer, b []byte) error {
	if len(b) == 0 {
		return nil
	}
	writeNormalizedNumber(out, b)
	return nil
}

func bareword(out *bytes.Buffer, b []byte) {
	if isNull(b) || isTrue(b) || isFalse(b) {
		out.Write(b)
		return
	}
	out.Write(b)
}

// writeString takes an "quoted string with escapes" and converts to a JSON-spec string.
// it needs to handle
//
//   - single quote strings
//   - double quote strings
//   - backtick quote strings
//
// all with a variety of escape sequences.
func writeString(out *bytes.Buffer, src []byte) {
	// get quote type
	qchar := src[0]

	// strip off quotes
	src = src[1 : len(src)-1]

	// do we need to decode anything?
	hasEscape := false
	for i := 0; i < len(src); {
		b := src[i]
		if b < utf8.RuneSelf {
			if !safeSet[b] {
				hasEscape = true
				break
			}
			i++
			continue
		}
		n := min(len(src)-i, utf8.UTFMax)
		c, size := utf8.DecodeRune(src[i : i+n])
		if c == ' ' || c == ' ' {
			hasEscape = true
			break
		}
		i += size
	}

	if !hasEscape {
		out.WriteByte('"')
		out.Write(src)
		out.WriteByte('"')
		return
	}

	if qchar == backQuote {
		// TBD: ERROR -- \` needs to be unescaped.
		//
		// no need to unescape first -- directly encode
		writeQuoted(out, src)
		return
	}

	buf := out.AvailableBuffer()
	buf = appendRecodeString(buf, src)
	out.Write(buf)
}

func writeQuoted(out *bytes.Buffer, src []byte) {
	for i := 0; i < len(src); {
		b := src[i]
		if b < utf8.RuneSelf {
			if !safeSet[b] {
				buf := out.AvailableBuffer()
				buf = appendString(buf, src)
				out.Write(buf)
				return
			}
			i++
			continue
		}
		n := min(len(src)-i, utf8.UTFMax)
		c, size := utf8.DecodeRune(src[i : i+n])
		if c == ' ' || c == ' ' {
			buf := out.AvailableBuffer()
			buf = appendString(buf, src)
			out.Write(buf)
			return
		}
		i += size
	}
	out.WriteByte('"')
	out.Write(src)
	out.WriteByte('"')
}
