{
  "title": "Fix stripInlineComment '' handling for TOML literal strings",
  "id": "20260428T055526Z-8630a88e",
  "state": "done",
  "created": "2026-04-28T05:55:26Z",
  "labels": [
    "bug"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:55:26Z",
      "type": "filed",
      "to": "backlog"
    },
    {
      "ts": "2026-06-12T12:42:59Z",
      "type": "moved",
      "from": "backlog",
      "to": "done"
    }
  ]
}

## Symptom

`stripInlineComment` treats `''` inside a single-quoted region as an escaped quote (YAML rule). TOML literal strings cannot contain `'` at all — `''` means "empty literal, then start of next token." The helper may misjudge whether a later `#` is inside or outside a string, either keeping a real comment in the line or stripping part of a value.

In practice the line is then re-parsed by `writeTOMLValue` with correct rules, so most cases recover, but the comment-detection step is wrong.

## Suspected fix

Pairs naturally with the companion issue on `#`-without-whitespace: introduce a TOML-specific `stripTOMLComment` that uses TOML literal-string rules (no escapes inside `'…'`).

## Resolution

Fixed by the companion issue `5bf00f52`. `stripTOMLComment` in `toml_scalar.go` uses TOML literal-string rules: when inside a single-quoted region, the first `'` always ends the string (no `''` escape). Both TOML parser callers (`toml_line.go`, `toml_tree.go`) use `stripTOMLComment`; `stripInlineComment` (YAML `''` escape rules) is only used by YAML callers.

What landed:
- `toml_test.go`: new cases added to `TestTOMLComments` covering literal strings:
  - `s = 'literal with # inside'` — `#` inside must not be stripped
  - `key = ''# comment` — empty literal string; `#` immediately after closing `'` is a comment
  - `key = 'a'# comment` — same pattern with a non-empty value
