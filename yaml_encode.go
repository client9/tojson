package tojson

import (
	"bytes"
	"fmt"
	"io"
	"unicode"
	"unicode/utf8"
)

// yamlMaxDepth bounds container nesting so that malformed or hostile input
// cannot drive the recursive emitter into a stack overflow.
const yamlMaxDepth = 200

// yamlDefaultIndent is the number of spaces per nesting level that YAMLStyle's
// zero value asks for.
const yamlDefaultIndent = 2

// yamlDashWidth is the width of the "- " that opens a sequence item, and so
// the column its value starts at. It is a property of YAML, not of the style.
const yamlDashWidth = 2

var yamlSpaces = []byte("                                ")

// encoder walks the JSON token stream and writes block-style YAML.
// It shares the tokenizer with the JSON5 path, so it accepts the same
// JSON variants that FromJSONVariant does.
type encoder struct {
	style    YAMLStyle
	tok      tokenizer
	buf      bytes.Buffer
	out      *bytes.Buffer
	scratch  bytes.Buffer
	line     []byte // decoded block-scalar content, reused across values
	peeked   token
	havePeek bool
	depth    int
}

func (e *encoder) next() (token, error) {
	if e.havePeek {
		e.havePeek = false
		return e.peeked, nil
	}
	for {
		t, err := e.tok.Next()
		if err != nil {
			return t, err
		}
		if t.kind == 'c' {
			continue
		}
		return t, nil
	}
}

func (e *encoder) peek() (token, error) {
	if e.havePeek {
		return e.peeked, nil
	}
	t, err := e.next()
	if err != nil {
		return t, err
	}
	e.peeked = t
	e.havePeek = true
	return t, nil
}

func (e *encoder) writeIndent(n int) {
	for n > len(yamlSpaces) {
		e.out.Write(yamlSpaces)
		n -= len(yamlSpaces)
	}
	e.out.Write(yamlSpaces[:n])
}

// eof reports the parse error used when the input ends inside a container.
func (e *encoder) eof() error {
	return &ParseError{Line: e.tok.row + 1, Column: e.tok.col + 1, Message: "got end of file prematurely"}
}

func yamlEncode(src []byte, style YAMLStyle) ([]byte, error) {
	if style.Indent < 0 {
		return nil, errNegativeIndent
	}
	if style.Indent == 0 {
		style.Indent = yamlDefaultIndent
	}
	switch style.Multiline {
	case BlockLiteral, Quoted:
	default:
		return nil, errUnknownMultiline
	}

	e := &encoder{style: style}
	e.tok = tokenizer{data: src}
	e.out = &e.buf
	e.buf.Grow(len(src) + len(src)/4)

	t, err := e.next()
	if err == io.EOF {
		// mirrors FromJSONVariant: empty input produces empty output
		return e.buf.Bytes(), nil
	}
	if err != nil {
		return nil, err
	}
	if err := e.emitValue(t, 0, 0); err != nil {
		return nil, err
	}
	if t2, err := e.next(); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, atToken(t2, fmt.Errorf("unexpected token after top-level value"))
	}
	e.buf.WriteByte('\n')
	return e.buf.Bytes(), nil
}

// emitValue writes t at the current cursor position. The caller has already
// written whatever prefix belongs on this line ("- ", "key: ", or nothing).
// indent is the column that continuation lines of this value start at, and
// parent the indentation of the node it hangs off.
func (e *encoder) emitValue(t token, indent, parent int) error {
	switch t.kind {
	case leftBrace:
		return e.emitMapping(indent)
	case leftBracket:
		return e.emitSequence(indent)
	}
	return e.emitScalar(t, indent, parent)
}

func (e *encoder) emitMapping(indent int) error {
	if e.depth++; e.depth > yamlMaxDepth {
		return &ParseError{Line: e.tok.row + 1, Column: e.tok.col + 1, Message: "nesting too deep"}
	}
	defer func() { e.depth-- }()

	first := true
	for {
		t, err := e.next()
		if err == io.EOF {
			return e.eof()
		}
		if err != nil {
			return err
		}
		if t.kind == rightBrace {
			if first {
				e.out.WriteString("{}")
			}
			return nil
		}
		if t.kind == comma {
			continue
		}
		if !first {
			e.out.WriteByte('\n')
			e.writeIndent(indent)
		}
		first = false

		if err := e.emitKey(t); err != nil {
			return err
		}
		sep, err := e.next()
		if err == io.EOF {
			return e.eof()
		}
		if err != nil {
			return err
		}
		if sep.kind != colon {
			return atToken(sep, fmt.Errorf("invalid token after object key"))
		}
		v, err := e.next()
		if err == io.EOF {
			return e.eof()
		}
		if err != nil {
			return err
		}
		e.out.WriteByte(':')
		if err := e.emitNested(v, indent); err != nil {
			return err
		}
	}
}

