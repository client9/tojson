// toml_line.go — TOML→JSON converter using a top-level line-by-line state machine.
//
// A single outer loop lazily scans newlines and drives a four-state machine.
// Multiline constructs ("""...""", '''...''', multi-line inline arrays)
// accumulate content across iterations rather than pulling lines from a
// pre-split slice inside helper functions.
//
// TOML's root is always a table (the spec defines the document as a hash table),
// so the output always begins and ends with { }.

package tojson

import (
	"bytes"
	"fmt"
)

const (
	tomlStateNormal      = iota // reading section headers and key-value pairs
	tomlStateMLBasic            // accumulating a """...""" basic string
	tomlStateMLLiteral          // accumulating a '''...''' literal string
	tomlStateInlineArray        // accumulating a [...] inline array spanning lines
)

// multilineStart reports whether the TOML value s requires more than one line,
// returning the state to enter for accumulation. Returns (false, 0) when the value
// fits on the current line and can be written immediately.
func multilineStart(s []byte) (bool, int) {
	switch {
	case len(s) >= 3 && s[0] == '"' && s[1] == '"' && s[2] == '"':
		if !bytes.Contains(s[3:], []byte(`"""`)) {
			return true, tomlStateMLBasic
		}
	case len(s) >= 3 && s[0] == '\'' && s[1] == '\'' && s[2] == '\'':
		if !bytes.Contains(s[3:], []byte("'''")) {
			return true, tomlStateMLLiteral
		}
	case len(s) > 0 && s[0] == '[':
		if s[len(s)-1] != ']' {
			return true, tomlStateInlineArray
		}
	}
	return false, 0
}

// tomlMaxNesting is the maximum number of table-header levels ([a.b.c.d.e.f.g.h] = 8).
// Inputs that nest deeper return an error rather than allocating unboundedly.
const tomlMaxNesting = 8

// dotPath joins the segments of path with '.' for use in human-readable
// error messages. Called only on error paths; do not use on the hot path.
func dotPath(path [][]byte) string {
	if len(path) == 1 {
		return string(path[0])
	}
	return string(bytes.Join(path, []byte(".")))
}

// tomlLineParser holds the mutable state shared across methods during a single
// fromTOMLLine call.
//
// The parser maintains two parallel stacks. stackBuf/stackLen tracks the
// section nesting introduced by [table] and [[array]] headers; entries are
// tomlFrame values keyed by header segment. inlineKeys/inlineComma/inlineUsed
// track dotted-key prefixes opened on the current line (e.g. a.b.c = 1 opens
// the temporary inline objects for a and a.b) and are collapsed back to depth
// zero before any new header or bare key/value pair is processed.
//
// state, accumStart, and startLine drive the multi-line value sub-state
// machine: when a value begins with """, ”', or an unterminated [,
// accumStart records the byte offset in the input slice where the value's
// content begins; the outer loop continues consuming lines until the
// matching terminator is found, at which point input[accumStart:lineEnd]
// is parsed and emitted as a single JSON value.
// arrayDepth/arrayDouble/arraySingle are the bookkeeping fields used by
// scanArrayLine to track bracket nesting and quoted regions while in the
// tomlStateInlineArray state.
type tomlLineParser struct {
	buf         bytes.Buffer
	input       []byte                        // the source slice; sliced on terminator to feed the multi-line parsers
	stackBuf    [tomlMaxNesting + 1]tomlFrame // fixed backing; index 0 is the root frame
	stackLen    int                           // number of active frames in stackBuf
	closed      tomlClosedTables
	inlineKeys  [][]byte
	inlineComma []bool
	inlineUsed  [][][]byte
	state       int
	accumStart  int
	startLine   int
	startCol    int // 0-based column of the first byte of the multi-line value, for error attribution
	arrayDepth  int
	arrayDouble bool
	arraySingle bool
}

// topNC reports whether the innermost open container — an inline dotted-key
// object if one is open, otherwise the topmost section frame — already holds
// at least one entry and therefore needs a leading comma before the next.
func (p *tomlLineParser) topNC() bool {
	if len(p.inlineKeys) > 0 {
		return p.inlineComma[len(p.inlineComma)-1]
	}
	return p.stackBuf[p.stackLen-1].needComma
}

// setTopNC sets the needs-comma flag on whichever container topNC inspects.
func (p *tomlLineParser) setTopNC(v bool) {
	if len(p.inlineKeys) > 0 {
		p.inlineComma[len(p.inlineComma)-1] = v
	} else {
		p.stackBuf[p.stackLen-1].needComma = v
	}
}

