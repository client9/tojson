{
  "title": "Apply micro cleanups to TOML fast parser",
  "id": "20260428T055712Z-b18a291c",
  "state": "done",
  "created": "2026-04-28T05:57:12Z",
  "labels": [
    "refactor"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:57:12Z",
      "type": "filed",
      "to": "backlog"
    },
    {
      "ts": "2026-06-12T16:22:19Z",
      "type": "moved",
      "from": "backlog",
      "to": "done"
    }
  ]
}

## Concept

Bag of small style/cleanup items from a code review of `toml.go`, `toml_stack.go`, `toml_line.go`. Each is independently trivial; group them so they can be done in one pass.

## Items

1. **toml.go:24** — `err == errReentry` works fine, but `errors.Is(err, errReentry)` is the modern idiom and costs nothing.
2. **multilineStart return shape** — currently `(bool, int)`; `(int, bool)` with the bool last is more idiomatic Go, or collapse to a single returned state where `tomlStateNormal` means "no multiline."
3. **tomlFrame.usedKeys lifecycle comment** — doc says "lazily allocated" but the field is also `[:0]`-truncated on AoT reuse. One sentence on full lifecycle.
4. **tomlClosedNode.find** — linear scan via `bytes.Equal`. Correct trade-off for typical child counts; add a one-line comment so it doesn't look like an oversight.
5. **convert: lineNum := -1 comment** — already good; just keep it through any refactor of the loop.

## Anti-goals

Don't bundle anything that changes behavior or public API into this. Pure cosmetics only.

## Resolution

All four open items applied:

1. `errors.Is(err, errReentry)` — `toml.go:24`
2. `multilineStart` collapsed to `int` return; `tomlStateNormal` (0) means no multiline — `toml_line.go`
3. `usedKeys` comment updated to mention `[:0]` truncation on AoT reuse — `toml_stack.go:13`
4. `find` comment notes the linear scan is intentional — `toml_stack.go`

Item 5 (`lineNum := -1` comment) was already intact; preserved through the refactor. All tests green.
