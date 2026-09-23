package gotypes

import (
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/gogen"
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
