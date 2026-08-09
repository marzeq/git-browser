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
