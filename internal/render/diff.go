package render

import (
	"html/template"
	"strings"
)

// RenderDiffHTML computes a line-level diff between old and new strings
// and returns HTML with red/green coloring using inline styles.
func RenderDiffHTML(old, new string) template.HTML {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	// For very large inputs, fall back to simple before/after.
	if len(oldLines)*len(newLines) > 100000 {
		return simpleDiffHTML(oldLines, newLines)
	}

	diff := lcs(oldLines, newLines)
	return renderLines(diff)
}

type diffLine struct {
	typ  byte // ' ' context, '-' remove, '+' add
	text string
}

// lcs computes a diff using the longest common subsequence algorithm.
func lcs(oldLines, newLines []string) []diffLine {
	m, n := len(oldLines), len(newLines)

	// Build LCS table.
	table := make([][]int, m+1)
	for i := range table {
		table[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else if table[i+1][j] >= table[i][j+1] {
				table[i][j] = table[i+1][j]
			} else {
				table[i][j] = table[i][j+1]
			}
		}
	}

	// Walk table to produce diff lines.
	var result []diffLine
	i, j := 0, 0
	for i < m || j < n {
		if i < m && j < n && oldLines[i] == newLines[j] {
			result = append(result, diffLine{' ', oldLines[i]})
			i++
			j++
		} else if j < n && (i >= m || table[i][j+1] >= table[i+1][j]) {
			result = append(result, diffLine{'+', newLines[j]})
			j++
		} else if i < m {
			result = append(result, diffLine{'-', oldLines[i]})
			i++
		}
	}
	return result
}

func simpleDiffHTML(oldLines, newLines []string) template.HTML {
	var result []diffLine
	for _, l := range oldLines {
		result = append(result, diffLine{'-', l})
	}
	for _, l := range newLines {
		result = append(result, diffLine{'+', l})
	}
	return renderLines(result)
}

func renderLines(lines []diffLine) template.HTML {
	var b strings.Builder
	for _, dl := range lines {
		escaped := template.HTMLEscapeString(dl.text)
		switch dl.typ {
		case '-':
			b.WriteString(`<div style="color:#f87171;background:rgba(127,29,29,0.3)">- `)
			b.WriteString(escaped)
			b.WriteString("</div>")
		case '+':
			b.WriteString(`<div style="color:#4ade80;background:rgba(20,83,45,0.3)">+ `)
			b.WriteString(escaped)
			b.WriteString("</div>")
		default:
			b.WriteString(`<div style="color:#9ca3af">  `)
			b.WriteString(escaped)
			b.WriteString("</div>")
		}
	}
	return template.HTML(b.String())
}
