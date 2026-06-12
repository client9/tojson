{
  "title": "Harden multilineStart inline-array detection against trailing-byte ambiguity",
  "id": "20260428T055537Z-cd3ecca2",
  "state": "backlog",
  "created": "2026-04-28T05:55:37Z",
  "labels": [
    "bug",
    "design"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:55:37Z",
      "type": "filed",
      "to": "backlog"
    }
  ]
}

## Concept

`multilineStart` (toml_line.go:38-42) decides whether a `[…]` value is multi-line by checking only `s[len(s)-1] != ']'`. That ignores quoting context.

Examples:
- `arr = ["x] in string"` ends in `"`, so we enter multi-line and eventually error "unterminated inline array" with the start position. Acceptable but not great.
- `arr = [ "abc" ]  # comment` after the related `#`-stripping fix should end in `]` and stay single-line; verify with a test.

## Hard parts

A correct check has to scan the line respecting `"…"`, `'…'`, `"""…"""`, `'''…'''` to know whether the trailing byte is structural or inside a string. That overlaps with `tomlValueEnd` in toml_scalar.go.

## Recommended phasing

1. Add tests pinning current behavior on the ambiguous cases.
2. Decide whether to do a quote-aware scan or accept the current "enter multi-line, error later" behavior as the contract.
3. If scanning, factor the scan from `tomlValueEnd` so both paths share it.
