package gogen

import (
	"fmt"

	"github.com/adl-lang/adl-go/adl"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/loader"
)

// ExpandStruct is st with ExpandTypeAliases applied to each field's type.
func ExpandStruct(lr *loader.LoadResult, st adlast.Struct) adlast.Struct {
	var fields = make([]adlast.Field, len(st.Fields))
	for i, f := range st.Fields {
		fields[i] = adlast.MakeAll_Field(
			f.Name,
			f.SerializedName,
			ExpandTypeAliases(lr, f.TypeExpr),
			f.Default,
			f.Annotations,
		)
	}
	return adlast.Make_Struct(st.TypeParams, fields)
}

// ExpandTypeAliases replaces a reference to a type alias at the top of te
// with the alias's definition, its type params bound to te's arguments,
// until the top of te is no longer an alias. te's own arguments are not
// expanded: callers that descend into them expand each level as they go.
func ExpandTypeAliases(lr *loader.LoadResult, te adlast.TypeExpr) adlast.TypeExpr {
	var (
		ref  adlast.ScopedName
		decl *adlast.Decl
		ta   adlast.TypeDef
		mono adlast.TypeExpr
		ok   bool
	)
	if ref, ok = te.TypeRef.Cast_reference(); !ok {
		return te
	}
	if decl, ok = lr.Resolver(ref); !ok {
		panic(fmt.Errorf("can't resolve type alias, %v ", ref))
	}
	if ta, ok = decl.Type_.Cast_type_(); !ok {
		return te
	}
	mono, _ = adl.SubstituteTypeBindings(
		adl.CreateDecBoundTypeParams(ta.TypeParams, te.Parameters),
		ta.TypeExpr,
	)
	return ExpandTypeAliases(lr, mono)
}
