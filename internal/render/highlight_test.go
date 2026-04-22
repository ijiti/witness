package render

import (
	"html/template"
	"strings"
	"testing"
)

func TestHighlightCode_GoKeywords(t *testing.T) {
	got := HighlightCode("main.go", "func main() {\n\treturn nil\n}")
	want := "color:#c084fc"
	if !strings.Contains(string(got), want) {
		t.Errorf("HighlightCode GoKeywords: output does not contain keyword color %q\ngot: %s", want, got)
	}
}

func TestHighlightCode_GoStrings(t *testing.T) {
	got := HighlightCode("main.go", `x := "hello world"`)
	want := "color:#a5d6a7"
	if !strings.Contains(string(got), want) {
		t.Errorf("HighlightCode GoStrings: output does not contain string color %q\ngot: %s", want, got)
	}
}

func TestHighlightCode_GoNumbers(t *testing.T) {
	got := HighlightCode("main.go", "x := 42")
	want := "color:#93c5fd"
	if !strings.Contains(string(got), want) {
		t.Errorf("HighlightCode GoNumbers: output does not contain number color %q\ngot: %s", want, got)
	}
}

func TestHighlightCode_GoComment(t *testing.T) {
	got := HighlightCode("main.go", "// this is a comment")
	if !strings.Contains(string(got), "color:#6b7280") {
		t.Errorf("HighlightCode GoComment: output does not contain comment color #6b7280\ngot: %s", got)
	}
	if !strings.Contains(string(got), "font-style:italic") {
		t.Errorf("HighlightCode GoComment: output does not contain font-style:italic\ngot: %s", got)
	}
}

func TestHighlightCode_Python(t *testing.T) {
	got := HighlightCode("script.py", "def hello():\n    return True")
	want := "color:#c084fc"
	if !strings.Contains(string(got), want) {
		t.Errorf("HighlightCode Python: output does not contain keyword color %q\ngot: %s", want, got)
	}
	// Verify each Python keyword that should be highlighted.
	for _, kw := range []string{"def", "return", "True"} {
		if !strings.Contains(string(got), kw) {
			t.Errorf("HighlightCode Python: output does not contain keyword %q\ngot: %s", kw, got)
		}
	}
}

func TestHighlightCode_JavaScript(t *testing.T) {
	got := HighlightCode("index.js", "const x = 42;")
	want := "color:#c084fc"
	if !strings.Contains(string(got), want) {
		t.Errorf("HighlightCode JavaScript: output does not contain keyword color %q\ngot: %s", want, got)
	}
	if !strings.Contains(string(got), "const") {
		t.Errorf("HighlightCode JavaScript: output does not contain 'const'\ngot: %s", got)
	}
}

func TestHighlightCode_UnknownExtension(t *testing.T) {
	input := "some <html> text"
	got := HighlightCode("README.md", input)

	// No color spans — no highlighting for unknown extension.
	if strings.Contains(string(got), "color:") {
		t.Errorf("HighlightCode UnknownExtension: unexpected color span in output\ngot: %s", got)
	}
	// HTML must be escaped.
	if !strings.Contains(string(got), "&lt;html&gt;") {
		t.Errorf("HighlightCode UnknownExtension: <html> not escaped to &lt;html&gt;\ngot: %s", got)
	}
	// Raw tag must not appear.
	if strings.Contains(string(got), "<html>") {
		t.Errorf("HighlightCode UnknownExtension: raw <html> found in output (not escaped)\ngot: %s", got)
	}
}

func TestHighlightCode_Empty(t *testing.T) {
	got := HighlightCode("main.go", "")
	if got != template.HTML("") {
		t.Errorf("HighlightCode Empty: got %q, want empty string", got)
	}
}

func TestHighlightCode_Truncation(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 600; i++ {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString("x := 1")
	}
	got := HighlightCode("main.go", sb.String())

	wantMsg := "... (100 more lines)"
	if !strings.Contains(string(got), wantMsg) {
		t.Errorf("HighlightCode Truncation: output does not contain %q\ngot: %s", wantMsg, got)
	}
}

func TestHighlightCode_LineNumberPrefix(t *testing.T) {
	// Input is cat -n formatted: line number + → arrow + code.
	input := "     1\u2192func main() {"
	got := HighlightCode("main.go", input)

	// Line number span.
	wantLineNumStyle := "color:#4b5563;user-select:none"
	if !strings.Contains(string(got), wantLineNumStyle) {
		t.Errorf("HighlightCode LineNumberPrefix: output does not contain line number style %q\ngot: %s", wantLineNumStyle, got)
	}
	// Keyword color for func.
	wantKeyword := "color:#c084fc"
	if !strings.Contains(string(got), wantKeyword) {
		t.Errorf("HighlightCode LineNumberPrefix: output does not contain keyword color %q\ngot: %s", wantKeyword, got)
	}
}

func TestSplitLineNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNum  string
		wantCode string
	}{
		{
			name:     "standard prefix",
			input:    "     1\u2192code here",
			wantNum:  "     1\u2192",
			wantCode: "code here",
		},
		{
			name:     "no prefix",
			input:    "no prefix",
			wantNum:  "",
			wantCode: "no prefix",
		},
		{
			name:     "empty string",
			input:    "",
			wantNum:  "",
			wantCode: "",
		},
		{
			name:     "multi-digit line number",
			input:    "   123\u2192long",
			wantNum:  "   123\u2192",
			wantCode: "long",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotNum, gotCode := splitLineNumber(tc.input)
			if gotNum != tc.wantNum {
				t.Errorf("splitLineNumber(%q) num = %q, want %q", tc.input, gotNum, tc.wantNum)
			}
			if gotCode != tc.wantCode {
				t.Errorf("splitLineNumber(%q) code = %q, want %q", tc.input, gotCode, tc.wantCode)
			}
		})
	}
}
