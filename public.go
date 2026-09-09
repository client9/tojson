package tojson

// FromJSONVariant converts JSON and common JSON-derived variants to standard JSON.
// It handles JSON5/HuJSON/JWCC/JSONC/HanSON features such as trailing/leading
// commas, line and block comments, unquoted keys, single-quoted and backtick
// strings, and hex literals.
func FromJSONVariant(src []byte) ([]byte, error) {
	d := &decoder{}
	d.out = &d.buf
	d.stack = d.stackbuf[:0]
	d.buf.Grow(len(src))
	err := d.Translate(src)
	return d.buf.Bytes(), err
}

// FromYAML converts a YAML subset to standard JSON.
// The output can be passed directly to encoding/json.Unmarshal using only json struct tags.
// Anchors/aliases, tags, and complex keys are not supported.
func FromYAML(src []byte) ([]byte, error) {
	return yamlConvert(src)
}

// FromTOML converts TOML to standard JSON.
// The output can be passed directly to encoding/json.Unmarshal using only json struct tags.
func FromTOML(src []byte) ([]byte, error) {
	return tomlConvert(src)
}

// ToYAML converts JSON, and the same JSON variants FromJSONVariant accepts,
// to block-style YAML. Strings are written as plain scalars where that
// round-trips, as literal blocks ("|") where they contain newlines, and as
// double-quoted scalars otherwise. Numbers pass through without evaluation.
func ToYAML(src []byte) ([]byte, error) {
	return yamlEncode(src)
}
