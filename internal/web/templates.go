package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html static/*
var assets embed.FS

func parseTemplates() (*template.Template, error) {
	return template.ParseFS(assets, "templates/*.html")
}