// markKey records key as used in the innermost open container and returns an
// error if the same key has already been seen there. The lookup is a linear
// bytes.Equal scan; TOML tables are typically small enough that this beats
// hashing.
func (p *tomlLineParser) markKey(key []byte) error {
	var keys *[][]byte
	if len(p.inlineKeys) > 0 {
		keys = &p.inlineUsed[len(p.inlineUsed)-1]
	} else {
		keys = &p.stackBuf[p.stackLen-1].usedKeys
	}
	for _, k := range *keys {
		if bytes.Equal(k, key) {
			return fmt.Errorf("duplicate key %q", key)
		}
	}
	*keys = append(*keys, key)
	return nil
}

// closeInlineTo pops inline dotted-key frames until exactly depth remain,
// emitting a closing '}' for each one. Passing 0 collapses the inline stack
// entirely — required before any new header or top-level key/value pair.
func (p *tomlLineParser) closeInlineTo(depth int) {
	for len(p.inlineKeys) > depth {
		p.buf.WriteByte('}')
		p.inlineKeys = p.inlineKeys[:len(p.inlineKeys)-1]
		p.inlineComma = p.inlineComma[:len(p.inlineComma)-1]
		p.inlineUsed = p.inlineUsed[:len(p.inlineUsed)-1]
	}
}

// closeSectionsTo pops section frames until stackLen equals depth, emitting
// '}' (or '}]' for an array-of-tables frame) for each one and recording the
// closed paths in p.closed so future re-entry can be detected.
func (p *tomlLineParser) closeSectionsTo(depth int) {
	for p.stackLen > depth {
		top := p.stackBuf[p.stackLen-1]
		p.closed.mark(p.stackBuf[:p.stackLen])
		p.stackLen--
		if top.isAoT {
			p.buf.WriteString("}]")
		} else {
			p.buf.WriteByte('}')
		}
	}
}

// currentSectionIs reports whether the open section stack (excluding the root
// frame at index 0) names exactly path. Used to detect a sibling [[a.b]]
// header that should append a new element to an existing array of tables
// rather than open a fresh nested chain.
func (p *tomlLineParser) currentSectionIs(path [][]byte) bool {
	if len(path) != p.stackLen-1 {
		return false
	}
	for i := range path {
		if !bytes.Equal(p.stackBuf[i+1].key, path[i]) {
			return false
		}
	}
	return true
}

// openSection brings the section stack into the state required by a
// [path] or [[path]] header. It computes the longest common prefix with the
// currently open stack, closes the divergent suffix, then opens any newly
// named segments — emitting the appropriate JSON punctuation as it goes.
//
// For [[...]] headers whose path matches the current AoT frame exactly,
// openSection emits "},{" to start a new array element instead of opening
// a fresh chain. Returns errReentry if path would re-open a table that has
// already been closed by a prior header.
func (p *tomlLineParser) openSection(path [][]byte, isAoT bool) error {
	if isAoT && p.stackLen > 1 {
		top := &p.stackBuf[p.stackLen-1]
		if top.isAoT && p.currentSectionIs(path) {
			p.buf.WriteString("},{")
			top.needComma = false
			top.usedKeys = top.usedKeys[:0]
			return nil
		}
	}
	cd := 0
	for cd < len(path) && cd+1 < p.stackLen {
		if !bytes.Equal(p.stackBuf[cd+1].key, path[cd]) {
			break
		}
		cd++
	}
	if p.closed.reopens(path, cd) {
		return errReentry
	}
	p.closeSectionsTo(cd + 1)
	if cd == len(path) {
		frame := &p.stackBuf[p.stackLen-1]
		if !isAoT {
			if frame.explicit {
				return fmt.Errorf("duplicate table header [%s]", dotPath(path))
			}
			frame.explicit = true
		}
		return nil
	}
	for i := cd; i < len(path); i++ {
		top := &p.stackBuf[p.stackLen-1]
		for _, k := range top.usedKeys {
			if bytes.Equal(k, path[i]) {
				return fmt.Errorf("cannot define table %q: key already has a value", dotPath(path[:i+1]))
			}
		}
		top.usedKeys = append(top.usedKeys, path[i])
		if top.needComma {
			p.buf.WriteByte(',')
		}
		writeJSONString(path[i], &p.buf)
		p.buf.WriteByte(':')
		isAoTFrame := i == len(path)-1 && isAoT
		if isAoTFrame {
			p.buf.WriteString("[{")
		} else {
			p.buf.WriteByte('{')
		}
		top.needComma = true
		if p.stackLen >= len(p.stackBuf) {
			return fmt.Errorf("table nesting exceeds maximum depth of %d", tomlMaxNesting)
		}
		p.stackBuf[p.stackLen] = tomlFrame{
			key:      path[i],
			isAoT:    isAoTFrame,
			explicit: i == len(path)-1 && !isAoT,
		}
		p.stackLen++
	}
	return nil
}

