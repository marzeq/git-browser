package web

import (
	"strings"
	"testing"
)

func TestRenderMarkdownUsesSpanFormatting(t *testing.T) {
	source := "# Heading <unsafe>\nText with **bold**, _italic_, and ++underlined++."
	rendered := string(renderMarkdown(source))

	for _, want := range []string{
		`<span class="md-heading md-h1">Heading &lt;unsafe&gt;</span>`,
		`<span class="md-bold">bold</span>`,
		`<span class="md-italic">italic</span>`,
		`<span class="md-underline">underlined</span>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered Markdown did not contain %q\noutput:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "<h1") || strings.Contains(rendered, "<strong") || strings.Contains(rendered, "<em") {
		t.Fatalf("rendered Markdown used a non-span formatting element:\n%s", rendered)
	}
}

func TestRenderMarkdownEscapesHTMLAndLeavesUnsupportedMarkupAlone(t *testing.T) {
	source := `<script>alert("x")</script> [link](https://example.com) **unfinished`
	rendered := string(renderMarkdown(source))

	if strings.Contains(rendered, "<script>") {
		t.Fatalf("rendered Markdown contained source HTML:\n%s", rendered)
	}
	for _, want := range []string{
		`&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;`,
		`[link](https://example.com)`,
		`**unfinished`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered Markdown did not preserve %q\noutput:\n%s", want, rendered)
		}
	}
}

func TestMarkdownHeadingSupportsSixLevels(t *testing.T) {
	for level := 1; level <= 6; level++ {
		source := strings.Repeat("#", level) + " title"
		want := `class="md-heading md-h` + string(rune('0'+level)) + `"`
		if rendered := string(renderMarkdown(source)); !strings.Contains(rendered, want) {
			t.Fatalf("level %d: output did not contain %q: %s", level, want, rendered)
		}
	}
}
