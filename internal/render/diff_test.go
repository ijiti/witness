package render

import (
	"strings"
	"testing"
)

func TestRenderDiffHTML_Identical(t *testing.T) {
	got := RenderDiffHTML("line1\nline2", "line1\nline2")
	s := string(got)

	// All lines should be context (gray), no add/remove prefixes.
	if !strings.Contains(s, "color:#9ca3af") {
		t.Errorf("RenderDiffHTML Identical: expected context color #9ca3af\ngot: %s", s)
	}
	if strings.Contains(s, "color:#f87171") {
		t.Errorf("RenderDiffHTML Identical: unexpected remove color #f87171\ngot: %s", s)
	}
	if strings.Contains(s, "color:#4ade80") {
		t.Errorf("RenderDiffHTML Identical: unexpected add color #4ade80\ngot: %s", s)
	}
}

func TestRenderDiffHTML_CompletelyDifferent(t *testing.T) {
	got := RenderDiffHTML("aaa", "bbb")
	s := string(got)

	if !strings.Contains(s, `color:#f87171`) {
		t.Errorf("RenderDiffHTML CompletelyDifferent: expected red remove div\ngot: %s", s)
	}
	if !strings.Contains(s, `color:#4ade80`) {
		t.Errorf("RenderDiffHTML CompletelyDifferent: expected green add div\ngot: %s", s)
	}
	if !strings.Contains(s, "- aaa") {
		t.Errorf("RenderDiffHTML CompletelyDifferent: expected '- aaa'\ngot: %s", s)
	}
	if !strings.Contains(s, "+ bbb") {
		t.Errorf("RenderDiffHTML CompletelyDifferent: expected '+ bbb'\ngot: %s", s)
	}
}

func TestRenderDiffHTML_SingleAdd(t *testing.T) {
	got := RenderDiffHTML("line1\nline2", "line1\nnew\nline2")
	s := string(got)

	if !strings.Contains(s, "+ new") {
		t.Errorf("RenderDiffHTML SingleAdd: expected '+ new'\ngot: %s", s)
	}
	// line1 and line2 should be context.
	if !strings.Contains(s, "color:#9ca3af") {
		t.Errorf("RenderDiffHTML SingleAdd: expected context color for unchanged lines\ngot: %s", s)
	}
	// No removes.
	if strings.Contains(s, "color:#f87171") {
		t.Errorf("RenderDiffHTML SingleAdd: unexpected remove color\ngot: %s", s)
	}
}

func TestRenderDiffHTML_SingleRemove(t *testing.T) {
	got := RenderDiffHTML("line1\nold\nline2", "line1\nline2")
	s := string(got)

	if !strings.Contains(s, "- old") {
		t.Errorf("RenderDiffHTML SingleRemove: expected '- old'\ngot: %s", s)
	}
	if !strings.Contains(s, "color:#f87171") {
		t.Errorf("RenderDiffHTML SingleRemove: expected red remove color\ngot: %s", s)
	}
	// No adds.
	if strings.Contains(s, "color:#4ade80") {
		t.Errorf("RenderDiffHTML SingleRemove: unexpected add color\ngot: %s", s)
	}
}

func TestRenderDiffHTML_LargeInputFallback(t *testing.T) {
	// 400 * 400 = 160000 > 100000 triggers simpleDiffHTML.
	var oldSB, newSB strings.Builder
	for i := 0; i < 400; i++ {
		if i > 0 {
			oldSB.WriteByte('\n')
			newSB.WriteByte('\n')
		}
		oldSB.WriteString("old line")
		newSB.WriteString("new line")
	}

	got := RenderDiffHTML(oldSB.String(), newSB.String())
	s := string(got)

	// simpleDiffHTML renders all old lines as removes first, then all new as adds.
	// All old lines should be removed (red).
	if !strings.Contains(s, "color:#f87171") {
		t.Errorf("RenderDiffHTML LargeInputFallback: expected red remove divs\ngot (truncated): %.200s", s)
	}
	// All new lines should be added (green).
	if !strings.Contains(s, "color:#4ade80") {
		t.Errorf("RenderDiffHTML LargeInputFallback: expected green add divs\ngot (truncated): %.200s", s)
	}
	// No context lines — simple diff has no common lines here.
	// Count that we have both remove and add markers.
	removeCount := strings.Count(s, "- old line")
	if removeCount != 400 {
		t.Errorf("RenderDiffHTML LargeInputFallback: expected 400 remove lines, got %d", removeCount)
	}
	addCount := strings.Count(s, "+ new line")
	if addCount != 400 {
		t.Errorf("RenderDiffHTML LargeInputFallback: expected 400 add lines, got %d", addCount)
	}
}

func TestRenderDiffHTML_BothEmpty(t *testing.T) {
	got := RenderDiffHTML("", "")
	s := string(got)

	// Both empty strings split to [""] vs [""] — they match, so one context div.
	if !strings.Contains(s, "color:#9ca3af") {
		t.Errorf("RenderDiffHTML BothEmpty: expected context div\ngot: %s", s)
	}
	if strings.Contains(s, "color:#f87171") {
		t.Errorf("RenderDiffHTML BothEmpty: unexpected remove div\ngot: %s", s)
	}
	if strings.Contains(s, "color:#4ade80") {
		t.Errorf("RenderDiffHTML BothEmpty: unexpected add div\ngot: %s", s)
	}
}

func TestRenderDiffHTML_EmptyOld(t *testing.T) {
	got := RenderDiffHTML("", "new content")
	s := string(got)

	// LCS of [""] vs ["new content"]: they differ, so "new content" is added.
	if !strings.Contains(s, `color:#4ade80`) {
		t.Errorf("RenderDiffHTML EmptyOld: expected green add div\ngot: %s", s)
	}
	if !strings.Contains(s, "+ new content") {
		t.Errorf("RenderDiffHTML EmptyOld: expected '+ new content'\ngot: %s", s)
	}
}

func TestRenderDiffHTML_EmptyNew(t *testing.T) {
	got := RenderDiffHTML("old content", "")
	s := string(got)

	// LCS of ["old content"] vs [""]: they differ, so "old content" is removed.
	if !strings.Contains(s, `color:#f87171`) {
		t.Errorf("RenderDiffHTML EmptyNew: expected red remove div\ngot: %s", s)
	}
	if !strings.Contains(s, "- old content") {
		t.Errorf("RenderDiffHTML EmptyNew: expected '- old content'\ngot: %s", s)
	}
}

func TestRenderDiffHTML_HTMLEscaping(t *testing.T) {
	old := `<script>alert("xss")</script>`
	got := RenderDiffHTML(old, "safe")
	s := string(got)

	// The XSS payload must be HTML-escaped.
	if !strings.Contains(s, "&lt;script&gt;") {
		t.Errorf("RenderDiffHTML HTMLEscaping: expected &lt;script&gt; in output\ngot: %s", s)
	}
	// The raw tag must not appear.
	if strings.Contains(s, "<script>") {
		t.Errorf("RenderDiffHTML HTMLEscaping: raw <script> found in output (not escaped)\ngot: %s", s)
	}
}
