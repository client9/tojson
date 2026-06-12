{
  "title": "Strip TOML # comments without requiring leading whitespace",
  "id": "20260428T055519Z-5bf00f52",
  "state": "done",
  "created": "2026-04-28T05:55:19Z",
  "labels": [
    "bug"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:55:19Z",
      "type": "filed",
      "to": "backlog"
    },
    {
      "ts": "2026-06-12T12:38:47Z",
      "type": "moved",
      "from": "backlog",
      "to": "done"
    }
  ]
}

## Symptom

`stripInlineComment` (yaml_scalar.go:322) requires the `#` to be preceded by space or tab. That is YAML's rule. TOML's grammar treats any `#` outside a string as a comment start, so `key=1#comment` and `arr=[1,2]#comment` leave `#…` attached to the value and `parseTOMLNumber("1#comment")` errors.

Affects both the line parser (toml_line.go:559) and the tree parser (toml_tree.go:130).

## Suspected fix

Either give TOML its own stripper, or relax the existing helper for TOML callers. Don't change the YAML caller — its whitespace requirement is intentional.

## Tests to add

- `key=1#comment`
- `arr=[1]#comment`
- `key = "v"#comment` (no space before `#`)
- `s = "string with # inside"` must NOT be stripped

## Resolution

Added `stripTOMLComment` in `yaml_scalar.go` alongside the existing `stripInlineComment`. The new function uses TOML rules: any `#` outside a string ends the line (no preceding whitespace required), and single-quoted (literal) strings have no escape mechanism — the first `'` always ends the string. `stripInlineComment` is unchanged; YAML callers continue to require whitespace before `#`.

What landed:
- `yaml_scalar.go`: new `stripTOMLComment` function
- `toml_line.go`: caller switched from `stripInlineComment` to `stripTOMLComment`
- `toml_tree.go`: caller switched from `stripInlineComment` to `stripTOMLComment`
- `toml_test.go`: four new cases added to `TestTOMLComments` covering all cases from the issue

Note: this fix also addresses the companion issue `8630a88e` (TOML literal-string `''` handling), since `stripTOMLComment` uses the correct single-quote rule for TOML.

Follow-ups: close `8630a88e` (resolved by this fix).
