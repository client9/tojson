package tojson

import (
	"fmt"
	"io"
)

const leftBrace = '{'
const rightBrace = '}'
const leftBracket = '['
const rightBracket = ']'
const comma = ','
const colon = ':'
const singleQuote = '\''
const doubleQuote = '"'
const backQuote = '`'
const backslash = '\\'
const newline = '\n'
const slash = '/'
const aster = '*'

type token struct {
	kind  byte
	value []byte
	row   int
	col   int
}

func (t token) String() string {
	kind := string([]byte{t.kind})
	return fmt.Sprintf("type %s: %s @ %d:%d", kind, string(t.value), t.row, t.col)
}

type tokenizer struct {
	row  int
	col  int
	data []byte
}

func newTokenizer(b []byte) *tokenizer {
	return &tokenizer{data: b}
}

func (tx *tokenizer) Next() (token, error) {
	if len(tx.data) == 0 {
		return token{}, io.EOF
	}
	//fmt.Printf("Left: %q\n", string(tx.data))
	for i, b := range tx.data {
		switch b {
		// single char tokens
		case leftBrace, rightBrace, leftBracket, rightBracket, comma, colon:
			tx.data = tx.data[i:]
			t := token{
				kind:  b,
				value: tx.data[0:1],
				row:   tx.row,
				col:   tx.col,
			}
			tx.col += 1
			tx.data = tx.data[1:]
			return t, nil
		case ' ', '\t', '\r':
			tx.col += 1
			continue
		case newline:
			tx.row += 1
			tx.col = 0
			continue
		case singleQuote, doubleQuote, backQuote:
			tx.data = tx.data[i:]
			return tx.string()
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '+', '-', '.':
			tx.data = tx.data[i:]
			return tx.number()
		case slash:
			tx.data = tx.data[i:]
			return tx.comment()
		case '#':
			tx.data = tx.data[i:]
			return tx.comment()
		default:
			tx.data = tx.data[i:]
			return tx.bareword()
		}
	}
	return token{}, io.EOF
}

func (tx *tokenizer) string() (token, error) {
	qchar := tx.data[0]

	skip := false
	// lineCol tracks the column of the most recently processed byte on the
	// current line, so we can update tx.col correctly after the string ends.
	lineCol := tx.col // opening quote column
	for i, b := range tx.data[1:] {
		switch b {
		case qchar:
			if skip {
				skip = false
				lineCol++
				continue
			}
			// +1 for the first char we skipped in loop
			// +1 for keeping last quote
			i += 2
			t := token{
				kind:  's',
				value: tx.data[:i],
				row:   tx.row,
				col:   tx.col,
			}
			tx.data = tx.data[i:]
			tx.col = lineCol + 2 // advance past closing quote
			return t, nil
		case backslash:
			// toggle: a backslash that is itself escaped does not escape
			// what follows, so "a\\" ends at the quote after it
			skip = !skip
			lineCol++
		case newline:
			if !skip && qchar == backQuote {
				tx.row += 1
				lineCol = -1
				continue
			}

			if !skip {
				return token{}, &ParseError{Line: tx.row + 1, Column: tx.col + 1, Message: "unescaped newline in string"}
			}
			skip = false
			tx.row += 1
			lineCol = -1
			// remap from \[newline] to \n
			tx.data[i] = 'n'
		default:
			if skip {
				skip = false
			}
			lineCol++
		}
	}
	// always an error
	return token{}, &ParseError{Line: tx.row + 1, Column: tx.col + 1, Message: "quoted string fell off edge"}
}

func (tx *tokenizer) comment() (token, error) {
	if tx.data[0] == '#' {
		return tx.commentSingle()
	}
	if len(tx.data) > 1 {
		switch tx.data[1] {
		case slash:
			return tx.commentSingle()
		case aster:
			return tx.commentMulti()
		}
	}
	// reparse as bareword
	return tx.bareword()
}

