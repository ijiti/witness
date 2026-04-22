package render

import (
	"fmt"
	"html/template"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// language defines syntax highlighting rules.
type language struct {
	lineComment string
	keywords    *regexp.Regexp
	strings_    *regexp.Regexp
	numbers     *regexp.Regexp
}

var langByExt = map[string]*language{}

var (
	stringPattern = regexp.MustCompile("`[^`]*`" + `|"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)
	numberPattern = regexp.MustCompile(`\b\d+\.?\d*(?:e[+-]?\d+)?\b`)
)

func init() {
	kw := map[string]string{
		"go":   `\b(break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go|goto|if|import|interface|map|package|range|return|select|struct|switch|type|var|nil|true|false|iota|append|len|cap|make|new|close|copy|delete|panic|recover|print|println)\b`,
		"py":   `\b(and|as|assert|async|await|break|class|continue|def|del|elif|else|except|finally|for|from|global|if|import|in|is|lambda|nonlocal|not|or|pass|raise|return|try|while|with|yield|True|False|None|self)\b`,
		"js":   `\b(async|await|break|case|catch|class|const|continue|debugger|default|delete|do|else|export|extends|finally|for|from|function|if|import|in|instanceof|let|new|of|return|static|super|switch|this|throw|try|typeof|var|void|while|with|yield|true|false|null|undefined|console|require|module)\b`,
		"bash": `\b(if|then|else|elif|fi|for|while|do|done|case|esac|in|function|return|exit|local|export|source|alias|set|unset|readonly|declare|typeset|shift|trap|eval|exec|echo|cd|ls|cat|grep|sed|awk|find|test|read)\b`,
		"json": `\b(true|false|null)\b`,
		"yaml": `\b(true|false|null|yes|no|on|off)\b`,
		"html": `(<\/?[a-zA-Z][a-zA-Z0-9]*)`,
		"css":  `\b(color|background|border|margin|padding|font|display|position|width|height|flex|grid|transition|animation|transform|opacity|overflow|z-index|cursor|content|align|justify|text|box|max|min)\b`,
		"rust": `\b(as|async|await|break|const|continue|crate|dyn|else|enum|extern|false|fn|for|if|impl|in|let|loop|match|mod|move|mut|pub|ref|return|self|Self|static|struct|super|trait|true|type|unsafe|use|where|while|Box|Vec|String|Option|Result|Some|None|Ok|Err|println|eprintln|format)\b`,
		"toml": `\b(true|false)\b`,
		"sql":  `(?i)\b(SELECT|FROM|WHERE|INSERT|INTO|UPDATE|DELETE|CREATE|DROP|ALTER|TABLE|INDEX|JOIN|LEFT|RIGHT|INNER|OUTER|ON|AND|OR|NOT|NULL|IS|IN|EXISTS|BETWEEN|LIKE|ORDER|BY|GROUP|HAVING|LIMIT|OFFSET|AS|SET|VALUES|PRIMARY|KEY|FOREIGN|REFERENCES|UNIQUE|DEFAULT|CHECK|CONSTRAINT|DISTINCT|UNION|ALL|CASE|WHEN|THEN|ELSE|END|COUNT|SUM|AVG|MAX|MIN)\b`,
	}

	register := func(ext, langName, comment string) {
		l := &language{
			lineComment: comment,
			strings_:    stringPattern,
			numbers:     numberPattern,
		}
		if kwp, ok := kw[langName]; ok && kwp != "" {
			l.keywords = regexp.MustCompile(kwp)
		}
		langByExt[ext] = l
	}

	register(".go", "go", "//")
	register(".py", "py", "#")
	register(".js", "js", "//")
	register(".ts", "js", "//")
	register(".tsx", "js", "//")
	register(".jsx", "js", "//")
	register(".mjs", "js", "//")
	register(".sh", "bash", "#")
	register(".bash", "bash", "#")
	register(".zsh", "bash", "#")
	register(".json", "json", "")
	register(".jsonl", "json", "")
	register(".yaml", "yaml", "#")
	register(".yml", "yaml", "#")
	register(".html", "html", "")
	register(".htm", "html", "")
	register(".css", "css", "/*")
	register(".rs", "rust", "//")
	register(".toml", "toml", "#")
	register(".sql", "sql", "--")
	register(".mod", "go", "//")
	register(".sum", "go", "")
}

// HighlightCode applies syntax highlighting to file content.
// Input may be cat -n formatted (line numbers with → arrows).
// Returns HTML with inline-styled <span> elements.
func HighlightCode(filePath, content string) template.HTML {
	lang := langByExt[filepath.Ext(filePath)]

	lines := strings.Split(content, "\n")
	maxLines := 500
	truncated := false
	if len(lines) > maxLines {
		truncated = true
		lines = lines[:maxLines]
	}

	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}

		// Separate line number prefix from code.
		lineNum, codePart := splitLineNumber(line)

		if lineNum != "" {
			b.WriteString(`<span style="color:#4b5563;user-select:none">`)
			b.WriteString(template.HTMLEscapeString(lineNum))
			b.WriteString("</span>")
		}

		if lang == nil {
			b.WriteString(template.HTMLEscapeString(codePart))
			continue
		}

		b.WriteString(highlightLine(codePart, lang))
	}

	if truncated {
		b.WriteString(fmt.Sprintf("\n<span style=\"color:#6b7280\">... (%d more lines)</span>", len(strings.Split(content, "\n"))-maxLines))
	}

	return template.HTML(b.String())
}

// splitLineNumber separates a "     1→content" prefix from the code.
func splitLineNumber(line string) (string, string) {
	// Look for the Unicode right arrow (→, 3 bytes in UTF-8).
	idx := strings.Index(line, "\u2192")
	if idx >= 0 && idx < 15 {
		return line[:idx+3], line[idx+3:]
	}
	return "", line
}

type token struct {
	start, end int
	color      string
}

const (
	colorString  = "#a5d6a7" // green
	colorKeyword = "#c084fc" // purple
	colorNumber  = "#93c5fd" // blue
	colorComment = "#6b7280" // gray
)

// highlightLine applies token highlighting to a single code line.
func highlightLine(line string, lang *language) string {
	if line == "" {
		return ""
	}

	// Check for line comment first — highlight entire line.
	if lang.lineComment != "" {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, lang.lineComment) {
			return `<span style="color:` + colorComment + `;font-style:italic">` + template.HTMLEscapeString(line) + "</span>"
		}
	}

	var tokens []token

	// Strings (highest priority).
	if lang.strings_ != nil {
		for _, loc := range lang.strings_.FindAllStringIndex(line, -1) {
			tokens = append(tokens, token{loc[0], loc[1], colorString})
		}
	}

	// Keywords.
	if lang.keywords != nil {
		for _, loc := range lang.keywords.FindAllStringIndex(line, -1) {
			tokens = append(tokens, token{loc[0], loc[1], colorKeyword})
		}
	}

	// Numbers.
	if lang.numbers != nil {
		for _, loc := range lang.numbers.FindAllStringIndex(line, -1) {
			tokens = append(tokens, token{loc[0], loc[1], colorNumber})
		}
	}

	if len(tokens) == 0 {
		return template.HTMLEscapeString(line)
	}

	// Mark covered ranges (strings have priority).
	covered := make([]bool, len(line))
	var final []token

	// First pass: strings.
	for _, t := range tokens {
		if t.color == colorString {
			for k := t.start; k < t.end && k < len(covered); k++ {
				covered[k] = true
			}
			final = append(final, t)
		}
	}
	// Second pass: keywords and numbers, skip if overlapping.
	for _, t := range tokens {
		if t.color == colorString {
			continue
		}
		overlap := false
		for k := t.start; k < t.end && k < len(covered); k++ {
			if covered[k] {
				overlap = true
				break
			}
		}
		if !overlap {
			for k := t.start; k < t.end && k < len(covered); k++ {
				covered[k] = true
			}
			final = append(final, t)
		}
	}

	sort.Slice(final, func(i, j int) bool {
		return final[i].start < final[j].start
	})

	// Build output.
	var b strings.Builder
	pos := 0
	for _, t := range final {
		if t.start > pos {
			b.WriteString(template.HTMLEscapeString(line[pos:t.start]))
		}
		b.WriteString(`<span style="color:`)
		b.WriteString(t.color)
		b.WriteString(`">`)
		b.WriteString(template.HTMLEscapeString(line[t.start:t.end]))
		b.WriteString("</span>")
		pos = t.end
	}
	if pos < len(line) {
		b.WriteString(template.HTMLEscapeString(line[pos:]))
	}
	return b.String()
}
