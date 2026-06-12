{
  "title": "Raise tomlMaxNesting from 4 to 8",
  "id": "20260428T055645Z-6f5ff501",
  "state": "done",
  "created": "2026-04-28T05:56:45Z",
  "labels": [
    "feature"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:56:45Z",
      "type": "filed",
      "to": "backlog"
    },
    {
      "ts": "2026-06-12T12:33:53Z",
      "type": "moved",
      "from": "backlog",
      "to": "done"
    }
  ]
}

## Concept

`tomlMaxNesting = 4` (toml_line.go:48) rejects headers like `[a.b.c.d.e]`. Real-world TOML hits 4 routinely — Cargo's `[profile.release.package."some-crate"]` is exactly 4 segments, right at the boundary. Anything deeper (or anything that wraps Cargo-like configs in another layer) fails.

## Suggested fix

Bump to 8. `stackBuf` becomes `[9]tomlFrame` — small constant cost, no algorithmic impact. Update README/CLAUDE.md.

## Anti-goals

Don't make the limit dynamic. The fixed array is what makes the no-allocation property cheap.

## Resolution

Implemented as proposed.

What landed:
- `toml_line.go`: `tomlMaxNesting` changed from `4` to `8`; `stackBuf` is now `[9]tomlFrame`
- `toml_test.go`: `TestTOMLErrorNestingLimit` updated — at-limit case uses `[a.b.c.d.e.f.g.h]` (8 segments), over-limit uses `[a.b.c.d.e.f.g.h.i]` (9 segments)
- `CLAUDE.md`: updated both references from `tomlMaxNesting = 4` to `tomlMaxNesting = 8`

All tests pass. No algorithmic or allocation impact.
