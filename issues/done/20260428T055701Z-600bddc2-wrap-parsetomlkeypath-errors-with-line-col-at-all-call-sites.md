{
  "title": "Wrap parseTOMLKeyPath errors with line/col at all call sites",
  "id": "20260428T055701Z-600bddc2",
  "state": "done",
  "created": "2026-04-28T05:57:01Z",
  "labels": [
    "bug"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:57:01Z",
      "type": "filed",
      "to": "backlog"
    },
    {
      "ts": "2026-06-12T12:49:00Z",
      "type": "moved",
      "from": "backlog",
      "to": "done"
    }
  ]
}

## Symptom

`parseTOMLKeyPath` returns plain errors. The line-parser callers (`handleHeader`, `handleDottedKeyValue` in toml_line.go) wrap with `atLineCol`. Other call sites — notably `parseTOMLInlineTable` in toml_scalar.go — don't, so errors from key parsing inside an inline table surface without source position.

## Suspected fix

Audit all `parseTOMLKeyPath` call sites; ensure every one wraps the error with `atLineCol` (or the column-relative variant the caller has access to).

## Tests to add

- Inline table with a malformed key: `t = { "unterminated = 1 }` — error should carry a line number.

## Resolution

No code change needed — the `atLineCol` wrapping already present at the outer callers (`writeValue` and `handleDottedKeyValue` in `toml_line.go`, `parseKeyValue` in `toml_tree.go`) provides the line number. `parseTOMLKeyPath` errors from inside `parseTOMLInlineTable` bubble up as plain errors through `parseTOMLValue`/`writeTOMLValue`, then get wrapped at the value-write level. The column in the resulting `ParseError` points to the opening `{` of the inline table (the start of the value expression), not the exact character — that precision would require threading column offsets through the inline-table parser, which is out of scope here.

What landed:
- `toml_test.go`: new `TestTOMLErrorInlineTableBadKey` — asserts that `parseTOMLKeyPath` failures inside inline tables surface as `*ParseError` with the correct line number, for both line 1 and line 2 inputs. Runs against all three parsers.