// scanArrayLine advances the inline-array parse state by scanning b,
// returning true when the top-level ']' is reached (arrayDepth → 0).
// A '#' outside any quoted region starts a comment that runs to the end
// of b, so brackets and quotes inside the comment are ignored.
func (p *tomlLineParser) scanArrayLine(b []byte) bool {
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case p.arrayDouble:
			if c == '\\' {
				i++
			} else if c == '"' {
				p.arrayDouble = false
			}
		case p.arraySingle:
			if c == '\'' {
				p.arraySingle = false
			}
		case c == '#':
			return false
		case c == '"':
			p.arrayDouble = true
		case c == '\'':
			p.arraySingle = true
		case c == '[':
			p.arrayDepth++
		case c == ']':
			p.arrayDepth--
			if p.arrayDepth == 0 {
				return true
			}
		}
	}
	return false
}

// finishAccumValue marks the just-emitted value as present and returns the
// parser to tomlStateNormal.
func (p *tomlLineParser) finishAccumValue() {
	p.setTopNC(true)
	p.state = tomlStateNormal
}

// handleAccumLine drives the multi-line accumulation states. The first
// return reports whether the line was consumed by accumulation (true) or
// should be processed as ordinary input (false). When the terminator for
// the current state is found, input[p.accumStart:lineEnd] is handed to the
// matching multi-line parser and emitted as a single JSON value before
// returning true. lineEnd is the offset in p.input of the byte just past
// the current line's content (i.e. the '\n' position, or len(input) at EOF).
func (p *tomlLineParser) handleAccumLine(line []byte, lineEnd int) (bool, error) {
	// String content is raw here; stripping comments or whitespace would corrupt
	// multiline values.
	switch p.state {
	case tomlStateNormal:
		return false, nil
	case tomlStateMLBasic:
		if !bytes.Contains(line, []byte(`"""`)) {
			return true, nil
		}
		str, _, err := parseTOMLMultilineBasic(p.input[p.accumStart:lineEnd], nil, 0)
		if err != nil {
			return true, atLineCol(p.startLine, p.startCol, err)
		}
		writeJSONString(str, &p.buf)
		p.finishAccumValue()
		return true, nil
	case tomlStateMLLiteral:
		if !bytes.Contains(line, []byte("'''")) {
			return true, nil
		}
		str, _, err := parseTOMLMultilineLiteral(p.input[p.accumStart:lineEnd], nil, 0)
		if err != nil {
			return true, atLineCol(p.startLine, p.startCol, err)
		}
		writeJSONString(str, &p.buf)
		p.finishAccumValue()
		return true, nil
	case tomlStateInlineArray:
		if !p.scanArrayLine(line) {
			return true, nil
		}
		if _, err := writeTOMLInlineArray(p.input[p.accumStart:lineEnd], nil, 0, &p.buf); err != nil {
			return true, atLineCol(p.startLine, p.startCol, err)
		}
		p.finishAccumValue()
		return true, nil
	default:
		return true, fmt.Errorf("toml: unknown line parser state %d", p.state)
	}
}

// handleHeader parses a [table] or [[array.of.tables]] header and updates
// the section stack accordingly. trimmed is the line with surrounding
// whitespace removed; lineNum and leading are used to attach source
// positions to any error returned. pathBuf is a caller-owned scratch buffer
// reused across calls to avoid per-line allocations.
func (p *tomlLineParser) handleHeader(trimmed []byte, lineNum, leading int, pathBuf *[tomlMaxNesting][]byte, isAoT bool) error {
	p.closeInlineTo(0)

	var inner []byte
	if isAoT {
		if !bytes.HasSuffix(trimmed, []byte("]]")) {
			return atLineCol(lineNum, leading, fmt.Errorf("malformed array-of-tables header: %s", trimmed))
		}
		inner = trimmed[2 : len(trimmed)-2]
	} else {
		if trimmed[len(trimmed)-1] != ']' {
			return atLineCol(lineNum, leading, fmt.Errorf("malformed table header: %s", trimmed))
		}
		inner = trimmed[1 : len(trimmed)-1]
	}

	path, rest, err := parseTOMLKeyPath(inner, pathBuf[:0])
	if err != nil {
		return atLineCol(lineNum, leading, err)
	}
	if rest = bytes.TrimSpace(rest); len(rest) != 0 {
		if isAoT {
			return atLineCol(lineNum, leading, fmt.Errorf("unexpected content after [[header]]: %s", rest))
		}
		return atLineCol(lineNum, leading, fmt.Errorf("unexpected content after [header]: %s", rest))
	}
	if err := p.openSection(path, isAoT); err != nil {
		if err == errReentry {
			return err
		}
		return atLineCol(lineNum, leading, err)
	}
	return nil
}

