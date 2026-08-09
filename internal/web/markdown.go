package web

import (
	"html"
	"html/template"
	"strings"
)

type markdownDelimiter struct {
	marker string
	class  string
}

var markdownDelimiters = []markdownDelimiter{
	{marker: "**", class: "md-bold"},
	{marker: "__", class: "md-bold"},
	{marker: "++", class: "md-underline"},
	{marker: "*", class: "md-italic"},
	{marker: "_", class: "md-italic"},
}

func renderMarkdown(source string) template.HTML {
	lines := strings.Split(source, "\n")
	var rendered strings.Builder
	for i, line := range lines {
		level, content := markdownHeading(line)
		if level > 0 {
			rendered.WriteString(`<span class="md-heading md-h`)
			rendered.WriteByte(byte('0' + level))
			rendered.WriteString(`">`)
			renderMarkdownInline(&rendered, content)
			rendered.WriteString(`</span>`)
		} else {
			renderMarkdownInline(&rendered, line)
		}
		if i < len(lines)-1 {
			rendered.WriteByte('\n')
		}
	}

	return template.HTML(rendered.String()) // #nosec G203 -- all source text is escaped above.
}

func markdownHeading(line string) (int, string) {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0, line
	}
	return level, line[level+1:]
}

func renderMarkdownInline(rendered *strings.Builder, source string) {
	for len(source) > 0 {
		delimiter, opening, closing := nextMarkdownDelimiter(source)
		if delimiter == nil {
			rendered.WriteString(html.EscapeString(source))
			return
		}

		rendered.WriteString(html.EscapeString(source[:opening]))
		if closing < 0 {
			rendered.WriteString(html.EscapeString(source[opening:]))
			return
		}

		rendered.WriteString(`<span class="`)
		rendered.WriteString(delimiter.class)
		rendered.WriteString(`">`)
		contentStart := opening + len(delimiter.marker)
		renderMarkdownInline(rendered, source[contentStart:closing])
		rendered.WriteString(`</span>`)
		source = source[closing+len(delimiter.marker):]
	}
}

func nextMarkdownDelimiter(source string) (*markdownDelimiter, int, int) {
	opening := -1
	var found *markdownDelimiter
	for i := range markdownDelimiters {
		delimiter := &markdownDelimiters[i]
		candidate := strings.Index(source, delimiter.marker)
		if candidate >= 0 && (opening < 0 || candidate < opening) {
			found = delimiter
			opening = candidate
		}
	}
	if found == nil {
		return nil, -1, -1
	}

	contentStart := opening + len(found.marker)
	closing := strings.Index(source[contentStart:], found.marker)
	if closing < 0 {
		return found, opening, -1
	}
	return found, opening, contentStart + closing
}