func (tx *tokenizer) commentMulti() (token, error) {
	endAster := false
	row := tx.row
	col := tx.col
	tx.col += 2 // account for /*
	for i, b := range tx.data[2:] {
		switch b {
		case newline:
			tx.row += 1
			tx.col = 0
		case aster:
			tx.col += 1
			endAster = true
		case slash:
			tx.col += 1
			if endAster {
				i += 3
				t := token{
					kind:  'c',
					value: tx.data[:i],
					row:   row,
					col:   col,
				}
				tx.data = tx.data[i:]
				return t, nil
			}
			endAster = false
		default:
			endAster = false
			tx.col += 1
		}
	}

	// multi-line comment wasn't closed
	return token{}, &ParseError{Line: row + 1, Column: col + 1, Message: "unterminated block comment"}
}
func (tx *tokenizer) commentSingle() (token, error) {
	t := token{
		kind:  'c',
		value: tx.data,
		row:   tx.row,
		col:   tx.col,
	}
	for i, b := range tx.data {
		if b == newline || b == '\r' {
			t.value = tx.data[:i]
			tx.data = tx.data[i:]
			return t, nil
		}
	}
	tx.data = nil
	return t, nil
}

func (tx *tokenizer) hexnumber() (token, error) {

	kind := byte('2')

	// [2:] is safe since checked already
	for i, b := range tx.data[2:] {
		switch b {

		case leftBrace, rightBrace, leftBracket, rightBracket, colon, comma, ' ', '\t', '\n', '\r':

			t := token{
				kind:  kind,
				value: tx.data[:i+2],
				row:   tx.row,
				col:   tx.col,
			}
			tx.col += i
			tx.data = tx.data[i+2:]
			return t, nil
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f', 'A', 'B', 'C', 'D', 'E', 'F':
			// keep going

		default:
			// non hex characters found
			// treat as regular bareword
			kind = 'w'
		}
	}
	// fell off the edge
	t := token{
		kind:  kind,
		value: tx.data,
		row:   tx.row,
		col:   tx.col,
	}
	tx.col += len(tx.data)
	tx.data = nil
	return t, nil
}
func (tx *tokenizer) number() (token, error) {

	// check if starts with "0x"
	if len(tx.data) > 2 && tx.data[0] == '0' && (tx.data[1] == 'x' || tx.data[1] == 'X') {
		return tx.hexnumber()
	}
	kind := byte('0') // integer
	for i, b := range tx.data {
		switch b {

		case leftBrace, rightBrace, leftBracket, rightBracket, colon, comma, ' ', '\t', '\n', '\r':
			t := token{
				kind:  kind,
				value: tx.data[:i],
				row:   tx.row,
				col:   tx.col,
			}
			tx.col += i
			tx.data = tx.data[i:]
			return t, nil
		case '+', '-':
			// TBD keep going
			// exponential
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			//
		case '.', 'e', 'E':
			// it's not an integer
			// if already marked as 'w', keep as 'w'
			if kind == '0' {
				kind = '1'
			}
		default:
			// this isn't a number
			kind = 'w'
		}
	}
	// fell off the edge
	t := token{
		kind:  kind,
		value: tx.data,
		row:   tx.row,
		col:   tx.col,
	}
	tx.col += len(tx.data)
	tx.data = nil
	return t, nil
}

func (tx *tokenizer) bareword() (token, error) {
	for i, b := range tx.data {
		switch b {
		case leftBrace, rightBrace, leftBracket, rightBracket, colon, comma, ' ', '\t', '\n', '\r':
			t := token{
				kind:  'w',
				value: tx.data[0:i],
				row:   tx.row,
				col:   tx.col,
			}
			tx.data = tx.data[i:]
			tx.col += i
			return t, nil
		default:
		}
	}
	// fell off the edge
	t := token{
		kind:  'w',
		value: tx.data,
		row:   tx.row,
		col:   tx.col,
	}
	tx.col += len(tx.data)
	tx.data = nil
	return t, nil
}
