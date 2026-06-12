{
  "title": "Deduplicate bare-key emission path in convert",
  "id": "20260428T055654Z-37192ab0",
  "state": "done",
  "created": "2026-04-28T05:56:54Z",
  "labels": [
    "refactor"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:56:54Z",
      "type": "filed",
      "to": "backlog"
    },
    {
      "ts": "2026-06-12T16:19:24Z",
      "type": "moved",
      "from": "backlog",
      "to": "done"
    }
  ]
}

## Concept

`convert` (toml_line.go:576-600) reimplements what `handleDottedKeyValue` does for the single-key case: `markKey`, comma, write key, `:`, `writeValue`. This is faster (skips `parseTOMLKeyPath`) but duplicates emission logic — if the dotted path adds a new validation or changes how it emits, the bare path won't get it.

## Recommended fix

Extract a shared `emitKeyValue(key []byte, rest []byte, lineNum, valCol int)` once correctness tests are in place. The bare-key path calls it directly with the parsed key; the dotted path calls it after opening prefix frames.

## Anti-goals

Don't merge to the point of losing the bare-key fast path's allocation/parse savings — keep the `tomlBareKeyValue` shortcut, just share the emission tail.

## Resolution

Extracted `emitKeyValue(key, rest []byte, lineNum, leading, valCol int) error` (`toml_line.go`). Both `handleDottedKeyValue` and the bare-key path in `convert` now delegate to it. The `tomlBareKeyValue` fast path is preserved — only the emission tail is shared. All tests green.
