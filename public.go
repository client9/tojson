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
