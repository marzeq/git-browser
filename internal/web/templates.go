package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html static/*
var assets embed.FS

func parseTemplates() (*template.Template, error) {
	return template.New("").Funcs(template.FuncMap{
		"markdown": renderMarkdown,
	}).ParseFS(assets, "templates/*.html")
}
