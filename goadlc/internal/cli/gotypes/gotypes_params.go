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
	G          *gogen.Generator
	Name       string
	TypeParams gogen.TypeParam
	Fields     []fieldParams
}

// ContainsTypeToken reports whether any field of the struct is a TypeToken.
// Structs that contain one get no Make_ funcs generated.
func (p structParams) ContainsTypeToken() bool {
	for _, f := range p.Fields {
		if f.IsTypeToken() {
			return true
		}
	}
	return false
}

type unionParams struct {
	G          *gogen.Generator
	Name       string
	TypeParams gogen.TypeParam
	Branches   []fieldParams
}

type fieldParams struct {
	gogen.Field
	DeclName string
	G        *gogen.Generator
}

type typeAliasParams struct {
	G           *gogen.Generator
	Name        string
	TypeParams  gogen.TypeParam
	TypeExpr    adlast.TypeExpr
	Annotations adlast.Annotations
}
type newTypeParams typeAliasParams
