{
  "title": "Guard the cap-difference offset trick in startMultilineValue",
  "id": "20260428T055634Z-ec27df98",
  "state": "done",
  "created": "2026-04-28T05:56:34Z",
  "labels": [
    "bug",
    "refactor"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:56:34Z",
      "type": "filed",
      "to": "backlog"
    },
    {
      "ts": "2026-06-12T16:15:59Z",
      "type": "moved",
      "from": "backlog",
      "to": "done"
    }
  ]
}

## Concept

`startMultilineValue` recovers `rest`'s offset within `p.input` via `cap(p.input) - cap(rest)`. That works only if every slice operation between `p.input` and `rest` was 2-arg slicing (cap preserved). The doc comment says so.

Audit of the current path confirms the invariant holds:

- `input[pos:pos+nl]` — 2-arg
- `bytes.TrimRight(line, " \t\r")` — `s[:i]`, 2-arg
- `stripInlineComment` — returns `bytes.TrimRight(s[:i], …)` or `s` itself, 2-arg
- `bytes.TrimSpace`, `bytes.TrimLeft` — 2-arg
- `trimmed[eqPos+1:]` — 2-arg

If anyone introduces a 3-arg slice or a `make`+`copy` in this path, `accumStart` will silently point to the wrong byte and multi-line values will decode garbage.

## Suggested mitigation

Either:

1. Add a debug-only sanity check in `startMultilineValue`: `if p.accumStart < 0 || p.accumStart > len(p.input) { panic(...) }`. Cheap and catches any future regression.
2. Add `// MUST: 2-arg slicing only beyond this point` comments at each boundary above.

Option 1 alone is probably sufficient.

## Resolution

Implemented option 1: added a bounds panic in `startMultilineValue` (`toml_line.go:488`) immediately after computing `accumStart`. If a future change violates the 2-arg slicing invariant, it panics with a clear message rather than silently decoding garbage. All tests green.
