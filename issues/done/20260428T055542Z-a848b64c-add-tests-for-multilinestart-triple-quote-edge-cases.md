{
  "title": "Add tests for multilineStart triple-quote edge cases",
  "id": "20260428T055542Z-a848b64c",
  "state": "done",
  "created": "2026-04-28T05:55:42Z",
  "labels": [
    "refactor"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:55:42Z",
      "type": "filed",
      "to": "backlog"
    },
    {
      "ts": "2026-06-12T16:05:35Z",
      "type": "moved",
      "from": "backlog",
      "to": "done"
    }
  ]
}

## Concept

`multilineStart` rejects same-line triple-terminators via `bytes.Contains(s[3:], …)`. Edge cases worth pinning with tests:

- `s = """"""` (empty multi-line basic) — `s[3:] = """` contains `"""`, so single-line. Confirm.
- `s = """abc"""extra` — same: stays single-line. The trailing junk is then a problem for the single-line value parser; pin its error.
- Same matrix for `'''`.

No bug suspected — just no tests.

## Resolution

There was a real bug: `"""abc"""extra` and `'''abc'''extra` silently discarded the trailing junk instead of erroring.

Fixed `parseTOMLMultilineBasic` and `parseTOMLMultilineLiteral` in `toml_scalar.go` to check the tail after the closing delimiter; any non-whitespace, non-comment content returns `"unexpected content after closing \"\"\""` / `"unexpected content after closing '''"`.

Tests added to `TestTOMLMultilineBasic` and `TestTOMLMultilineLiteral`: empty single-line (`""""""`/`''''''`), simple single-line, trailing-comment ok, and trailing-junk error.
