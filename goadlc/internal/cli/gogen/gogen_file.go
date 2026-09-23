package gogen

import (
	"bytes"

	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/templates"
)

// FileParams is the data for the "file" template - the one entry point for a
// generated file.
//
// A file's import block sits above its body, but which imports the file needs
// is only discovered while the body renders: the templates call GoType,
// GoValue and GoImport, and each registers into Imports as a side effect.
// Imports therefore renders the body first and caches it, so by the time the
// import block is written the set is complete and {{.Body}} is just a lookup.
type FileParams struct {
	// Pkg is the package clause of the generated file.
	Pkg string
	// G owns the Imports that rendering the body populates.
	G *Generator
	// BodyTmpl renders the whole body from BodyData.
	BodyTmpl string
	BodyData any
	// Special are imports the caller knows about up front, appended after
	// the ones discovered by rendering.
	Special []goimports.ImportSpec

	body *string
}

// Body is the rendered file body. It renders once and caches, so that
// Imports can force it without paying for it twice.
func (p *FileParams) Body() string {
	if p.body == nil {
		var (
			buf bytes.Buffer
			err error
		)
		if err = templates.Gen.ExecuteTemplate(&buf, p.BodyTmpl, p.BodyData); err != nil {
			renderPanic(p.BodyData, err)
		}
		rendered := buf.String()
		p.body = &rendered
	}
	return *p.body
}

// Imports are the specs the file actually uses. It forces Body first, since
// rendering is what marks a spec used.
func (p *FileParams) Imports() []goimports.ImportSpec {
	p.Body()
	used := []goimports.ImportSpec{}
	for _, spec := range p.G.Imports.Specs {
		if p.G.Imports.Used[spec.Path] {
			used = append(used, spec)
		}
	}
	return append(used, p.Special...)
}
