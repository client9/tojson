// Tree-based TOML→JSON translator. Used as a fallback when the streaming path
// detects an out-of-order section that requires re-entry into a closed table.

package tojson

import (
	"bytes"
	"fmt"
)

// --------------------------------------------------------------------------
// Intermediate JSON node tree
// --------------------------------------------------------------------------

// jnode is a node in the minimal JSON value tree built during TOML parsing.
// Exactly one of raw/obj/arr/aot is non-nil.
type jnode struct {
	raw []byte   // scalar: already-encoded JSON bytes
	obj []*jpair // object: ordered key-value pairs
	arr []*jnode // inline array  (immutable after parse)
	aot []*jnode // array-of-tables (grows with each [[header]])
}

type jpair struct {
	key      []byte
	val      *jnode
	explicit bool // true when created by a [table] header line
}

var (
	nodeTrue  = &jnode{raw: []byte("true")}
	nodeFalse = &jnode{raw: []byte("false")}
)

func newObjectNode() *jnode {
	return &jnode{obj: make([]*jpair, 0, 4)}
}

func newScalarNode(raw []byte) *jnode {
	return &jnode{raw: raw}
}

// findPair returns the jpair with the given key, or nil.
func (n *jnode) findPair(key []byte) *jpair {
	for _, p := range n.obj {
		if bytes.Equal(p.key, key) {
			return p
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// Serializer
// --------------------------------------------------------------------------

func serializeNode(n *jnode, buf *bytes.Buffer) {
	switch {
	case n.raw != nil:
		buf.Write(n.raw)
	case n.obj != nil:
		buf.WriteByte('{')
		for i, p := range n.obj {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(p.key, buf)
			buf.WriteByte(':')
			serializeNode(p.val, buf)
		}
		buf.WriteByte('}')
	case n.arr != nil:
		buf.WriteByte('[')
		for i, elem := range n.arr {
			if i > 0 {
				buf.WriteByte(',')
			}
			serializeNode(elem, buf)
		}
		buf.WriteByte(']')
	case n.aot != nil:
		buf.WriteByte('[')
		for i, elem := range n.aot {
			if i > 0 {
				buf.WriteByte(',')
			}
			serializeNode(elem, buf)
		}
		buf.WriteByte(']')
	}
}

// --------------------------------------------------------------------------
// Parser
// --------------------------------------------------------------------------

type tomlParser struct {
	rawLines [][]byte
	lineIdx  int
	root     *jnode
	ctx      *jnode // current table context (reset by [header] and [[header]])
}

func newTOMLParser(input []byte) *tomlParser {
	lines := bytes.Split(input, []byte{'\n'})
	// remove spurious trailing empty element from Split
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	root := newObjectNode()
	return &tomlParser{rawLines: lines, root: root, ctx: root}
}

func tomlConvertTree(input []byte) ([]byte, error) {
	p := newTOMLParser(input)
	if err := p.parseDocument(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Grow(len(input))
	serializeNode(p.root, &buf)
	return buf.Bytes(), nil
}

func (p *tomlParser) parseDocument() error {
	for p.lineIdx < len(p.rawLines) {
		line := p.rawLines[p.lineIdx]
		p.lineIdx++
		line = bytes.TrimRight(line, " \t\r")
		line = stripComment(line, false)
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		leading := leadingSpaces(line)
		if bytes.HasPrefix(trimmed, []byte("[[")) {
			if err := p.parseArrayTableHeader(trimmed); err != nil {
				return atLineCol(p.lineIdx-1, leading, err)
			}
		} else if trimmed[0] == '[' {
			if err := p.parseTableHeader(trimmed); err != nil {
				return atLineCol(p.lineIdx-1, leading, err)
			}
		} else {
			if err := p.parseKeyValue(trimmed, p.lineIdx-1, leading, p.ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// Table headers
// --------------------------------------------------------------------------

func (p *tomlParser) parseTableHeader(line []byte) error {
	if len(line) < 2 || line[0] != '[' || line[len(line)-1] != ']' {
		return fmt.Errorf("malformed table header: %s", line)
	}
	inner := line[1 : len(line)-1]
	var pathBuf [tomlMaxNesting][]byte
	path, rest, err := parseTOMLKeyPath(inner, pathBuf[:0])
	if err != nil {
		return err
	}
	rest = bytes.TrimSpace(rest)
	if len(rest) != 0 {
		return fmt.Errorf("unexpected content after table header key: %s", rest)
	}
	if len(path) == 0 {
		return fmt.Errorf("empty table header")
	}
	node, err := p.getOrCreateNode(p.root, path[:len(path)-1], false)
	if err != nil {
		return err
	}
	lastKey := path[len(path)-1]
	existing := node.findPair(lastKey)
	if existing != nil {
		switch {
		case existing.val.raw != nil:
			return fmt.Errorf("cannot define table %q: key already has a scalar value", bytes.Join(path, []byte(".")))
		case existing.val.arr != nil:
			return fmt.Errorf("cannot define table %q: key already has an inline array", bytes.Join(path, []byte(".")))
		case existing.explicit:
			return fmt.Errorf("duplicate table header [%s]", bytes.Join(path, []byte(".")))
		case existing.val.aot != nil:
			// [a] after [[a]] — enter the last aot element
			p.ctx = existing.val.aot[len(existing.val.aot)-1]
			return nil
		}
		// implicit object — mark explicit and use it
		existing.explicit = true
		p.ctx = existing.val
		return nil
	}
	newNode := newObjectNode()
	node.obj = append(node.obj, &jpair{key: lastKey, val: newNode, explicit: true})
	p.ctx = newNode
	return nil
}

func (p *tomlParser) parseArrayTableHeader(line []byte) error {
	if len(line) < 4 || !bytes.HasPrefix(line, []byte("[[")) || !bytes.HasSuffix(line, []byte("]]")) {
		return fmt.Errorf("malformed array-of-tables header: %s", line)
	}
	inner := line[2 : len(line)-2]
	var pathBuf [tomlMaxNesting][]byte
	path, rest, err := parseTOMLKeyPath(inner, pathBuf[:0])
	if err != nil {
		return err
	}
	rest = bytes.TrimSpace(rest)
	if len(rest) != 0 {
		return fmt.Errorf("unexpected content after array-of-tables header key: %s", rest)
	}
	if len(path) == 0 {
		return fmt.Errorf("empty array-of-tables header")
	}
	node, err := p.getOrCreateNode(p.root, path[:len(path)-1], false)
	if err != nil {
		return err
	}
	lastKey := path[len(path)-1]
	existing := node.findPair(lastKey)
	newEntry := newObjectNode()
	if existing != nil {
		if existing.val.aot == nil {
			return fmt.Errorf("cannot use [[%s]]: key already exists as a non-array", bytes.Join(path, []byte(".")))
		}
		existing.val.aot = append(existing.val.aot, newEntry)
	} else {
		aotNode := &jnode{aot: []*jnode{newEntry}}
		node.obj = append(node.obj, &jpair{key: lastKey, val: aotNode})
	}
	p.ctx = newEntry
	return nil
}

// getOrCreateNode navigates or creates a path of intermediate object nodes
// under root. Used for table headers and dotted key traversal.
func (p *tomlParser) getOrCreateNode(root *jnode, path [][]byte, _ bool) (*jnode, error) {
	cur := root
	for i, key := range path {
		if cur.obj == nil {
			return nil, fmt.Errorf("cannot navigate into non-object node at %q", bytes.Join(path[:i+1], []byte(".")))
		}
		pair := cur.findPair(key)
		if pair == nil {
			next := newObjectNode()
			cur.obj = append(cur.obj, &jpair{key: key, val: next})
			cur = next
			continue
		}
		v := pair.val
		switch {
		case v.raw != nil:
			return nil, fmt.Errorf("key %q already has a scalar value", bytes.Join(path[:i+1], []byte(".")))
		case v.arr != nil:
			return nil, fmt.Errorf("key %q is an inline array and cannot have subtables", bytes.Join(path[:i+1], []byte(".")))
		case v.aot != nil:
			cur = v.aot[len(v.aot)-1]
		default:
			cur = v
		}
	}
	return cur, nil
}

// --------------------------------------------------------------------------
// Key-value parsing
// --------------------------------------------------------------------------

func (p *tomlParser) parseKeyValue(line []byte, rawLine int, leading int, ctx *jnode) error {
	var pathBuf [tomlMaxNesting][]byte
	path, rest, err := parseTOMLKeyPath(line, pathBuf[:0])
	if err != nil {
		return atLineCol(rawLine, leading, err)
	}
	rest = bytes.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != '=' {
		return atLineCol(rawLine, leading+len(line)-len(rest), fmt.Errorf("expected '=' after key, got: %s", rest))
	}
	rest = bytes.TrimSpace(rest[1:])
	valCol := leading + len(line) - len(rest)

	var targetNode *jnode
	if len(path) > 1 {
		targetNode, err = p.getOrCreateNode(ctx, path[:len(path)-1], false)
		if err != nil {
			return atLineCol(rawLine, leading, err)
		}
	} else {
		targetNode = ctx
	}
	lastKey := path[len(path)-1]
	if targetNode.findPair(lastKey) != nil {
		return atLineCol(rawLine, leading, fmt.Errorf("duplicate key %q", lastKey))
	}

	raw, consumed, err := parseTOMLValue(rest, p.rawLines, p.lineIdx-1)
	if err != nil {
		return atLineCol(rawLine, valCol, err)
	}
	p.lineIdx += consumed

	targetNode.obj = append(targetNode.obj, &jpair{key: lastKey, val: raw})
	return nil
}

// fromTOMLTree converts TOML to JSON using the tree-based path directly,
// skipping the streaming attempt.
func fromTOMLTree(src []byte) ([]byte, error) {
	return tomlConvertTree(src)
}
