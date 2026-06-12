package tojson

import (
	"bytes"
	"fmt"
)

// fmDef describes one recognised front matter format.
type fmDef struct {
	open   string // opening sentinel line (without \n)
	close  string // closing sentinel line (without \n)
	format string // "yaml", "toml", or "json"
}

// frontMatterFormats lists every supported opening sentinel in match-priority
// order. Longer/more-specific sentinels must come before shorter ones that
// share a prefix (e.g. "---yaml" before "---").
var frontMatterFormats = []fmDef{
	{"---yaml", "---", "yaml"},
	{"---toml", "---", "toml"},
	{"---json", "---", "json"},
	{"---", "---", "yaml"},
	{"+++", "+++", "toml"},
	{"```yaml", "```", "yaml"},
	{"```toml", "```", "toml"},
	{"```json", "```", "json"},
	{"{", "}", "json"},
}

// FromFrontMatter splits a document into a front matter block and a body,
// converts the front matter to JSON, and returns both.
//
// Supported opening/closing sentinel pairs:
//
//	---     / ---   YAML
//	---yaml / ---   YAML (explicit qualifier)
//	---toml / ---   TOML (explicit qualifier)
//	---json / ---   JSON (explicit qualifier)
//	+++     / +++   TOML
//	{       / }     JSON (each sentinel must be the only character on its line)
//
// Trailing whitespace on sentinel lines is ignored on both open and close,
// since invisible characters cause hard-to-diagnose authoring errors.
//
// A ---<qualifier> opening that is not one of the recognised qualifiers above
// returns an error rather than silently treating the file as having no front
// matter, so typos like "---yml" are caught immediately.
//
// A missing closing sentinel is an error. Silently returning no metadata
// would risk leaking private front matter fields into the document body;
// silently parsing the entire file as metadata would produce a confusing
// error far from the actual problem.
//
// On success, meta is compact JSON and body is the bytes after the closing
// sentinel line. If no recognised front matter opening is detected, meta is
// nil, body is the full input, and err is nil. Parse failures inside the
// front matter block are returned as *ParseError.
//
// body is always an unmodified subslice of the input — no copying or
// reallocation occurs. The byte offset of body within src can be recovered
// with len(src)-len(body) when an exact position is needed (e.g. to adjust
// line numbers or byte offsets in downstream error messages).
func FromFrontMatter(in []byte) (meta []byte, body []byte, err error) {
	def, rest, found, err := detectFrontMatterFormat(in)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, in, nil
	}

	block, tail, err := extractFMBlock(rest, def.open, def.close)
	if err != nil {
		return nil, nil, err
	}

	// The { format uses the opening brace as part of the content: reconstruct
	// the full JSON object so the converter receives valid input.
	src := block
	if def.open == "{" {
		if len(bytes.TrimSpace(block)) == 0 {
			return []byte("{}"), tail, nil
		}
		buf := make([]byte, 0, len(block)+4)
		buf = append(buf, '{', '\n')
		buf = append(buf, block...)
		if len(buf) == 0 || buf[len(buf)-1] != '\n' {
			buf = append(buf, '\n')
		}
		buf = append(buf, '}')
		src = buf
	}

	if len(bytes.TrimSpace(src)) == 0 {
		return []byte("{}"), tail, nil
	}

	switch def.format {
	case "yaml":
		meta, err = FromYAML(src)
	case "toml":
		meta, err = FromTOML(src)
	case "json":
		meta, err = FromJSONVariant(src)
	}
	if err != nil {
		return nil, nil, err
	}
	return meta, tail, nil
}

// detectFrontMatterFormat inspects the first line of in and returns the
// matching fmDef and the bytes after the opening sentinel line.
//
// Trailing whitespace on the first line is stripped before matching, so
// "---  \n" is treated identically to "---\n".
//
// If the first line looks like a ---<qualifier> but the qualifier is not
// recognised, an error is returned so the author sees the typo immediately
// rather than getting silently empty metadata.
func detectFrontMatterFormat(in []byte) (def fmDef, rest []byte, found bool, err error) {
	firstLine, after, hasNewline := bytes.Cut(in, []byte("\n"))
	if !hasNewline {
		return fmDef{}, nil, false, nil
	}

	trimmed := string(bytes.TrimRight(firstLine, " \t\r"))

	for _, d := range frontMatterFormats {
		if trimmed == d.open {
			return d, after, true, nil
		}
	}

	// A line that starts with "---" followed by non-dash characters is an
	// attempted qualifier. Treat it as an error rather than no front matter.
	if len(trimmed) > 3 && trimmed[:3] == "---" && trimmed[3] != '-' {
		return fmDef{}, nil, false, &ParseError{
			Line:    1,
			Column:  1,
			Message: fmt.Sprintf("unknown front matter qualifier %q", trimmed[3:]),
		}
	}

	// A line that starts with "```" followed by non-backtick characters is an
	// attempted backtick qualifier. Only json/yaml/toml are supported;
	// unqualified "```" is not supported (too common in Markdown).
	if len(trimmed) > 3 && trimmed[:3] == "```" && trimmed[3] != '`' {
		return fmDef{}, nil, false, &ParseError{
			Line:    1,
			Column:  1,
			Message: fmt.Sprintf("unknown front matter qualifier %q", trimmed[3:]),
		}
	}

	return fmDef{}, nil, false, nil
}

// extractFMBlock scans rest line by line looking for the closing sentinel.
// Trailing whitespace is trimmed from each candidate line before comparison,
// matching the same leniency applied to the opening sentinel.
//
// A missing closing sentinel is an error: silently treating the file as
// having no front matter risks leaking private metadata into the body, and
// silently parsing the whole file as metadata produces confusing errors.
func extractFMBlock(rest []byte, openLine, closeLine string) (block, tail []byte, err error) {
	closeBytes := []byte(closeLine)
	pos := 0
	remaining := rest
	lineNum := 1 // 1-based; rest begins on the line after the opening sentinel

	for len(remaining) > 0 {
		var line []byte
		nl := bytes.IndexByte(remaining, '\n')
		if nl >= 0 {
			line = remaining[:nl]
		} else {
			line = remaining
		}

		if bytes.Equal(bytes.TrimRight(line, " \t\r"), closeBytes) {
			block = rest[:pos]
			if nl >= 0 {
				tail = remaining[nl+1:]
			}
			return block, tail, nil
		}

		if nl < 0 {
			break
		}
		pos += nl + 1
		remaining = remaining[nl+1:]
		lineNum++
	}

	return nil, nil, &ParseError{
		Line:    lineNum,
		Column:  1,
		Message: fmt.Sprintf("front matter opened with %q but no closing %q found", openLine, closeLine),
	}
}
