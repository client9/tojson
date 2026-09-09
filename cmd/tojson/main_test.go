package main

import (
	"errors"
	"testing"
)

type errWriter struct {
	writes int
	failAt int
	err    error
	data   []byte
}

func (w *errWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}
	w.data = append(w.data, p...)
	return len(p), nil
}

func TestWriteOutputCompact(t *testing.T) {
	w := &errWriter{}
	if err := writeOutput(w, []byte(`{"a":1}`), false); err != nil {
		t.Fatalf("writeOutput() error = %v", err)
	}
	if got, want := string(w.data), "{\"a\":1}\n"; got != want {
		t.Fatalf("writeOutput() = %q, want %q", got, want)
	}
}

func TestWriteOutputRaw(t *testing.T) {
	w := &errWriter{}
	if err := writeOutput(w, []byte(`{"a":1}`), true); err != nil {
		t.Fatalf("writeOutput() error = %v", err)
	}
	if got, want := string(w.data), `{"a":1}`; got != want {
		t.Fatalf("writeOutput() = %q, want %q", got, want)
	}
}

func TestWriteOutputBodyError(t *testing.T) {
	wantErr := errors.New("broken pipe")
	w := &errWriter{failAt: 1, err: wantErr}
	if err := writeOutput(w, []byte(`{"a":1}`), false); !errors.Is(err, wantErr) {
		t.Fatalf("writeOutput() error = %v, want %v", err, wantErr)
	}
}

func TestWriteOutputNewlineError(t *testing.T) {
	wantErr := errors.New("broken pipe")
	w := &errWriter{failAt: 2, err: wantErr}
	if err := writeOutput(w, []byte(`{"a":1}`), false); !errors.Is(err, wantErr) {
		t.Fatalf("writeOutput() error = %v, want %v", err, wantErr)
	}
}

// prettyJSON re-indents the converter's bytes rather than unmarshalling and
// marshalling back, so key order and string contents survive untouched.
func TestPrettyJSON(t *testing.T) {
	got, err := prettyJSON([]byte(`{"b":1,"a":{"z":[1,2],"y":"<tag>"}}`))
	if err != nil {
		t.Fatalf("prettyJSON() error = %v", err)
	}
	want := "{\n  \"b\": 1,\n  \"a\": {\n    \"z\": [\n      1,\n      2\n    ],\n    \"y\": \"<tag>\"\n  }\n}"
	if string(got) != want {
		t.Errorf("prettyJSON() =\n%s\nwant\n%s", got, want)
	}
}

func TestPrettyJSONEmpty(t *testing.T) {
	got, err := prettyJSON(nil)
	if err != nil || len(got) != 0 {
		t.Errorf("prettyJSON(nil) = %q, %v; want empty, nil", got, err)
	}
}

func TestPrettyJSONInvalid(t *testing.T) {
	if _, err := prettyJSON([]byte("{oops")); err == nil {
		t.Error("prettyJSON(invalid) = nil error, want error")
	}
}