func (e *encoder) emitSequence(indent int) error {
	if e.depth++; e.depth > yamlMaxDepth {
		return &ParseError{Line: e.tok.row + 1, Column: e.tok.col + 1, Message: "nesting too deep"}
	}
	defer func() { e.depth-- }()

	first := true
	for {
		t, err := e.next()
		if err == io.EOF {
			return e.eof()
		}
		if err != nil {
			return err
		}
		if t.kind == rightBracket {
			if first {
				e.out.WriteString("[]")
			}
			return nil
		}
		if t.kind == comma {
			continue
		}
		if !first {
			e.out.WriteByte('\n')
			e.writeIndent(indent)
		}
		first = false

		empty, err := e.emptyContainer(t)
		if err != nil {
			return err
		}
		if empty {
			e.out.WriteString("- ")
			if err := e.emitEmpty(t); err != nil {
				return err
			}
			continue
		}
		// An item written after "- " begins two columns in, whatever the
		// configured indent, because that is the width of the marker. Its
		// continuation lines have to line up with it.
		child := indent + yamlDashWidth
		if t.kind == leftBracket {
			// A nested sequence goes on its own indented lines, so it takes a
			// real indentation level. The compact "- - 1" form is equivalent,
			// but this one states the nested sequence's indentation outright
			// instead of implying it.
			child = indent + e.style.Indent
			e.out.WriteByte('-')
			e.out.WriteByte('\n')
			e.writeIndent(child)
		} else {
			e.out.WriteString("- ")
		}
		if err := e.emitValue(t, child, indent); err != nil {
			return err
		}
	}
}

// emitNested writes a mapping value after the ":" has been written. parent is
// the indentation of the key. Containers move to their own lines; scalars stay
// on the key's line.
func (e *encoder) emitNested(t token, parent int) error {
	child := parent + e.style.Indent
	if t.kind == leftBracket && e.style.CompactSequence {
		// a sequence may sit at the indentation of its own key
		child = parent
	}
	if t.kind == leftBrace || t.kind == leftBracket {
		empty, err := e.emptyContainer(t)
		if err != nil {
			return err
		}
		if empty {
			e.out.WriteByte(' ')
			return e.emitEmpty(t)
		}
		e.out.WriteByte('\n')
		e.writeIndent(child)
		return e.emitValue(t, child, parent)
	}
	e.out.WriteByte(' ')
	return e.emitScalar(t, child, parent)
}

// emptyContainer reports whether the container just opened by t closes immediately.
func (e *encoder) emptyContainer(t token) (bool, error) {
	if t.kind != leftBrace && t.kind != leftBracket {
		return false, nil
	}
	nt, err := e.peek()
	if err == io.EOF {
		return false, e.eof()
	}
	if err != nil {
		return false, err
	}
	if t.kind == leftBrace {
		return nt.kind == rightBrace, nil
	}
	return nt.kind == rightBracket, nil
}

// emitEmpty writes the flow form of an empty container and consumes its closer.
func (e *encoder) emitEmpty(t token) error {
	if t.kind == leftBrace {
		e.out.WriteString("{}")
	} else {
		e.out.WriteString("[]")
	}
	_, err := e.next()
	return err
}

func (e *encoder) emitKey(t token) error {
	switch t.kind {
	case 's':
		e.writeMaybePlain(e.jsonBody(t.value))
		return nil
	case 'w', '0', '1', '2':
		e.writeMaybePlain(t.value)
		return nil
	}
	return atToken(t, fmt.Errorf("invalid token at object key: %s", t))
}

// jsonBody returns the body of quoted string token src as it would appear
// inside a JSON string: quotes stripped, JSON5 escapes recoded to JSON ones.
// The common case, a double-quoted string needing no changes, aliases the
// input and copies nothing.
func (e *encoder) jsonBody(src []byte) []byte {
	if src[0] == doubleQuote && !needsRecode(src[1:len(src)-1]) {
		return src[1 : len(src)-1]
	}
	e.scratch.Reset()
	writeString(&e.scratch, src)
	b := e.scratch.Bytes()
	return b[1 : len(b)-1]
}

