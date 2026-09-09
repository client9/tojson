# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `ToYAML` — converts JSON, and the JSON variants `FromJSONVariant` accepts, to
  block-style YAML. Strings become plain scalars where that round-trips,
  literal blocks (`|`) where they contain newlines, and double-quoted scalars
  otherwise. Numbers are copied through without evaluation.
- `ToYAMLStyle` and `YAMLStyle` — set the indent width, whether a multi-line
  string is written as a literal block or a JSON-style quoted scalar, and
  whether a sequence is indented under its key or sits at the key's own
  indentation. The zero value is what `ToYAML` produces. Style never changes
  the document. To indent JSON output, pass a `From*` result to `json.Indent`,
  which re-indents bytes without reflection.
- CLI: `-o yaml` emits YAML instead of JSON, for any input format.
- Fuzz targets and a `make fuzz` target to run them. `FuzzYAMLLayout` builds a
  YAML document and the JSON it must convert to at the same time, so it checks
  `FromYAML` without a reference parser; the other three check that arbitrary
  input never panics, that a successful conversion is always JSON, and that a
  document survives a trip through `ToYAML` and back. Inputs that once failed
  are kept as regression seeds under `testdata/fuzz`.

### Fixed

- `FromJSONVariant` hung forever on an unterminated block comment. It now
  returns a `ParseError` at the position the comment opened.
- `FromJSONVariant` panicked on a comma with no container open, such as `0,0`.
- `FromJSONVariant` produced output that was not JSON for numbers it could not
  repair (`0..`, `0+0`, `1E`), for unquoted values other than null/true/false,
  for a second top-level value, and for a `\u` escape without four hex digits.
  The first four are now parse errors; an incomplete `\u` is written as a
  literal backslash, matching how a malformed `\xNN` was already handled.
- `FromJSONVariant` classified numbers with an uppercase exponent as barewords,
  so `+1E5` skipped normalization and came out as `+1E5`.
- `FromJSONVariant` failed on a string ending with an escaped backslash, such
  as `"C:\\dir\\"`. The tokenizer read the closing quote as escaped and ran
  off the end of the input.
- `FromYAML` now reads multi-line plain scalars, where an unquoted value
  continues on the indented lines below it. `desc: one` followed by an indented
  `two` is the string `one two`. Continuations fold with single spaces and a
  blank line between them becomes a newline. Previously such a document was
  truncated at the wrap, silently dropping the rest of the value and every key
  after it.
- `ToYAML` sent a multi-line string to a JSON-style quoted scalar whenever it
  held a double quote, a backslash or a tab, which is most ordinary prose. A
  literal block carries all three as themselves, so those now come out as
  blocks. Only what a block cannot represent, control characters and carriage
  returns above all, still gets quoted.
- `ToYAML` also quoted a multi-line string that ended in more than one newline.
  It now keeps the trailing blank lines with `|+`.
- `ToYAML` quoted a multi-line string whose lines were indented, which covers
  most embedded code and markup. Indentation below the first line is now
  content of an ordinary block; only a first line that is itself indented needs
  an explicit indentation indicator (`|2`), since that is the line a reader
  takes the block's indentation from.
- `FromYAML` read an indentation indicator on a block scalar that was the whole
  document one column short, so `|2` put its content at column one. A root
  scalar has no parent node to count from and the indicator is now its own
  column, which is what other YAML readers do.
- `ToYAML` wrote strings holding whitespace outside ASCII, such as U+0085 or a
  no-break space, as plain or block scalars, where a reader trims them or
  treats them as a line break. They are double-quoted now.
- `FromYAML` read a sequence nested on one line (`- - 1`) as the plain string
  `"- 1"` and silently discarded every line after it.
- `FromYAML` treated an empty sequence item as a nested sequence, so `-` on its
  own line swallowed the items that followed it instead of yielding null.
- `FromYAML` let a mapping key with an empty value absorb the sibling keys
  below it, when that mapping was opened on a sequence item line.
- `FromYAML` dropped keys of a mapping opened on a sequence item line when
  extra spaces followed the dash, as in `-   name: a`.
- `FromYAML` panicked on tab-indented input where the tab expansion ran past
  the length of the line, such as `"\t\t0"`.
- `FromYAML` treated a scalar with a digitless exponent, such as `0e`, as a
  number and wrote it through unchanged, producing output that was not JSON.
  Those scalars are strings.

### Changed

- CLI: `-pretty` now re-indents with `json.Indent` instead of unmarshalling
  into an `any` tree and marshalling back. Same shape, no reflection, and the
  document keeps its original key order rather than being sorted by key.
  String contents are no longer HTML-escaped on the way out.

- `FromYAML` now returns a `ParseError` when a line does not belong to any
  block, instead of silently discarding it and everything after it. This
  rejects some input that previously converted, always input that was being
  truncated: over-indented lines, and a continuation after a quoted scalar or
  past a comment.

## [1.1.0] - 2026-09-07 

### Fixed

- YAML: Fixed various issues in multiline regarding leading and trailing white
  space and indentation.
- YAML: Fixed parseInlineMap to correctly handle block-scalars and multi-line.

## [1.0.0] - 2026-06-12

First stable release.

### Added

- `FromYAML` — converts a practical YAML subset (mappings, sequences, scalars,
  block strings) to standard JSON bytes. Intentionally excludes anchors/aliases,
  tags, and complex keys.
- `FromTOML` — converts valid TOML documents to standard JSON bytes.
- `FromJSONVariant` — normalizes JSON5, HuJSON, JWCC, JSONC, and HanSON-style
  inputs to strict JSON. Handles comments (`//`, `/* */`, `#`), trailing
  commas, unquoted keys, single-quoted and backtick strings, hex literals
  (`0x…`), and non-finite number clamping.
- `FromFrontMatter` — splits a document into a metadata block and a body,
  converts the metadata to JSON, and returns both. Supports six sentinel
  formats: `---`/`+++`/`{` openers and `---yaml`/`---toml`/`---json`
  qualifiers. Trailing whitespace on sentinels is ignored; unknown qualifiers
  and missing closing sentinels are errors.
- `ParseError` — structured error type returned by all `From*` functions,
  carrying 1-based line and column numbers for precise error reporting.
- `tojson` CLI — converts files or stdin to JSON. Supports `-f` to set the
  input format explicitly and `-pretty` for formatted output.
- Zero dependencies. The entire package uses the Go standard library only.
- Requires Go 1.24+.
