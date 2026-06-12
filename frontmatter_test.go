package tojson

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestFromFrontMatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMeta string // expected JSON, "" means nil
		wantBody string
		wantErr  bool
	}{
		{
			name:     "no front matter",
			input:    "just body text",
			wantMeta: "",
			wantBody: "just body text",
		},
		{
			name:     "no front matter empty input",
			input:    "",
			wantMeta: "",
			wantBody: "",
		},
		{
			name:     "yaml basic",
			input:    "---\ntitle: Hello\nauthor: alice\n---\nbody text\n",
			wantMeta: `{"title":"Hello","author":"alice"}`,
			wantBody: "body text\n",
		},
		{
			name:     "yaml no trailing newline on body",
			input:    "---\ntitle: Hello\n---\nbody",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body",
		},
		{
			name:     "yaml closing sentinel at EOF without newline",
			input:    "---\ntitle: Hello\n---",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "",
		},
		{
			name:     "yaml empty front matter",
			input:    "---\n---\nbody\n",
			wantMeta: `{}`,
			wantBody: "body\n",
		},
		{
			name:     "toml basic",
			input:    "+++\ntitle = \"Hello\"\nauthor = \"alice\"\n+++\nbody text\n",
			wantMeta: `{"title":"Hello","author":"alice"}`,
			wantBody: "body text\n",
		},
		{
			name:     "toml closing sentinel at EOF without newline",
			input:    "+++\ntitle = \"Hello\"\n+++",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "",
		},
		{
			name:     "toml empty front matter",
			input:    "+++\n+++\nbody\n",
			wantMeta: `{}`,
			wantBody: "body\n",
		},
		{
			name:    "yaml parse error",
			input:   "---\nkey: [unclosed\n---\nbody\n",
			wantErr: true,
		},
		{
			name:     "sentinel-like text not at start",
			input:    "preamble\n---\ntitle: Hello\n---\nbody\n",
			wantMeta: "",
			wantBody: "preamble\n---\ntitle: Hello\n---\nbody\n",
		},
		{
			name:    "unclosed front matter is an error",
			input:   "---\ntitle: Hello\n",
			wantErr: true,
		},

		// --- qualified openers ---
		{
			name:     "---yaml qualifier",
			input:    "---yaml\ntitle: Hello\n---\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "---toml qualifier",
			input:    "---toml\ntitle = \"Hello\"\n---\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "---json qualifier",
			input:    "---json\n{\"title\":\"Hello\"}\n---\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "---json qualifier with whitespace and comments",
			input:    "---json\n{\n  // comment\n  title: 'Hello'\n}\n---\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "---yaml qualifier empty block",
			input:    "---yaml\n---\nbody\n",
			wantMeta: `{}`,
			wantBody: "body\n",
		},
		{
			name:    "---yaml qualifier parse error",
			input:   "---yaml\nkey: [unclosed\n---\nbody\n",
			wantErr: true,
		},
		{
			name:    "---yaml unclosed is an error",
			input:   "---yaml\ntitle: Hello\n",
			wantErr: true,
		},

		// { / } JSON format ---
		{
			name:     "brace json basic",
			input:    "{\n  \"title\": \"Hello\"\n}\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "brace json empty block",
			input:    "{\n}\nbody\n",
			wantMeta: `{}`,
			wantBody: "body\n",
		},
		{
			name:     "brace json at EOF without trailing newline",
			input:    "{\n  \"title\": \"Hello\"\n}",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "",
		},
		{
			name:     "brace json allows JSON5 extensions",
			input:    "{\n  // comment\n  title: 'Hello',\n}\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "brace json not at start treated as no front matter",
			input:    "preamble\n{\n  \"title\": \"Hello\"\n}\nbody\n",
			wantMeta: "",
			wantBody: "preamble\n{\n  \"title\": \"Hello\"\n}\nbody\n",
		},
		{
			name:    "brace json parse error",
			input:   "{\n  title: [unclosed\n}\nbody\n",
			wantErr: true,
		},

		// --- with JSON content (YAML is superset of JSON) ---
		{
			name:     "yaml sentinel with json content",
			input:    "---\n{\"title\":\"Hello\"}\n---\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},

		// trailing whitespace on sentinels ---
		{
			name:     "trailing spaces on opening sentinel",
			input:    "---   \ntitle: Hello\n---\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "trailing tab on opening sentinel",
			input:    "---\t\ntitle: Hello\n---\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "trailing spaces on closing sentinel",
			input:    "---\ntitle: Hello\n---   \nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "trailing spaces on both sentinels",
			input:    "---  \ntitle: Hello\n---  \nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "trailing spaces on ---yaml qualifier",
			input:    "---yaml   \ntitle: Hello\n---\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},

		// bogus qualifiers ---
		{
			name:    "bogus qualifier ---junk",
			input:   "---junk\ntitle: Hello\n---\nbody\n",
			wantErr: true,
		},
		{
			name:    "bogus qualifier ---yml (common typo)",
			input:   "---yml\ntitle: Hello\n---\nbody\n",
			wantErr: true,
		},
		{
			name:    "bogus qualifier ---JSON (wrong case)",
			input:   "---JSON\ntitle: Hello\n---\nbody\n",
			wantErr: true,
		},
		{
			name:     "extra dashes ---- not treated as bogus qualifier",
			input:    "----\ntitle: Hello\n---\nbody\n",
			wantMeta: "",
			wantBody: "----\ntitle: Hello\n---\nbody\n",
		},

		// backtick code-fence qualifiers ---
		{
			name:     "backtick yaml basic",
			input:    "```yaml\ntitle: Hello\nauthor: alice\n```\nbody text\n",
			wantMeta: `{"title":"Hello","author":"alice"}`,
			wantBody: "body text\n",
		},
		{
			name:     "backtick toml basic",
			input:    "```toml\ntitle = \"Hello\"\nauthor = \"alice\"\n```\nbody text\n",
			wantMeta: `{"title":"Hello","author":"alice"}`,
			wantBody: "body text\n",
		},
		{
			name:     "backtick json basic",
			input:    "```json\n{\"title\":\"Hello\"}\n```\nbody text\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body text\n",
		},
		{
			name:     "backtick json with JSON5 extensions",
			input:    "```json\n{\n  // comment\n  title: 'Hello',\n}\n```\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "backtick yaml empty block",
			input:    "```yaml\n```\nbody\n",
			wantMeta: `{}`,
			wantBody: "body\n",
		},
		{
			name:     "backtick yaml closing sentinel at EOF without newline",
			input:    "```yaml\ntitle: Hello\n```",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "",
		},
		{
			name:     "trailing spaces on backtick opening sentinel",
			input:    "```yaml   \ntitle: Hello\n```\nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:     "trailing spaces on backtick closing sentinel",
			input:    "```yaml\ntitle: Hello\n```   \nbody\n",
			wantMeta: `{"title":"Hello"}`,
			wantBody: "body\n",
		},
		{
			name:    "backtick yaml unclosed is an error",
			input:   "```yaml\ntitle: Hello\n",
			wantErr: true,
		},
		{
			name:    "backtick yaml parse error",
			input:   "```yaml\nkey: [unclosed\n```\nbody\n",
			wantErr: true,
		},
		{
			name:     "unqualified backtick is not front matter",
			input:    "```\ntitle: Hello\n```\nbody\n",
			wantErr:  false,
			wantBody: "```\ntitle: Hello\n```\nbody\n",
		},
		{
			name:    "bogus backtick qualifier ```js",
			input:   "```js\ntitle: Hello\n```\nbody\n",
			wantErr: true,
		},
		{
			name:    "bogus backtick qualifier ```YAML (wrong case)",
			input:   "```YAML\ntitle: Hello\n```\nbody\n",
			wantErr: true,
		},

		// missing closing sentinel ---
		{
			name:    "toml unclosed is an error",
			input:   "+++\ntitle = \"Hello\"\n",
			wantErr: true,
		},
		{
			name:    "brace json unclosed is an error",
			input:   "{\n  \"title\": \"Hello\"\n",
			wantErr: true,
		},
	}

	// Verify that a missing closing sentinel reports the correct line number.
	t.Run("missing sentinel line number", func(t *testing.T) {
		// 3 content lines after the opening "---", so the error should be on line 4.
		input := "---\nline1\nline2\nline3\n"
		_, _, err := FromFrontMatter([]byte(input))
		if err == nil {
			t.Fatal("expected error for missing closing sentinel")
		}
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Fatalf("error is not *ParseError: %v", err)
		}
		if pe.Line != 4 {
			t.Errorf("got line %d, want 4", pe.Line)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, body, err := FromFrontMatter([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantMeta == "" {
				if meta != nil {
					t.Errorf("expected nil meta, got %q", meta)
				}
			} else {
				if meta == nil {
					t.Fatal("expected meta, got nil")
				}
				// compare as normalised JSON
				var got, want any
				if err := json.Unmarshal(meta, &got); err != nil {
					t.Fatalf("meta is not valid JSON: %v", err)
				}
				if err := json.Unmarshal([]byte(tt.wantMeta), &want); err != nil {
					t.Fatalf("bad wantMeta: %v", err)
				}
				gotB, _ := json.Marshal(got)
				wantB, _ := json.Marshal(want)
				if string(gotB) != string(wantB) {
					t.Errorf("meta:\n got  %s\n want %s", gotB, wantB)
				}
			}
			if string(body) != tt.wantBody {
				t.Errorf("body:\n got  %q\n want %q", body, tt.wantBody)
			}
		})
	}
}
