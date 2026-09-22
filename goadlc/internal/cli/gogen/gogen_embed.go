package gogen

import (
	"embed"
	"strings"

	"github.com/millergarym/gotmpl/text/template"

	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
)

func public(s string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

var (
	//go:embed templates/*
	templateFS embed.FS

	templates = template.Must(
		template.
			New("").
			Funcs(template.FuncMap{
				"public": public,
				"lower": func(s string) string {
					return strings.ToLower(s)
				},
			}).
			ParseFS(templateFS, "templates/*"))
)

type headerParams struct {
	Pkg string
}

type importsParams struct {
	// Rt      string
	Imports []goimports.ImportSpec
}