// tomlBareKeyValue attempts to split trimmed into a single bare key and the
// value text following the '=' sign. It returns ok == false when the input
// uses a quoted or dotted key, leaving that case to handleDottedKeyValue.
func tomlBareKeyValue(trimmed []byte) (key, rest []byte, ok bool) {
	bareEnd := 0
	for bareEnd < len(trimmed) {
		c := trimmed[bareEnd]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' {
			bareEnd++
		} else {
			break
		}
	}
	eqPos := bareEnd
	for eqPos < len(trimmed) && (trimmed[eqPos] == ' ' || trimmed[eqPos] == '\t') {
		eqPos++
	}
	if bareEnd == 0 || eqPos >= len(trimmed) || trimmed[eqPos] != '=' {
		return nil, nil, false
	}
	return trimmed[:bareEnd], bytes.TrimLeft(trimmed[eqPos+1:], " \t"), true
}

// handleDottedKeyValue handles any key/value line not consumed by the bare
// key fast path: dotted keys such as a.b.c = 1 and quoted keys. It opens
// inline objects for every prefix segment, marks the leaf key as used in its
// parent, then writes the value.
func (p *tomlLineParser) handleDottedKeyValue(trimmed []byte, lineNum, leading int, pathBuf *[tomlMaxNesting][]byte) error {
	path, rest, err := parseTOMLKeyPath(trimmed, pathBuf[:0])
	if err != nil {
		return atLineCol(lineNum, leading, err)
	}
	rest = bytes.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != '=' {
		return atLineCol(lineNum, leading, fmt.Errorf("expected '=' after key, got: %s", rest))
	}
	rest = bytes.TrimSpace(rest[1:])
	valCol := leading + len(trimmed) - len(rest)

	lastKey := path[len(path)-1]
	prefix := path[:len(path)-1]
	if err := p.openInlinePrefix(prefix, lineNum, leading); err != nil {
		return err
	}
	if err := p.markKey(lastKey); err != nil {
		return atLineCol(lineNum, leading, err)
	}
	if p.topNC() {
		p.buf.WriteByte(',')
	}
	writeJSONString(lastKey, &p.buf)
	p.buf.WriteByte(':')
	return p.writeValue(rest, lineNum, valCol)
}

// openInlinePrefix ensures the inline-object stack matches prefix exactly,
// closing any divergent suffix and opening any missing segments. Each newly
// opened segment is marked as a used key in its parent so a later attempt to
// redefine it as a scalar (or vice versa) is rejected.
func (p *tomlLineParser) openInlinePrefix(prefix [][]byte, lineNum, leading int) error {
	if len(prefix) == 0 {
		p.closeInlineTo(0)
		return nil
	}

	cd := 0
	for cd < len(prefix) && cd < len(p.inlineKeys) {
		if !bytes.Equal(p.inlineKeys[cd], prefix[cd]) {
			break
		}
		cd++
	}
	p.closeInlineTo(cd)
	for i := cd; i < len(prefix); i++ {
		if err := p.markKey(prefix[i]); err != nil {
			return atLineCol(lineNum, leading, err)
		}
		if p.topNC() {
			p.buf.WriteByte(',')
		}
		writeJSONString(prefix[i], &p.buf)
		p.buf.WriteByte(':')
		p.buf.WriteByte('{')
		p.setTopNC(true)
		p.inlineKeys = append(p.inlineKeys, prefix[i])
		p.inlineComma = append(p.inlineComma, false)
		p.inlineUsed = append(p.inlineUsed, nil)
	}
	return nil
}

// startMultilineValue switches the parser into one of the multi-line
// accumulation states, recording the offset of rest within p.input as the
// start of the value and lineNum as the value's start line for error
// messages. For inline arrays it also primes the bracket and quote tracking
// by scanning the seed bytes.
//
// rest must be a sub-slice of p.input that has only been narrowed via
// 2-arg slicing (TrimRight, TrimSpace, TrimLeft, s[:n], s[a:]); the
// cap-difference trick below depends on that invariant to recover rest's
// offset without an explicit position parameter.
func (p *tomlLineParser) startMultilineValue(rest []byte, lineNum, valCol, mlState int) {
	p.accumStart = cap(p.input) - cap(rest)
	p.state = mlState
	p.startLine = lineNum
	p.startCol = valCol
	if mlState == tomlStateInlineArray {
		p.arrayDepth, p.arrayDouble, p.arraySingle = 0, false, false
		p.scanArrayLine(rest)
	}
}