// needsRecode reports whether the raw body of a string literal differs from
// its JSON form. It mirrors the scan in writeString.
func needsRecode(src []byte) bool {
	for i := 0; i < len(src); {
		b := src[i]
		if b < utf8.RuneSelf {
			if !safeSet[b] {
				return true
			}
			i++
			continue
		}
		n := min(len(src)-i, utf8.UTFMax)
		c, size := utf8.DecodeRune(src[i : i+n])
		if c == '\u2028' || c == '\u2029' {
			return true
		}
		i += size
	}
	return false
}

// writeMaybePlain writes an unescaped string as a YAML plain scalar when that
// round-trips, and as a double-quoted scalar otherwise. A JSON double-quoted
// string is always a valid YAML double-quoted scalar.
func (e *encoder) writeMaybePlain(inner []byte) {
	if yamlPlainSafe(inner) {
		e.out.Write(inner)
		return
	}
	e.writeQuotedScalar(inner)
}

// writeQuotedScalar writes inner as a double-quoted scalar, escaping the line breaks
// YAML recognizes beyond \n. A raw NEL, LS or PS ends the line even inside
// quotes, where it would come back folded into a space.
func (e *encoder) writeQuotedScalar(inner []byte) {
	e.out.WriteByte('"')
	start := 0
	for i := 0; i < len(inner); {
		esc, size := yamlBreakEscape(inner[i:])
		if size == 0 {
			i++
			continue
		}
		e.out.Write(inner[start:i])
		e.out.WriteString(esc)
		i += size
		start = i
	}
	e.out.Write(inner[start:])
	e.out.WriteByte('"')
}

// yamlBreakEscape returns the escape for the line break at the start of b, and
// how many bytes that break occupies, or a zero size if b does not start with
// one. It covers NEL (U+0085), LS (U+2028) and PS (U+2029); \n and \r reach
// here already escaped.
func yamlBreakEscape(b []byte) (string, int) {
	if len(b) >= 2 && b[0] == 0xc2 && b[1] == 0x85 {
		return `\u0085`, 2
	}
	if len(b) >= 3 && b[0] == 0xe2 && b[1] == 0x80 {
		switch b[2] {
		case 0xa8:
			return `\u2028`, 3
		case 0xa9:
			return `\u2029`, 3
		}
	}
	return "", 0
}

func (e *encoder) emitScalar(t token, indent, parent int) error {
	switch t.kind {
	case 's':
		body := e.jsonBody(t.value)
		if e.style.Multiline == BlockLiteral && e.emitBlockScalar(body, indent, parent) {
			return nil
		}
		e.writeMaybePlain(body)
		return nil
	case '0', '1':
		writeNormalizedNumber(e.out, t.value)
		return nil
	case '2':
		if err := writeHex(e.out, t.value); err != nil {
			return atToken(t, err)
		}
		return nil
	case 'w':
		if err := e.emitBareword(t.value); err != nil {
			return atToken(t, err)
		}
		return nil
	}
	return atToken(t, fmt.Errorf("unknown token for value: %s", t))
}

// emitBareword writes an unquoted value. YAML can represent the infinities and
// NaN that JSON cannot, so those convert rather than fail; anything else
// unquoted is not a value FromJSONVariant accepts either.
func (e *encoder) emitBareword(b []byte) error {
	switch {
	case isNull(b), isTrue(b), isFalse(b):
		e.out.Write(b)
	case isInfinity(b):
		if b[0] == '-' {
			e.out.WriteString("-.inf")
		} else {
			e.out.WriteString(".inf")
		}
	case isNaN(b):
		e.out.WriteString(".nan")
	default:
		return fmt.Errorf("%s is not a JSON value", b)
	}
	return nil
}

// blockEscape decodes the JSON escapes whose character a literal block scalar
// can hold as itself, reporting false for the rest. A tab qualifies: YAML
// forbids tabs in indentation, not in content, and a line that starts or ends
// with one is turned away later.
func blockEscape(c byte) (byte, bool) {
	switch c {
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case '"':
		return '"', true
	case '\\':
		return '\\', true
	case '/':
		return '/', true
	}
	return 0, false
}

