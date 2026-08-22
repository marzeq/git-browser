package web

import (
	"bytes"
	"html/template"
	"net/url"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type markdownLinkTransformer struct {
	linkBaseURL  *url.URL
	imageBaseURL *url.URL
}

func renderMarkdown(source, linkBaseURL, imageBaseURL string) (template.HTML, error) {
	linkBase, err := url.Parse(linkBaseURL)
	if err != nil {
		return "", err
	}
	imageBase, err := url.Parse(imageBaseURL)
	if err != nil {
		return "", err
	}
	renderer := goldmark.New(
		goldmark.WithExtensions(extension.GFM, &markdownMathExtension{}),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(
				util.Prioritized(&markdownLinkTransformer{
					linkBaseURL:  linkBase,
					imageBaseURL: imageBase,
				}, 100),
			),
		),
	)

	var rendered bytes.Buffer
	if err := renderer.Convert([]byte(source), &rendered); err != nil {
		return "", err
	}
	return template.HTML(rendered.String()), nil // #nosec G203 -- Goldmark's safe renderer produced this HTML.
}

func (t *markdownLinkTransformer) Transform(document *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.Link:
			node.Destination = resolveMarkdownDestination(t.linkBaseURL, node.Destination)
		case *ast.Image:
			node.Destination = resolveMarkdownDestination(t.imageBaseURL, node.Destination)
		}
		return ast.WalkContinue, nil
	})
}

func resolveMarkdownDestination(baseURL *url.URL, destination []byte) []byte {
	raw := string(destination)
	if raw == "" || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "//") {
		return destination
	}
	reference, err := url.Parse(raw)
	if err != nil || reference.IsAbs() {
		return destination
	}
	return []byte(baseURL.ResolveReference(reference).String())
}
