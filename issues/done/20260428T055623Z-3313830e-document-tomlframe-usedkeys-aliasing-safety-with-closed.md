{
  "title": "Document tomlFrame.usedKeys aliasing safety with closed-table trie",
  "id": "20260428T055623Z-3313830e",
  "state": "done",
  "created": "2026-04-28T05:56:23Z",
  "labels": [
    "refactor"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:56:23Z",
      "type": "filed",
      "to": "backlog"
    },
    {
      "ts": "2026-06-12T16:24:01Z",
      "type": "moved",
      "from": "backlog",
      "to": "done"
    }
  ]
}

## Concept

`top.usedKeys = top.usedKeys[:0]` (used in AoT next-element handling) keeps the backing array but resets length. Later `append`s overwrite cells whose old `[]byte` headers may also be referenced from `tomlClosedTables` via `mark`'s `child(stack[i].key)`.

This is safe today because the trie copies the slice header — its `tomlClosedNode.key` carries its own ptr/len/cap pointing at the original input bytes (or a heap allocation from a quoted-key decode), not at `usedKeys[i]`. So overwriting `usedKeys` cells does not invalidate the trie's references.

This is the kind of aliasing trap that could regress under refactor if someone changes how keys are sourced. Add a short comment near the `usedKeys[:0]` line and near `tomlClosedNode.key` documenting the invariant: "trie keys must outlive the parse; never store a slice into a reusable scratch buffer."

## Resolution

Added two comments:
- On `usedKeys[:0]` in `openSection` (`toml_line.go`): notes the trie keys are safe because they point into input, not into `usedKeys`.
- On `tomlClosedNode.key` (`toml_stack.go`): states the invariant that the field must never point into a reusable scratch buffer.

All tests green.
