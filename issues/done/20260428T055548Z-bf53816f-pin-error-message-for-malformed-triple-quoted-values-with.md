{
  "title": "Pin error message for malformed triple-quoted values with trailing junk",
  "id": "20260428T055548Z-bf53816f",
  "state": "done",
  "created": "2026-04-28T05:55:48Z",
  "labels": [
    "bug"
  ],
  "assignees": [],
  "milestone": "",
  "projects": [],
  "template": "",
  "events": [
    {
      "ts": "2026-04-28T05:55:48Z",
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

## Symptom

Inputs like `s = '''abc'''xyz` are malformed TOML. `multilineStart` returns single-line (because `s[3:]` contains `'''`), then the single-line value parser sees the whole string and produces some error — but no test currently pins which error or where it surfaces.

## Suspected fix

Add a test that exercises the form and asserts a specific, useful error message at a known line/col. If the current message is poor, improve it in `parseTOMLLiteralStringRaw` / `parseTOMLBasicStringRaw`.

## Resolution

The bug was in `parseTOMLMultilineBasic` / `parseTOMLMultilineLiteral` (not `parseTOMLBasicStringRaw` as originally suspected). Both functions found the closing `"""` / `'''` but never inspected the tail bytes after it, so trailing junk was silently dropped.

Fixed by checking `content[idx+3:]` after the closing delimiter: after trimming whitespace, any non-`#` byte is an error (`"unexpected content after closing \"\"\""`). Tests added to `TestTOMLMultilineBasic` / `TestTOMLMultilineLiteral` pin the error for both `"""abc"""extra` and `'''abc'''extra`.
