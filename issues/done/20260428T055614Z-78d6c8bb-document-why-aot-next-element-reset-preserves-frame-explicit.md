{
  "title": "Document why AoT next-element reset preserves frame.explicit",
  "id": "20260428T055614Z-78d6c8bb",
  "state": "done",
  "created": "2026-04-28T05:56:14Z",
  "labels": [
    "refactor"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:56:14Z",
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

In `openSection` (toml_line.go:192-195), the AoT next-element path resets `top.needComma = false` and `top.usedKeys = top.usedKeys[:0]` but leaves `top.explicit` set. That is correct: the explicit-vs-implicit duplicate-header check only fires in the `cd == len(path) && !isAoT` branch, which `[[…]]` headers never enter.

But the asymmetry looks like an oversight at first glance. Add a one-line comment beside the reset explaining the invariant.

## Suggested wording

```go
// explicit is intentionally not reset: [[…]] never re-checks it,
// so retaining the value avoids losing state if a [table] header
// for the same path arrives later.
```

## Resolution

Added the suggested comment verbatim beside the `usedKeys[:0]` reset in `openSection` (`toml_line.go`). All tests green.
