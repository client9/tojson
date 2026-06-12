# tojson

Parse YAML, TOML, JSON variants, and document front matter into standard JSON bytes. Zero dependencies, stdlib only.

[![Go Reference](https://pkg.go.dev/badge/github.com/client9/tojson.svg)](https://pkg.go.dev/github.com/client9/tojson)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Build Status](https://github.com/client9/tojson/actions/workflows/go.yml/badge.svg)](https://github.com/client9/tojson/actions)

This library converts various JSON variants, YAML, and TOML directly ("transpile") into JSON. Then one can use the huge JSON ecosystem and the native stdlib `encoding/json` for futher processing. The performance of conversion and then calling `json.Unmarshal` is simimlar if not siginificantly faster (especailly with json v2) than using specialized libraries. This package also provides a function to split input "front matter" found in blog documents, into metadata and content.  [Reddit](https://www.reddit.com/r/golang/comments/1u0hb2z/comment/or9z6jy/?utm_source=share&utm_medium=web3x&utm_name=web3xcss&utm_term=1&utm_content=share_button)

## Summary

- One library for the configuration and front matter formats you are most likely to encounter.
- Zero dependencies. `tojson` uses the Go standard library only.
- Convert everything to JSON bytes, then use the normal Go JSON ecosystem for unmarshaling, validation, and downstream tooling.
- No custom marshaling layer. Use `json` struct tags only.
- Standardized API and error handling across all supported formats.

Typical use cases:

- High-performance, minimal-dependency YAML config decoding.
- Static site and content pipelines: parse front matter, decode the metadata, pass the body to a renderer.
- Accepting "better JSON" that allows comments and trailing commas.
- Normalizing obsolete JSON variants and recovering broken configurations.

## Quick Start

Requires Go 1.24+.

```bash
go get github.com/client9/tojson
```

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/client9/tojson"
)

type Article struct {
	Title  string `json:"title"`   // use JSON struct tags only
	Author string `json:"author"`
	Draft  bool   `json:"draft"`
}

func main() {

	// YAML SOURCE
	src := []byte(`
title: hello-world
author: alice
draft: false
`))

	// CONVERT TO JSON
	raw, err := tojson.FromYAML(src)
	if err != nil {
		log.Fatal(err)
	}

	// NORMAL UNMARSHAL
	var article Article
	if err := json.Unmarshal(raw, &article); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%+v\n", article)
}
```

## API

```go
tojson.FromJSONVariant(src []byte) ([]byte, error)
tojson.FromYAML(src []byte) ([]byte, error)
tojson.FromTOML(src []byte) ([]byte, error)
tojson.FromFrontMatter(src []byte) (meta []byte, body []byte, err error)
```

`FromJSONVariant`, `FromYAML`, and `FromTOML` return compact JSON on success. `FromFrontMatter` returns compact JSON metadata and the raw body bytes; meta is nil when no front matter is present.

### Error Handling

Parse failures are returned as `*tojson.ParseError`, which includes a 1-based line number and a 1-based column number where the failure occurred.

```
_, err := tojson.FromJSONVariant([]byte("{ unclosed: [1, 2, }"))
if err != nil {
	var pe *tojson.ParseError
	if errors.As(err, &pe) {
		log.Printf("parse error at line %d, col %d: %s", pe.Line, pe.Column, pe.Message)
	}
}
```

## Examples

### JSON variants

```go
src := []byte(`
{
  // comments are allowed
  unquoted: 'value',
  hex: 0x2a,
  trailing: [1, 2, 3,],
}
`)

raw, err := tojson.FromJSONVariant(src)
if err != nil {
	log.Fatal(err)
}

// raw == {"unquoted":"value","hex":42,"trailing":[1,2,3]}
```

### YAML

```go
type Article struct {
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
}

src := []byte(`
title: Hello
tags:
  - go
  - yaml
`)

raw, err := tojson.FromYAML(src)
if err != nil {
	log.Fatal(err)
}

var article Article
if err := json.Unmarshal(raw, &article); err != nil {
	log.Fatal(err)
}
```

### TOML

```go
type Article struct {
	Title  string `json:"title"`
	Author string `json:"author"`
}

src := []byte(`
title = "hello-world"
author = "alice"
`)

raw, err := tojson.FromTOML(src)
if err != nil {
	log.Fatal(err)
}

var article Article
if err := json.Unmarshal(raw, &article); err != nil {
	log.Fatal(err)
}
```

### Front matter

```go
type Article struct {
	Title  string `json:"title"`
	Author string `json:"author"`
}

src := []byte(`---
title: Hello World
author: alice
---
This is the body.
`)

meta, body, err := tojson.FromFrontMatter(src)
if err != nil {
	log.Fatal(err)
}

if meta != nil {
	var article Article
	if err := json.Unmarshal(meta, &article); err != nil {
		log.Fatal(err)
	}
}

// body == []byte("This is the body.\n")
_ = body
```

## Supported Inputs

`FromJSONVariant` handles JSON5, JWCC, HuJSON, JSONC, and HanSON-style inputs: comments, trailing commas, unquoted keys, single-quoted strings, hex literals, and more. `FromYAML` supports a practical subset covering mappings, sequences, scalars, and block strings — not anchors, tags, or complex keys. `FromTOML` accepts valid TOML. `FromFrontMatter` detects the format from the opening sentinel (`---`, `+++`, `{`, or qualified variants like `---toml`).

See [docs/supported-inputs.md](docs/supported-inputs.md) for the full breakdown.

## Performance

On frontmatter-style benchmark inputs in this repo, `FromYAML` used substantially less memory and was several times faster than common Go YAML packages. `FromTOML` used about half the memory of the TOML packages tested, with speed roughly comparable to `pelletier/go-toml` and faster than `BurntSushi/toml`.

See [docs/performance.md](docs/performance.md) for benchmark methodology, exact library comparisons, and raw numbers.

## CLI

The repo also includes a `tojson` command for testing and scripting:

```bash
go install github.com/client9/tojson/cmd/tojson@latest

tojson file.yaml
tojson file.toml
tojson file.json5
cat file.yaml | tojson -f yaml
tojson -pretty file.yaml
```

Use `-f` when reading from stdin so the input format is explicit.

## License

MIT. See [LICENSE.txt](LICENSE.txt)

