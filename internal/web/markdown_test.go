package web

import (
	"strings"
	"testing"
)

func TestRenderMarkdownSupportsGFM(t *testing.T) {
	source := `# Heading

Text with **bold**, _italic_, [a link](https://example.com), and ![an image](image.png).

Inline code: ` + "`**not bold**`" + `

` + "```go\n**also not bold**\n```"
	renderedHTML, err := renderMarkdown(source, "/demo/blob/main/docs/", "/demo/raw/blob/main/docs/")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(renderedHTML)

	for _, want := range []string{
		`<h1 id="heading">Heading</h1>`,
		`<strong>bold</strong>`,
		`<em>italic</em>`,
		`<a href="https://example.com">a link</a>`,
		`<img src="/demo/raw/blob/main/docs/image.png" alt="an image">`,
		`<code>**not bold**</code>`,
		"<pre><code class=\"language-go\">**also not bold**\n</code></pre>",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered Markdown did not contain %q\noutput:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, `<strong>not bold</strong>`) || strings.Contains(rendered, `<strong>also not bold</strong>`) {
		t.Fatalf("Markdown inside code was parsed:\n%s", rendered)
	}
}

func TestRenderMarkdownUsesSafeRenderer(t *testing.T) {
	source := `<script>alert("x")</script>

[unsafe](javascript:alert('x'))

![unsafe](javascript:alert('x'))`
	renderedHTML, err := renderMarkdown(source, "/demo/blob/main/", "/demo/raw/blob/main/")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(renderedHTML)

	if strings.Contains(rendered, "<script>") || strings.Contains(rendered, `href="javascript:`) || strings.Contains(rendered, `src="javascript:`) {
		t.Fatalf("rendered Markdown contained unsafe HTML:\n%s", rendered)
	}
}

func TestRenderMarkdownResolvesRelativeLinksAgainstPreviewDirectory(t *testing.T) {
	renderedHTML, err := renderMarkdown(
		`[guide](../guide.md#usage) ![logo](images/logo.png)`,
		"/demo/blob/main/docs/reference/",
		"/demo/raw/blob/main/docs/reference/",
	)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(renderedHTML)
	for _, want := range []string{
		`href="/demo/blob/main/docs/guide.md#usage"`,
		`src="/demo/raw/blob/main/docs/reference/images/logo.png"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered Markdown did not contain %q\noutput:\n%s", want, rendered)
		}
	}
}

func TestRenderMarkdownSupportsInlineAndDisplayMath(t *testing.T) {
	source := `Euler wrote $e^{i\pi} + 1 = 0$.

$$
\int_0^1 x^2 \, dx = \frac{1}{3}
$$`
	renderedHTML, err := renderMarkdown(source, "/demo/blob/main/", "/demo/raw/blob/main/")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(renderedHTML)
	for _, want := range []string{
		`<span class="math-inline">e^{i\pi} + 1 = 0</span>`,
		"<span class=\"math-display\">\n\\int_0^1 x^2 \\, dx = \\frac{1}{3}\n</span>",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered Markdown did not contain %q\noutput:\n%s", want, rendered)
		}
	}
}

func TestRenderMarkdownMathDoesNotConsumeCodeOrCurrency(t *testing.T) {
	source := "A ticket costs $10 and a meal costs $20. `code $x$`\n\n```tex\n$y$\n```"
	renderedHTML, err := renderMarkdown(source, "/demo/blob/main/", "/demo/raw/blob/main/")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(renderedHTML)
	if strings.Contains(rendered, `class="math-inline"`) || strings.Contains(rendered, `class="math-display"`) {
		t.Fatalf("currency or code was parsed as math:\n%s", rendered)
	}
	for _, want := range []string{"$10", "$20", "<code>code $x$</code>", "<pre><code class=\"language-tex\">$y$"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered Markdown did not contain %q\noutput:\n%s", want, rendered)
		}
	}
}

func TestRenderMarkdownLeavesUnclosedMathAsText(t *testing.T) {
	renderedHTML, err := renderMarkdown(`An unfinished $x + 1 expression.`, "/demo/blob/main/", "/demo/raw/blob/main/")
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(renderedHTML)
	if strings.Contains(rendered, `class="math-inline"`) || !strings.Contains(rendered, `$x + 1`) {
		t.Fatalf("unclosed math was not preserved as text:\n%s", rendered)
	}
}
