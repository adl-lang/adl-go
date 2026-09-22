package gotypes

import (
	"embed"
	"strings"

	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/gogen"
	"github.com/millergarym/gotmpl/text/template"
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

type scopedDeclParams struct {
	G          *gogen.Generator
	ModuleName string
	Name       string
	TypeParams gogen.TypeParam
	Decl       adlast.Decl
}

type aTexprParams struct {
	G          *gogen.Generator
	ModuleName string
	Name       string
	TypeName   string
	TypeParams gogen.TypeParam
}

type structParams struct {
	G                 *gogen.Generator
	Name              string
	TypeParams        gogen.TypeParam
	Fields            []fieldParams
	ContainsTypeToken bool
}

type unionParams struct {
	G          *gogen.Generator
	Name       string
	TypeParams gogen.TypeParam
	Branches   []fieldParams
}

type fieldParams struct {
	adlast.Field
	DeclName   string
	G          *gogen.Generator
	HasDefault bool
	Just       any
	IsVoid     bool
}

type typeAliasParams struct {
	G           *gogen.Generator
	Name        string
	TypeParams  gogen.TypeParam
	TypeExpr    adlast.TypeExpr
	Annotations adlast.Annotations
}
type newTypeParams typeAliasParams
