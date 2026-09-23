// Package templates holds the one template set for the whole generator.
//
// Every sub-task (gotypes, goapi, ...) renders through this set, so a
// template defined in one .tmpl file can invoke a template defined in
// another. Each .tmpl file is a collection of {{define}} blocks named after
// the Go xxxParams type it renders.
//
// One generated file is one "file" call: see gogen.WriteFile and the
// per-file body templates it dispatches to.
package templates

import (
	"embed"
	"strings"

	"github.com/millergarym/gotmpl/text/template"
)

var (
	//go:embed *.tmpl
	tmplFS embed.FS

	// Gen is the parsed template set.
	Gen = template.Must(
		template.
			New("gen", template.WithDynamicScopedVars()).
			Funcs(template.FuncMap{
				"public": public,
				"lower":  strings.ToLower,
			}).
			// SuffixLineNos("", 0, "", "").
			ParseFS(tmplFS, "*.tmpl"))
)

func public(s string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
