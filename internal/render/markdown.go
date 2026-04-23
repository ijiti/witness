package render

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// md is a shared goldmark instance with table and autolink extensions.
// These cover the common patterns in Claude Code task briefs (tables, URLs,
// bold, italics, numbered lists, fenced code blocks). Goldmark is CommonMark-
// conformant and does not make outbound network calls.
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.Table,
		extension.Linkify,
	),
)

// Markdown converts a markdown string to safe HTML for use in templates.
// The returned value is marked template.HTML so Go's html/template will not
// double-escape it. Goldmark escapes any raw HTML present in the source by
// default (WithUnsafe is not set), so injection is not a concern.
func Markdown(s string) template.HTML {
	if s == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(s), &buf); err != nil {
		// Fallback: return the source as plain text inside a <pre>.
		return template.HTML("<pre>" + template.HTMLEscapeString(s) + "</pre>")
	}
	return template.HTML(buf.String())
}