// writeValue emits the JSON encoding of a TOML value. If rest opens a
// multi-line construct the parser transitions into the matching accumulation
// state instead and emits nothing until the terminator arrives. valCol is
// the 1-based column of the first byte of rest, used for error positions.
func (p *tomlLineParser) writeValue(rest []byte, lineNum, valCol int) error {
	if ml, mlState := multilineStart(rest); ml {
		p.startMultilineValue(rest, lineNum, valCol, mlState)
		return nil
	}
	if _, err := writeTOMLValue(rest, nil, 0, &p.buf); err != nil {
		return atLineCol(lineNum, valCol, err)
	}
	p.setTopNC(true)
	return nil
}

// fromTOMLLine is the entry point for the line-based TOML→JSON converter.
// A returned errReentry indicates the caller should fall back to a stricter
// parser that handles out-of-order table re-entry.
func fromTOMLLine(input []byte) ([]byte, error) {
	p := &tomlLineParser{stackLen: 1} // stackBuf[0] is the root frame, zero-initialised
	p.buf.Grow(len(input))
	p.buf.WriteByte('{')
	return p.convert(input)
}

// convert walks input one line at a time, dispatching to the header,
// key/value, and accumulation handlers, and returns the emitted JSON
// document.
func (p *tomlLineParser) convert(input []byte) ([]byte, error) {
	p.input = input
	var pathBuf [tomlMaxNesting][]byte
	// lineNum is the 0-based index of the line currently being processed.
	// It is incremented at the top of each loop iteration; -1 here means the
	// first iteration produces lineNum == 0 — the convention atLineCol expects.
	lineNum := -1
	pos := 0

	for pos < len(input) {
		// Lazily scan the next line without pre-splitting the whole input.
		nl := bytes.IndexByte(input[pos:], '\n')
		var line []byte
		var lineEnd int
		if nl < 0 {
			line = input[pos:]
			lineEnd = len(input)
			pos = len(input)
		} else {
			line = input[pos : pos+nl]
			lineEnd = pos + nl
			pos += nl + 1
		}
		lineNum++

		if handled, err := p.handleAccumLine(line, lineEnd); handled || err != nil {
			if err != nil {
				return nil, err
			}
			continue
		}

		line = bytes.TrimRight(line, " \t\r")
		line = stripComment(line, false)
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		leading := leadingSpaces(line)

		switch {
		case bytes.HasPrefix(trimmed, []byte("[[")):
			if err := p.handleHeader(trimmed, lineNum, leading, &pathBuf, true); err != nil {
				return nil, err
			}
		case trimmed[0] == '[':
			if err := p.handleHeader(trimmed, lineNum, leading, &pathBuf, false); err != nil {
				return nil, err
			}
		default:
			key, rest, ok := tomlBareKeyValue(trimmed)
			if !ok {
				if err := p.handleDottedKeyValue(trimmed, lineNum, leading, &pathBuf); err != nil {
					return nil, err
				}
				continue
			}
			p.closeInlineTo(0)
			if err := p.markKey(key); err != nil {
				return nil, atLineCol(lineNum, leading, err)
			}
			if p.topNC() {
				p.buf.WriteByte(',')
			}
			writeJSONString(key, &p.buf)
			p.buf.WriteByte(':')
			valCol := leading + len(trimmed) - len(rest)
			if ml, mlState := multilineStart(rest); ml {
				p.startMultilineValue(rest, lineNum, valCol, mlState)
				continue
			}
			if _, err := writeTOMLValue(rest, nil, 0, &p.buf); err != nil {
				return nil, atLineCol(lineNum, valCol, err)
			}
			p.setTopNC(true)
		}
	}

	if p.state != tomlStateNormal {
		if p.state == tomlStateInlineArray {
			return nil, atLineCol(p.startLine, p.startCol, fmt.Errorf("missing ']' to close inline array"))
		}
		return nil, atLineCol(p.startLine, p.startCol, fmt.Errorf("unterminated multiline string"))
	}

	p.closeInlineTo(0)
	for i := p.stackLen - 1; i >= 1; i-- {
		if p.stackBuf[i].isAoT {
			p.buf.WriteString("}]")
		} else {
			p.buf.WriteByte('}')
		}
	}
	p.buf.WriteByte('}')
	return p.buf.Bytes(), nil
}
