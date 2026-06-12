# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
