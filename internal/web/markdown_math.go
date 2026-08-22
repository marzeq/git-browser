package web

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var kindMarkdownMath = ast.NewNodeKind("MarkdownMath")

type markdownMathNode struct {
	ast.BaseInline
	equation []byte
	display  bool
}

func (n *markdownMathNode) Inline() {}

func (n *markdownMathNode) IsBlank(_ []byte) bool {
	return len(bytes.TrimSpace(n.equation)) == 0
}

func (n *markdownMathNode) Kind() ast.NodeKind {
	return kindMarkdownMath
}

func (n *markdownMathNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type markdownMathParser struct{}

func (p *markdownMathParser) Trigger() []byte {
	return []byte{'$'}
}

func (p *markdownMathParser) Parse(_ ast.Node, reader text.Reader, _ parser.Context) ast.Node {
	line, _ := reader.PeekLine()
	if len(line) < 2 || line[0] != '$' {
		return nil
	}
	if line[1] == '$' {
		return parseDisplayMath(reader)
	}
	return parseInlineMath(reader, line)
}

func parseInlineMath(reader text.Reader, line []byte) ast.Node {
	// These are Pandoc-style delimiter rules. Requiring a non-space before the
	// closer keeps ordinary currency such as "$10 and $20" as text.
	if len(line) < 3 || isMathSpace(line[1]) {
		return nil
	}
	for i := 1; i < len(line); i++ {
		if line[i] != '$' || isEscapedMathDelimiter(line, i) {
			continue
		}
		if i+1 < len(line) && line[i+1] == '$' || isMathSpace(line[i-1]) ||
			i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9' {
			return nil
		}

		equation := append([]byte(nil), line[1:i]...)
		reader.Advance(i + 1)
		return &markdownMathNode{equation: equation}
	}
	return nil
}

func parseDisplayMath(reader text.Reader) ast.Node {
	startLine, startPosition := reader.Position()
	var equation bytes.Buffer

	line, _ := reader.PeekLine()
	line = line[2:]
	for {
		for i := 0; i+1 < len(line); i++ {
			if line[i] == '$' && line[i+1] == '$' && !isEscapedMathDelimiter(line, i) {
				equation.Write(line[:i])
				if len(bytes.TrimSpace(equation.Bytes())) == 0 {
					reader.SetPosition(startLine, startPosition)
					return nil
				}
				reader.Advance(i + 2)
				return &markdownMathNode{
					equation: append([]byte(nil), equation.Bytes()...),
					display:  true,
				}
			}
		}

		equation.Write(line)
		reader.AdvanceLine()
		line, _ = reader.PeekLine()
		if line == nil {
			reader.SetPosition(startLine, startPosition)
			return nil
		}
	}
}

func isEscapedMathDelimiter(line []byte, index int) bool {
	backslashes := 0
	for index > 0 && line[index-1] == '\\' {
		backslashes++
		index--
	}
	return backslashes%2 == 1
}

func isMathSpace(value byte) bool {
	return util.IsSpace(value)
}

type markdownMathRenderer struct{}

func (r *markdownMathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMarkdownMath, r.renderMath)
}

func (r *markdownMathRenderer) renderMath(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	math := node.(*markdownMathNode)
	class := "math-inline"
	if math.display {
		class = "math-display"
	}
	_, _ = w.WriteString(`<span class="` + class + `">`)
	_, _ = w.Write(util.EscapeHTML(math.equation))
	_, _ = w.WriteString("</span>")
	return ast.WalkContinue, nil
}

type markdownMathExtension struct{}

func (e *markdownMathExtension) Extend(markdown goldmark.Markdown) {
	markdown.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(&markdownMathParser{}, 50),
	))
	markdown.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(&markdownMathRenderer{}, 50),
	))
}