// emitBlockScalar writes inner as a literal block scalar ("|") when that is
// both possible and an improvement, reporting whether it did so. inner is the
// JSON-escaped body of the string. parent is the indentation of the node this
// scalar hangs off, which is what an explicit indentation indicator counts
// from; indent is the column the content is written at.
func (e *encoder) emitBlockScalar(inner []byte, indent, parent int) bool {
	// Decode the body. Block content is literal text, so it carries a quote,
	// a backslash or a tab as itself; only what it cannot represent, control
	// characters above all, sends the string to a quoted scalar.
	e.line = e.line[:0]
	hasNewline := false
	for i := 0; i < len(inner); i++ {
		if inner[i] != backslash {
			e.line = append(e.line, inner[i])
			continue
		}
		if i+1 >= len(inner) {
			return false
		}
		c, ok := blockEscape(inner[i+1])
		if !ok {
			return false
		}
		if c == '\n' {
			hasNewline = true
		}
		e.line = append(e.line, c)
		i++
	}
	if !hasNewline || hasNonASCIISpace(e.line) {
		return false
	}
	// Block content must be indented deeper than its parent node, so a
	// top-level string still needs one level of indent.
	if indent < e.style.Indent {
		indent = e.style.Indent
	}
	body := e.line

	// Chomping. One trailing newline is the default, "clip" (|). None is
	// "strip" (|-). More than one is "keep" (|+), which also holds on to the
	// blank lines at the end.
	trailing := 0
	for len(body) > 0 && body[len(body)-1] == '\n' {
		body = body[:len(body)-1]
		trailing++
	}
	if len(body) == 0 {
		return false // nothing but line breaks
	}
	chomp := ""
	switch trailing {
	case 0:
		chomp = "-"
	case 1:
	default:
		chomp = "+"
	}

	// A reader with no indentation indicator to go on takes the block's
	// indentation from its first non-empty line, so leading whitespace there
	// would be read as indentation and lost. Only that line calls for an
	// indicator: once it has fixed the indentation, whitespace opening a line
	// below it is content, since every line is written at the block
	// indentation plus whatever it carries of its own. Trailing whitespace has
	// no such remedy: it does not survive a round trip.
	indicator := ""
	firstNonEmpty := true
	for rest := body; ; {
		ln, more := nextLine(&rest)
		if len(ln) > 0 {
			if firstNonEmpty && (ln[0] == ' ' || ln[0] == '\t') {
				// The indicator counts from the parent's indentation and is a
				// single digit, so a deeply nested block gives up here.
				rel := indent - parent
				if rel < 1 || rel > 9 {
					return false
				}
				indicator = string([]byte{'0' + byte(rel)})
			}
			firstNonEmpty = false
			if c := ln[len(ln)-1]; c == ' ' || c == '\t' {
				return false
			}
		}
		if !more {
			break
		}
	}

	e.out.WriteByte('|')
	e.out.WriteString(indicator)
	e.out.WriteString(chomp)
	for rest := body; ; {
		ln, more := nextLine(&rest)
		e.out.WriteByte('\n')
		if len(ln) > 0 {
			e.writeIndent(indent)
			e.out.Write(ln)
		}
		if !more {
			break
		}
	}
	// Under keep, the last content line accounts for one trailing newline and
	// each blank line after it for one more.
	for i := 1; i < trailing; i++ {
		e.out.WriteByte('\n')
	}
	return true
}

// nextLine splits the leading newline-terminated line off of *rest, reporting
// whether any further lines remain.
func nextLine(rest *[]byte) (line []byte, more bool) {
	if i := bytes.IndexByte(*rest, '\n'); i >= 0 {
		line, *rest = (*rest)[:i], (*rest)[i+1:]
		return line, true
	}
	line, *rest = *rest, nil
	return line, false
}

