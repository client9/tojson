package tojson

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"testing"
)

//go:embed testdata/bench.toml
var benchTOML string

var frontmatter1JSONBytes = []byte(frontmatter1JSON)
var frontmatter1YAMLBytes = []byte(frontmatter1YAML)
var frontmatter1TOMLBytes = []byte(frontmatter1TOML)
var benchTOMLBytes = []byte(benchTOML)

func BenchmarkFromJSONVariantOnly(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := FromJSONVariant(frontmatter1JSONBytes); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkFromYAMLOnly(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := FromYAML(frontmatter1YAMLBytes); err != nil {
			b.Fatal(err)
		}
	}
}

const frontmatter1JSON = `{
   "date": "2024-02-02T04:14:54-08:00",
   "draft": false,
   "genres": [
      "mystery",
      "romance"
   ],
   "tags": [
      "red",
      "blue"
   ],
   "title": "Example",
   "weight": 10,
   "params": {
      "author": "John Smith"
   },
}
`

const frontmatter1TOML = `date = 2024-02-02T04:14:54-08:00
draft = false
genres = ['mystery', 'romance']
tags = ['red', 'blue']
title = 'Example'
weight = 10
[params]
  author = 'John Smith'
`

func BenchmarkFromTOMLOnly(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := FromTOML(frontmatter1TOMLBytes); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromTOMLLineOnly(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := fromTOMLLine(frontmatter1TOMLBytes); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromTOMLTreeOnly(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := fromTOMLTree(frontmatter1TOMLBytes); err != nil {
			b.Fatal(err)
		}
	}
}

// Small-input benchmarks compare line-by-line vs tree on frontmatter1TOML.

func BenchmarkFromTOMLLineSmall(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := fromTOMLLine(frontmatter1TOMLBytes); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromTOMLTreeSmall(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tomlConvertTree(frontmatter1TOMLBytes); err != nil {
			b.Fatal(err)
		}
	}
}

// Large-input benchmarks compare line-by-line vs tree on bench.toml.

func BenchmarkFromTOMLLineLarge(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := fromTOMLLine(benchTOMLBytes); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromTOMLTreeLarge(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tomlConvertTree(benchTOMLBytes); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeTokens(b *testing.B) {
	data, err := os.ReadFile("samples/chromium/runtime_enabled_features.json5")
	if err != nil {
		log.Fatalf("Cant read file - %v", err)
	}

	for b.Loop() {
		rx := newTokenizer(data)
		for {
			_, err := rx.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("Unexpected error %v ", err)
			}
		}
	}
}
func BenchmarkDecodeFile(b *testing.B) {
	data, err := os.ReadFile("samples/chromium/runtime_enabled_features.json5")
	if err != nil {
		log.Fatalf("Cant read file - %v", err)
	}

	dst := bytes.Buffer{}
	dst.Grow(len(data))

	for b.Loop() {
		dst.Reset()
		d := decoder{out: &dst}
		err := d.Translate(data)
		if err != nil && err != io.EOF {
			b.Errorf("JsonRx - Decode failed %v", err)
		}
	}
}
func BenchmarkJson(b *testing.B) {
	data, err := os.ReadFile("samples/chromium/runtime_enabled_features.json5")
	if err != nil {
		log.Fatalf("Cant read file - %v", err)
	}
	var dst bytes.Buffer
	d := decoder{out: &dst}
	err = d.Translate(data)
	if err != nil && err != io.EOF {
		b.Errorf("JsonRx - Decode failed %v", err)
	}
	data = dst.Bytes()
	fmt.Printf("Final size: %d\n", len(data))
	for b.Loop() {
		var out map[string]any
		err := json.Unmarshal(data, &out)
		if err != nil && err != io.EOF {
			b.Fatalf("Decode failed %v", err)
		}
	}
}

func BenchmarkInt(b *testing.B) {
	data := []byte("123456789")
	out := &bytes.Buffer{}
	for b.Loop() {
		out.Reset()
		writeInt(out, data)
	}
}
func BenchmarkFloatFast(b *testing.B) {
	data := []byte("1.23456789")
	out := &bytes.Buffer{}
	for b.Loop() {
		out.Reset()
		writeFloat(out, data)
	}
}
func BenchmarkFloatSlow(b *testing.B) {
	data := []byte("+1.23456789")
	out := &bytes.Buffer{}
	for b.Loop() {
		out.Reset()
		writeFloat(out, data)
	}
}
func BenchmarkHex(b *testing.B) {
	data := []byte("0xDEADbeef")
	out := &bytes.Buffer{}
	for b.Loop() {
		out.Reset()
		writeHex(out, data)
	}
}
func BenchmarkString(b *testing.B) {
	data := []byte("\"a quoted string\"")
	out := &bytes.Buffer{}
	for b.Loop() {
		out.Reset()
		writeString(out, data)
	}
}
func BenchmarkQuotedFast(b *testing.B) {
	data := []byte("abcdefgh1234567890")
	out := &bytes.Buffer{}
	for b.Loop() {
		out.Reset()
		writeQuoted(out, data)
	}
}
func BenchmarkQuotedSlow(b *testing.B) {
	data := []byte("abcdefgh\\n1234567890")
	out := &bytes.Buffer{}
	for b.Loop() {
		out.Reset()
		writeQuoted(out, data)
	}
}
