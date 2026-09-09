// YAML-to-JSON support. Converts a subset of YAML to JSON without reflection
// or intermediate data structures.
//
// Supported: block mappings, block sequences, flow style, bare/quoted strings (scalars),
// null/bool literals, numbers, nested structures, comments,
// literal (|) and folded (>) strings (block scalars).
//
// Not supported: anchors & aliases, tags, complex keys (? ...).

package tojson

import (
	"bytes"
	"errors"
)

var (
	errSeqTooDeep      = errors.New("sequence nested too deeply")
	errTrailingContent = errors.New("unexpected indentation: line does not belong to any block")
)

func yamlConvert(input []byte) ([]byte, error) {
	var p parser
	if err := p.init(input); err != nil {
		return nil, err
	}
	if len(p.lines) == 0 {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.Grow(len(input) + 64)
	if err := p.parseBlock(-1, &buf); err != nil {
		return nil, err
	}
	// Every line must land somewhere. A leftover line means the document has
	// an indentation the parser could not attach to anything, which would
	// otherwise be dropped without a word.
	if p.pos < len(p.lines) {
		return nil, atLineCol(p.rawIdx[p.pos], p.lines[p.pos].indent, errTrailingContent)
	}
	return buf.Bytes(), nil
}

// --------------------------------------------------------------------------
// Parser
// --------------------------------------------------------------------------

type parser struct {
	lines    []pline
	pos      int
	rawLines [][]byte // original input lines (split on \n, \r stripped)
	rawIdx   []int    // rawIdx[i] = index into rawLines for lines[i]
	compact  int      // current depth of sequences nested within one line
}

type pline struct {
	indent  int
	content []byte // leading whitespace stripped, trailing whitespace stripped
}

// init populates p from input. Kept as a method so yamlConvert can declare
// parser on the stack and avoid the &parser{} heap escape.
func (p *parser) init(input []byte) error {
	// Count newlines for pre-allocation — avoids repeated slice growth.
	n := bytes.Count(input, []byte{'\n'}) + 1

	// Build rawLines without bytes.Split to avoid genSplit's backing alloc;
	// pre-allocate with n so we get exactly one allocation.
	// We must match bytes.Split semantics: always emit one element after the
	// last separator, even when it is empty (so "a\n" → ["a",""] not ["a"]).
	rawLines := make([][]byte, 0, n)
	remaining := input
	for {
		i := bytes.IndexByte(remaining, '\n')
		if i < 0 {
			rawLines = append(rawLines, remaining)
			break
		}
		rawLines = append(rawLines, remaining[:i])
		remaining = remaining[i+1:]
		if len(remaining) == 0 {
			rawLines = append(rawLines, remaining) // trailing empty matches bytes.Split
			break
		}
	}
	// Remove spurious trailing empty element from a \n-terminated input.
	if len(rawLines) > 0 && len(rawLines[len(rawLines)-1]) == 0 {
		rawLines = rawLines[:len(rawLines)-1]
	}

	lines := make([]pline, 0, n)
	rawIdx := make([]int, 0, n)
	for i, raw := range rawLines {
		s := bytes.TrimRight(raw, " \t\r")
		if len(s) == 0 {
			continue
		}
		trimmed := bytes.TrimSpace(s)
		// skip blank, comment-only, and YAML document-marker lines
		if len(trimmed) == 0 || trimmed[0] == '#' ||
			bytes.Equal(trimmed, []byte("---")) || bytes.Equal(trimmed, []byte("...")) {
			continue
		}
		indent, err := yamlLeadingIndent(s)
		if err != nil {
			return atLineCol(i, 0, err)
		}
		content := yamlStripIndent(s, indent)
		// strip inline comment (outside quotes) — best-effort
		content = stripInlineComment(content)
		if len(content) == 0 {
			continue
		}
		lines = append(lines, pline{indent: indent, content: content})
		rawIdx = append(rawIdx, i)
	}
	p.lines = lines
	p.rawLines = rawLines
	p.rawIdx = rawIdx
	return nil
}

func (p *parser) peek() (pline, bool) {
	if p.pos >= len(p.lines) {
		return pline{}, false
	}
	return p.lines[p.pos], true
}

func (p *parser) consume() pline {
	l := p.lines[p.pos]
	p.pos++
	return l
}

// parseBlock writes a JSON value for the block starting at the current
// position. Only considers lines with indent > parentIndent, except that
// a block sequence may begin at the same indent as its parent mapping key
// (YAML compact notation).
func (p *parser) parseBlock(parentIndent int, buf *bytes.Buffer) error {
	l, ok := p.peek()
	if !ok {
		buf.WriteString("null")
		return nil
	}
	if l.indent <= parentIndent {
		// Compact notation: block sequence value at same indent as mapping key.
		if l.indent == parentIndent && isSeqItem(l.content) {
			return p.parseSequence(l.indent, buf)
		}
		buf.WriteString("null")
		return nil
	}
	blockIndent := l.indent

	switch {
	case isSeqItem(l.content):
		return p.parseSequence(blockIndent, buf)
	case isMapKey(l.content):
		return p.parseMapping(blockIndent, buf)
	default:
		p.consume()
		rawLine := p.rawIdx[p.pos-1]
		if style, chomping, ind, ok := detectBlockScalar(l.content); ok {
			scalar, last, err := p.collectBlockScalar(style, chomping, ind, rawLine, parentIndent)
			if err != nil {
				return err
			}
			p.skipPastRawLine(last)
			writeJSONString(scalar, buf)
			return nil
		}
		if isFlowValue(l.content) {
			src, last := p.gatherFlowSrc(l.content, rawLine)
			if err := parseFlowExpr(src, buf); err != nil {
				return atLineCol(rawLine, l.indent, err)
			}
			p.skipPastRawLine(last)
			return nil
		}
		scalar := p.foldPlainScalar(l.content, parentIndent, rawLine)
		if err := writeScalar(scalar, buf); err != nil {
			return atLineCol(rawLine, l.indent, err)
		}
		return nil
	}
}

// foldPlainScalar returns the plain scalar first together with any
// continuation lines that follow it, consuming them. A continuation is a line
// indented past minIndent that is not itself structure. One line break between
// two of them folds to a space, and blank lines between them become newlines.
//
// rawLine is the raw index of the line first came from.
func (p *parser) foldPlainScalar(first []byte, minIndent, rawLine int) []byte {
	// only plain scalars continue; a quoted one is decoded as written
	if len(first) == 0 || first[0] == '"' || first[0] == '\'' {
		return first
	}

	folded := first
	copied := false
	prevRaw := rawLine
	for {
		l, ok := p.peek()
		if !ok || l.indent <= minIndent || isMapKey(l.content) || isSeqItem(l.content) {
			break
		}
		blanks, ok := p.blankGap(prevRaw, p.rawIdx[p.pos])
		if !ok {
			// a comment sits between the two lines; a scalar continuing
			// past one is outside this subset
			break
		}
		if !copied {
			// folded still aliases the input, so copy before growing
			folded = append(make([]byte, 0, len(first)+len(l.content)+8), first...)
			copied = true
		}
		if blanks == 0 {
			folded = append(folded, ' ')
		}
		for ; blanks > 0; blanks-- {
			folded = append(folded, '\n')
		}
		folded = append(folded, l.content...)
		prevRaw = p.rawIdx[p.pos]
		p.consume()
	}
	return folded
}

// blankGap counts the raw lines strictly between from and to, reporting false
// if any of them holds something other than whitespace.
func (p *parser) blankGap(from, to int) (int, bool) {
	n := 0
	for i := from + 1; i < to; i++ {
		if len(bytes.TrimSpace(p.rawLines[i])) != 0 {
			return 0, false
		}
		n++
	}
	return n, true
}

// parseMapping writes a JSON object for all map-key lines at indent.
func (p *parser) parseMapping(indent int, buf *bytes.Buffer) error {
	buf.WriteByte('{')
	first := true
	for {
		l, ok := p.peek()
		if !ok || l.indent != indent || !isMapKey(l.content) {
			break
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		p.consume()
		rawLine := p.rawIdx[p.pos-1]

		key, rest, err := splitMapKey(l.content)
		if err != nil {
			return err
		}
		writeJSONString(key, buf)
		buf.WriteByte(':')

		if len(rest) == 0 {
			if err := p.parseBlock(indent, buf); err != nil {
				return err
			}
		} else if style, chomping, ind, ok := detectBlockScalar(rest); ok {
			scalar, last, err := p.collectBlockScalar(style, chomping, ind, rawLine, l.indent)
			if err != nil {
				return err
			}
			p.skipPastRawLine(last)
			writeJSONString(scalar, buf)
		} else if isFlowValue(rest) {
			src, last := p.gatherFlowSrc(rest, rawLine)
			if err := parseFlowExpr(src, buf); err != nil {
				return atLineCol(rawLine, l.indent+len(l.content)-len(rest), err)
			}
			p.skipPastRawLine(last)
		} else {
			scalar := p.foldPlainScalar(rest, l.indent, rawLine)
			if err := writeScalar(scalar, buf); err != nil {
				return atLineCol(rawLine, l.indent+len(l.content)-len(rest), err)
			}
		}
	}
	buf.WriteByte('}')
	return nil
}

// yamlMaxCompactDepth bounds sequences nested inside a single line ("- - - 1").
// Every other recursion in this parser consumes at least one line, so the input
// itself bounds their depth; compact items do not.
const yamlMaxCompactDepth = 100

// seqItemValue splits a sequence-item line into the value written after the
// dash and the column that value starts at. content must satisfy isSeqItem.
func seqItemValue(content []byte, indent int) (rest []byte, col int) {
	rest = bytes.TrimLeft(content[1:], " \t")
	return rest, indent + len(content) - len(rest)
}

// parseSequence writes a JSON array for all sequence-item lines at indent.
func (p *parser) parseSequence(indent int, buf *bytes.Buffer) error {
	buf.WriteByte('[')
	first := true
	for {
		l, ok := p.peek()
		if !ok || l.indent != indent || !isSeqItem(l.content) {
			break
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		p.consume()
		rawLine := p.rawIdx[p.pos-1]

		rest, col := seqItemValue(l.content, l.indent)
		if err := p.writeSeqItem(rest, l.indent, col, rawLine, buf); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

// parseCompactSequence writes a JSON array for a sequence that starts on the
// same line as its parent's dash:
//
//   - - 1
//   - 2
//
// first is the text following the parent dash and virtIndent is the column it
// begins at, which is the indentation the sequence's later items must use.
func (p *parser) parseCompactSequence(first []byte, virtIndent, rawLine int, buf *bytes.Buffer) error {
	if p.compact++; p.compact > yamlMaxCompactDepth {
		return atLineCol(rawLine, virtIndent, errSeqTooDeep)
	}
	defer func() { p.compact-- }()

	buf.WriteByte('[')
	rest, col := seqItemValue(first, virtIndent)
	if err := p.writeSeqItem(rest, virtIndent, col, rawLine, buf); err != nil {
		return err
	}
	for {
		l, ok := p.peek()
		if !ok || l.indent != virtIndent || !isSeqItem(l.content) {
			break
		}
		buf.WriteByte(',')
		p.consume()
		itemRaw := p.rawIdx[p.pos-1]
		rest, col := seqItemValue(l.content, l.indent)
		if err := p.writeSeqItem(rest, l.indent, col, itemRaw, buf); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

// writeSeqItem writes the value of one sequence item. rest is the text after
// the dash, itemIndent the indentation of the sequence the item belongs to,
// and col the column rest starts at.
func (p *parser) writeSeqItem(rest []byte, itemIndent, col, rawLine int, buf *bytes.Buffer) error {
	if len(rest) == 0 {
		return p.parseSeqItemBlock(itemIndent, buf)
	}
	if isSeqItem(rest) {
		return p.parseCompactSequence(rest, col, rawLine, buf)
	}
	if style, chomping, ind, ok := detectBlockScalar(rest); ok {
		scalar, last, err := p.collectBlockScalar(style, chomping, ind, rawLine, itemIndent)
		if err != nil {
			return err
		}
		p.skipPastRawLine(last)
		writeJSONString(scalar, buf)
		return nil
	}
	if isFlowValue(rest) {
		src, last := p.gatherFlowSrc(rest, rawLine)
		if err := parseFlowExpr(src, buf); err != nil {
			return atLineCol(rawLine, col, err)
		}
		p.skipPastRawLine(last)
		return nil
	}
	if isMapKey(rest) {
		return p.parseInlineMap(rest, col, rawLine, buf)
	}
	scalar := p.foldPlainScalar(rest, itemIndent, rawLine)
	if err := writeScalar(scalar, buf); err != nil {
		return atLineCol(rawLine, col, err)
	}
	return nil
}

// parseSeqItemBlock writes the value of a sequence item with nothing after its
// dash. Unlike a mapping value, a sequence item at the item's own indentation
// is the next sibling rather than a nested block, so this item is null.
func (p *parser) parseSeqItemBlock(indent int, buf *bytes.Buffer) error {
	if l, ok := p.peek(); ok && l.indent <= indent {
		buf.WriteString("null")
		return nil
	}
	return p.parseBlock(indent, buf)
}

// parseInlineMap handles the case where a sequence item starts an inline
// mapping on the same line as the dash, e.g.:
//
//   - name: Alice
//     age: 30
//
// virtIndent is the column the key starts at, which is the indentation the
// mapping's later keys must use.
func (p *parser) parseInlineMap(firstLine []byte, virtIndent int, startRawLine int, buf *bytes.Buffer) error {
	buf.WriteByte('{')

	writeKeyValue := func(line []byte, rawLine int, lineCol int) error {
		key, rest, err := splitMapKey(line)
		if err != nil {
			return err
		}
		writeJSONString(key, buf)
		buf.WriteByte(':')
		if len(rest) == 0 {
			// virtIndent, not one less: a line at the mapping's own
			// indentation is the next key, not this key's value.
			if err := p.parseBlock(virtIndent, buf); err != nil {
				return err
			}
		} else if style, chomping, ind, ok := detectBlockScalar(rest); ok {
			scalar, last, err := p.collectBlockScalar(style, chomping, ind, rawLine, lineCol)
			if err != nil {
				return err
			}
			p.skipPastRawLine(last)
			writeJSONString(scalar, buf)
		} else if isFlowValue(rest) {
			src, last := p.gatherFlowSrc(rest, rawLine)
			if err := parseFlowExpr(src, buf); err != nil {
				return atLineCol(rawLine, lineCol+len(line)-len(rest), err)
			}
			p.skipPastRawLine(last)
		} else {
			scalar := p.foldPlainScalar(rest, lineCol, rawLine)
			if err := writeScalar(scalar, buf); err != nil {
				return atLineCol(rawLine, lineCol+len(line)-len(rest), err)
			}
		}
		return nil
	}

	if err := writeKeyValue(firstLine, startRawLine, virtIndent); err != nil {
		return err
	}

	for {
		l, ok := p.peek()
		if !ok || l.indent != virtIndent || !isMapKey(l.content) {
			break
		}
		buf.WriteByte(',')
		p.consume()
		rawLine := p.rawIdx[p.pos-1]
		if err := writeKeyValue(l.content, rawLine, l.indent); err != nil {
			return err
		}
	}

	buf.WriteByte('}')
	return nil
}

// --------------------------------------------------------------------------
// Block scalar support (| and >)
// --------------------------------------------------------------------------

// detectBlockScalar returns the style ('|' or '>'), chomping ('-' strip,
// '+' keep, 0 clip/default), the explicit indentation indicator (0 if absent,
// meaning auto-detect), and ok=true if s is a block scalar indicator.
//
// The two indicators may appear in either order ("|2-" and "|-2"), but each
// at most once.
func detectBlockScalar(s []byte) (style, chomping byte, indent int, ok bool) {
	s = bytes.TrimSpace(s)
	if len(s) == 0 || (s[0] != '|' && s[0] != '>') {
		return 0, 0, 0, false
	}
	style = s[0]
	for _, c := range s[1:] {
		switch {
		case c == '-' || c == '+':
			if chomping != 0 {
				return 0, 0, 0, false
			}
			chomping = c
		case c >= '1' && c <= '9':
			if indent != 0 {
				return 0, 0, 0, false
			}
			indent = int(c - '0')
		default:
			return 0, 0, 0, false
		}
	}
	return style, chomping, indent, true
}

// blockLine is one collected line of a block scalar. text is the line with the
// block indentation prefix removed; blank marks an empty line; more marks a
// line indented deeper than the block indentation, which folding leaves alone.
type blockLine struct {
	text  []byte
	blank bool
	more  bool
}

// collectBlockScalar reads the raw lines following rawLineIdx to build a
// literal (style='|') or folded (style='>') scalar, per YAML 1.2 section 8.1.
//
// indentIndicator is the explicit indentation indicator from the header, or 0
// to auto-detect from the first non-empty line. It counts from parentIndent,
// the indentation of the block scalar's parent node.
func (p *parser) collectBlockScalar(style, chomping byte, indentIndicator, rawLineIdx, parentIndent int) ([]byte, int, error) {
	blockIndent := -1
	if indentIndicator > 0 {
		// The indicator counts from the parent's indentation. A block scalar
		// that is the whole document has no parent, and the -1 that stands in
		// for one there would shift its content a column left.
		if parentIndent < 0 {
			parentIndent = 0
		}
		blockIndent = parentIndent + indentIndicator
	}
	var lines []blockLine
	leadingBlanks := 0
	lastIdx := rawLineIdx

	for i := rawLineIdx + 1; i < len(p.rawLines); i++ {
		raw := bytes.TrimRight(p.rawLines[i], "\r")
		blank := len(bytes.TrimSpace(raw)) == 0

		// Empty lines ahead of the first content line cannot be placed until
		// that line fixes the block indentation, so hold them.
		if blank && blockIndent < 0 {
			leadingBlanks++
			lastIdx = i
			continue
		}

		ind, err := yamlLeadingIndent(raw)
		if err != nil {
			return nil, -1, atLineCol(i, 0, err)
		}

		if blank {
			// An all-whitespace line never ends the block: short ones are
			// empty lines, and ones reaching past the block indentation carry
			// that trailing whitespace as content.
			if ind > blockIndent && len(raw) >= blockIndent {
				lines = append(lines, blockLine{text: yamlStripIndent(raw, blockIndent), more: true})
			} else {
				lines = append(lines, blockLine{blank: true})
			}
			lastIdx = i
			continue
		}

		if blockIndent < 0 {
			if ind <= parentIndent {
				break
			}
			blockIndent = ind
			for ; leadingBlanks > 0; leadingBlanks-- {
				lines = append(lines, blockLine{blank: true})
			}
		}
		if ind < blockIndent {
			break
		}
		lines = append(lines, blockLine{text: yamlStripIndent(raw, blockIndent), more: ind > blockIndent})
		lastIdx = i
	}

	// A block with no content line at all is just its empty lines.
	for ; leadingBlanks > 0; leadingBlanks-- {
		lines = append(lines, blockLine{blank: true})
	}

	// Trailing empty lines are governed by the chomping indicator, not by the
	// folding rules, so split them off first.
	end := len(lines)
	for end > 0 && lines[end-1].blank {
		end--
	}
	trailing := len(lines) - end
	lines = lines[:end]

	var out bytes.Buffer
	if style == '|' {
		for i, ln := range lines {
			if i > 0 {
				out.WriteByte('\n')
			}
			out.Write(ln.text)
		}
	} else {
		// Folding (section 8.1.3): between two content lines that are both at
		// the block indentation, a lone break becomes a space and a run of n
		// empty lines becomes n newlines. A more-indented line on either side
		// of a break suppresses that folding, keeping all n+1 breaks.
		emitted := false
		emptyRun := 0
		prevMore := false
		for _, ln := range lines {
			if ln.blank {
				emptyRun++
				continue
			}
			switch {
			case !emitted:
				// Leading empty lines: one newline each, nothing to fold onto.
				for ; emptyRun > 0; emptyRun-- {
					out.WriteByte('\n')
				}
			case prevMore || ln.more:
				// A more-indented line on either side keeps every break.
				for n := emptyRun + 1; n > 0; n-- {
					out.WriteByte('\n')
				}
				emptyRun = 0
			case emptyRun == 0:
				out.WriteByte(' ')
			default:
				// The break onto the empty run folds away, leaving one
				// newline per empty line.
				for ; emptyRun > 0; emptyRun-- {
					out.WriteByte('\n')
				}
			}
			out.Write(ln.text)
			emitted = true
			prevMore = ln.more
		}
	}
	result := out.Bytes()

	switch chomping {
	case '-': // strip: drop the final break and any trailing empty lines
	case '+': // keep: retain the final break and every trailing empty line
		if len(lines) > 0 {
			result = append(result, '\n')
		}
		for i := 0; i < trailing; i++ {
			result = append(result, '\n')
		}
	default: // clip: keep a single final break, drop trailing empty lines
		if len(lines) > 0 {
			result = append(result, '\n')
		}
	}

	return result, lastIdx, nil
}

// skipPastRawLine advances p.pos past all plines whose raw-line index is ≤ lastRawIdx.
func (p *parser) skipPastRawLine(lastRawIdx int) {
	for p.pos < len(p.lines) && p.rawIdx[p.pos] <= lastRawIdx {
		p.pos++
	}
}
