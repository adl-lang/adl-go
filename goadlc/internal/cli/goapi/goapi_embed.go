package goapi

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
				// "goEscape": goEscape,
			}).
			ParseFS(templateFS, "templates/*"))
)

type serviceParams struct {
	G          *gogen.Generator
	Name       string
	TypeParams gogen.TypeParam
	IsCap      bool
}

type registerParams struct {
	G           *gogen.Generator
	CapModule   string
	Name        string
	TypeParams  gogen.TypeParam
	IsCap       bool
	Annotations adlast.Annotations
	V           *adlast.TypeExpr
	CapApis     []tkid
}

type postParams struct {
	G           *gogen.Generator
	Name        string
	Annotations adlast.Annotations
	Req         adlast.TypeExpr
	Resp        adlast.TypeExpr
	IsCap       bool
}

type getParams struct {
	G           *gogen.Generator
	Name        string
	Annotations adlast.Annotations
	Resp        adlast.TypeExpr
	IsCap       bool
}

type regpostParams struct {
	G      *gogen.Generator
	Module string
	Name   string
	IsCap  bool
}
type reggetParams regpostParams

type regcapapiParams struct {
	G          *gogen.Generator
	StructName string
	Module     string
	Name       string
	Kids       []tkid
}

type regapiParams struct {
	G          *gogen.Generator
	StructName string
	Module     string
	Name       string
	// Kids       []tkid
}

type tkid struct {
	Name  string
	Field *adlast.Field
}

type getcapapiParams struct {
	G           *gogen.Generator
	Name        string
	StructName  string
	Annotations adlast.Annotations
	C           adlast.TypeExpr
	S           adlast.TypeExpr
	Params      []adlast.TypeExpr
}

type getapiParams struct {
	G           *gogen.Generator
	Name        string
	StructName  string
	Annotations adlast.Annotations
	Params      []adlast.TypeExpr
}