// hasNonASCIISpace reports whether b holds a whitespace rune outside ASCII.
// Those cover the line breaks YAML 1.1 recognizes (NEL, LS, PS) and the spaces
// a reader may trim, none of which survive a block or plain scalar.
func hasNonASCIISpace(b []byte) bool {
	for i := 0; i < len(b); {
		if b[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, size := utf8.DecodeRune(b[i:])
		if unicode.IsSpace(r) {
			return true
		}
		i += size
	}
	return false
}

// yamlPlainSafe reports whether s can be written as a YAML plain scalar and
// read back as the identical string. s must already be free of escapes.
func yamlPlainSafe(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	if s[0] == ' ' || s[len(s)-1] == ' ' || s[len(s)-1] == ':' {
		return false
	}
	// document markers, which end or start a document when they stand alone
	if len(s) >= 3 && (s[0] == '-' || s[0] == '.') &&
		s[1] == s[0] && s[2] == s[0] && (len(s) == 3 || s[3] == ' ') {
		return false
	}
	switch s[0] {
	case '-', '?', ':':
		if len(s) == 1 || s[1] == ' ' {
			return false
		}
	case ',', '[', ']', '{', '}', '#', '&', '*', '!', '|', '>', '\'', '"', '%', '@', '`':
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf {
			// Non-ASCII whitespace is trimmed or treated as a line break by
			// one reader or another, which a plain scalar cannot survive.
			r, size := utf8.DecodeRune(s[i:])
			if unicode.IsSpace(r) {
				return false
			}
			i += size - 1
			continue
		}
		if c < 0x20 || c == 0x7f || c == backslash || c == '"' {
			return false
		}
		if i+1 == len(s) {
			continue
		}
		if c == ':' && s[i+1] == ' ' {
			return false
		}
		if c == ' ' && s[i+1] == '#' {
			return false
		}
	}
	return !yamlResolvesToNonString(s)
}

// yamlKeyword reports whether s, case-folded, is a plain scalar that YAML
// resolves to a bool, null, or a special float.
func yamlKeyword(s []byte) bool {
	if len(s) > 5 {
		return false
	}
	var buf [5]byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		buf[i] = c
	}
	switch string(buf[:len(s)]) {
	case "true", "false", "null", "~",
		"y", "n", "yes", "no", "on", "off",
		".inf", ".nan", "-.inf", "+.inf":
		return true
	}
	return false
}

// yamlResolvesToNonString reports whether a YAML reader would turn the plain
// scalar s into something other than a string: a bool, null, a number, or a
// timestamp.
func yamlResolvesToNonString(s []byte) bool {
	if yamlKeyword(s) {
		return true
	}
	switch c := s[0]; {
	case '0' <= c && c <= '9', c == '-', c == '+', c == '.':
	default:
		return false
	}
	if yamlNumeric(s) {
		return true
	}
	// sexagesimal times and timestamps: digits joined by the punctuation a
	// YAML reader accepts in a date
	digits := false
	for i := 0; i < len(s); i++ {
		switch {
		case '0' <= s[i] && s[i] <= '9':
			digits = true
		case s[i] == '_' || s[i] == ':' || s[i] == '-' || s[i] == '.' ||
			s[i] == '+' || s[i] == 'T' || s[i] == 'Z' || s[i] == ' ':
		default:
			return false
		}
	}
	return digits
}

// yamlNumeric reports whether s is a YAML number: a decimal integer or float,
// with optional underscore separators, or a based integer (0x, 0o, 0b).
func yamlNumeric(s []byte) bool {
	i := 0
	if s[i] == '+' || s[i] == '-' {
		i++
	}
	if i >= len(s) {
		return false
	}
	if s[i] == '0' && i+1 < len(s) {
		switch s[i+1] {
		case 'x', 'X':
			return baseDigits(s[i+2:], 16)
		case 'o', 'O':
			return baseDigits(s[i+2:], 8)
		case 'b', 'B':
			return baseDigits(s[i+2:], 2)
		}
	}

	digits := false
	for ; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			digits = true
			continue
		}
		if s[i] != '_' {
			break
		}
	}
	if i < len(s) && s[i] == '.' {
		for i++; i < len(s); i++ {
			if s[i] >= '0' && s[i] <= '9' {
				digits = true
				continue
			}
			if s[i] != '_' {
				break
			}
		}
	}
	if !digits {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		expDigits := false
		for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
			expDigits = true
		}
		if !expDigits {
			return false
		}
	}
	return i == len(s)
}

// baseDigits reports whether s is a non-empty run of digits valid in the given
// base, allowing underscore separators.
func baseDigits(s []byte, base int) bool {
	found := false
	for _, c := range s {
		if c == '_' {
			continue
		}
		if hexVal(c) < 0 || hexVal(c) >= base {
			return false
		}
		found = true
	}
	return found
}
