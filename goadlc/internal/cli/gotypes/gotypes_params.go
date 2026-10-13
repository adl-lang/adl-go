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

// declParams renders one ADL decl. The template dispatches on the decl's
// DeclType branch via tmpl_by_type, so the struct/union/alias/newtype split
// lives in the templates rather than in a Handle_DeclType ladder here.
type declParams struct {
	G          *gogen.Generator
	Decl       adlast.Decl
	Name       string
	TypeParams gogen.TypeParam
}

func (p declParams) Annotations() adlast.Annotations {
	return p.Decl.Annotations
}

// Fields are the struct branch's fields, decorated for the templates.
func (p declParams) Fields() []gogen.Field {
	struct_, _ := p.Decl.Type_.Cast_struct_()
	return gogen.WrapFields(struct_.Fields)
}

// Branches are the union branch's fields, decorated for the templates.
func (p declParams) Branches() []gogen.Field {
	union_, _ := p.Decl.Type_.Cast_union_()
	return gogen.WrapFields(union_.Fields)
}

// ContainsTypeToken reports whether any field of the struct is a TypeToken.
// Structs that contain one get no Make_ funcs generated.
func (p declParams) ContainsTypeToken() bool {
	for _, f := range p.Fields() {
		if f.IsTypeToken() {
			return true
		}
	}
	return false
}
