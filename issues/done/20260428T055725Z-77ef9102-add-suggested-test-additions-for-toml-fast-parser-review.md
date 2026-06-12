{
  "title": "Add suggested test additions for TOML fast parser review",
  "id": "20260428T055725Z-77ef9102",
  "state": "done",
  "created": "2026-04-28T05:57:25Z",
  "labels": [
    "refactor"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:57:25Z",
      "type": "filed",
      "to": "backlog"
    },
    {
      "ts": "2026-06-12T16:05:36Z",
      "type": "moved",
      "from": "backlog",
      "to": "done"
    }
  ]
}

## Concept

Tests called out by the code review of `toml.go`, `toml_stack.go`, `toml_line.go`. Several of these are referenced by individual issues; collecting them here for a single test-writing pass.

## Tests

1. **`#`-without-whitespace stripping** — `key=1#comment`, `arr=[1]#comment`, `key = "v"#comment`. (Pairs with the strip-comment bug issue.)
2. **`#` inside strings** — `s = "string with # inside"` must NOT be stripped; `s = 'literal # here'` likewise.
3. **Cargo-style 4- and 5-segment headers** — `[profile.release.package."some-crate"]` (4 segs, currently at the limit) and a 5-seg variant. Pairs with the nesting-bump issue.
4. **Re-entry matrix** — every ordering of `[a]`, `[a.b]`, `[a.b.c]`, `[[a.b]]`, `[[a.c]]`. Each ordering asserts: success, plain error, or `errReentry` fallback. (See dedicated issue.)
5. **Multi-line value inside dotted key** — `a.b = """...\n..."""` followed by `a.c = 1`, then `[c]`. Verifies inline frame stays open across the multi-line body and closes correctly.
6. **Triple-quote edge cases** — `s = """"""`, `s = """abc"""extra`, same matrix for `'''`.
7. **Inline-array trailing-byte ambiguity** — `arr = ["x] in string"` (currently enters multi-line, errors as unterminated); `arr = [ "abc" ] # comment` (must remain single-line after stripping).
8. **Inline table with malformed key** — error must carry a line number (pairs with parseTOMLKeyPath wrapping issue).

## Resolution

All 8 items addressed:

- Items 1, 2: already present (`TestTOMLComments`).
- Item 3: added `TestTOMLQuotedSegmentInHeader` — `[profile.release.package."some-crate"]` (4-seg) and `[a.b.c.d."e-f"]` (5-seg).
- Item 4: added `TestTOMLHeaderOrderingFastPath` (depth-first, sibling, AoT-sibling orderings that stay on the line parser) and `TestTOMLHeaderOrderingReentry` (cross-branch orderings that trigger `errReentry` and fall back to tree; also pins `errReentry` from `fromTOMLLine` directly).
- Item 5: added `TestTOMLMultilineDottedKey` — `a.b = """…"""\na.c = 1\n[c]`.
- Items 6, 7: fixed and tested in a848b64c / cd3ecca2 sessions.
- Item 8: already present (`TestTOMLErrorInlineTableBadKey`).
