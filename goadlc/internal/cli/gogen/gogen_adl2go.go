package gogen

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	adl "github.com/adl-lang/adl-go/adl"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
	"github.com/samber/lo"
)

type goTypeExpr struct {
	Pkg             string
	Type            string
	TypeParams      TypeParam
	UnionTypeParams TypeParam
	IsTypeParam     bool
}

func (g goTypeExpr) String() string {
	if g.IsTypeParam {
		if g.Pkg != "" {
			return g.Pkg + "." + g.Type
		}
		return g.Type
	}
	if g.Pkg != "" {
		return g.Pkg + "." + g.Type + g.TypeParams.RSide()
	}
	return g.Type + g.TypeParams.RSide()
}

func (g goTypeExpr) Complete() string {
	if g.Pkg != "" {
		return g.Pkg + "." + g.Type + g.TypeParams.RSide()
	}
	return g.Type + g.TypeParams.RSide()
}

func (g goTypeExpr) sansTypeParam() string {
	if g.Pkg != "" {
		return g.Pkg + "." + g.Type
	}
	return g.Type
}

func (in *BaseGen) GoType(
	typeExpr adlast.TypeExpr,
	anns adlast.Annotations,
) goTypeExpr {
	unionTypeParams := TypeParam{}
	gotype := in.goType(typeExpr, &unionTypeParams, anns)
	unionTypeParams.TypeConstraints = get_type_constraints(anns)
	gotype.UnionTypeParams = unionTypeParams
	return gotype
}

func (in *BaseGen) goType(
	typeExpr adlast.TypeExpr,
	unionTypeParams *TypeParam,
	anns adlast.Annotations,
) goTypeExpr {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Fprintf(os.Stderr, "ERROR in GoType %v\n%v", r, string(debug.Stack()))
			panic(r)
		}
	}()
	_type := adlast.Handle_TypeRef(
		typeExpr.TypeRef,
		func(primitive string) goTypeExpr {
			_type := in.PrimitiveMap(primitive, typeExpr.Parameters, unionTypeParams, anns)
			return _type
		},
		func(tp string) goTypeExpr {
			if !lo.ContainsBy(unionTypeParams.Params, func(it param) bool {
				return it.Name == tp
			}) {
				unionTypeParams.Params = append(unionTypeParams.Params, param{Name: tp})
			}
			return goTypeExpr{
				"",
				tp,
				TypeParam{
					Params: []param{{Name: tp, Concrete: false}},
					// type_constraints: []string{},
					TypeConstraints: get_type_constraints(anns),
				},
				TypeParam{},
				true,
			}
		},
		func(ref adlast.ScopedName) goTypeExpr {
			decl, ok := in.Resolver(ref)
			if !ok {
				panic(fmt.Errorf("cannot find decl '%v", ref))
			}
			if adl.HasAnnotation(decl.Annotations, GoCustomTypeSN) {
				return in.gotype_ref_customtype(decl, typeExpr, unionTypeParams, anns)
			}
			// go can't have typeParam on lhs in type alias, so replace with concrete type
			if typ, ok := decl.Type_.Cast_type_(); ok {
				if len(typ.TypeParams) != 0 {
					tbind := adl.CreateDecBoundTypeParams(typ.TypeParams, typeExpr.Parameters)
					monoTe, _ := adl.SubstituteTypeBindings(tbind, typ.TypeExpr)
					return in.goType(monoTe, unionTypeParams, anns)
				}
			}

			packageName := ""
			if in.ModuleName != ref.ModuleName {
				packageName = in.Imports.AddModule(ref.ModuleName, in.ModulePath, in.MidPath)
			}
			goTypeParams := lo.Map(typeExpr.Parameters, func(a adlast.TypeExpr, _ int) goTypeExpr {
				return in.goType(a, unionTypeParams, anns)
			})

			return goTypeExpr{
				Pkg:  packageName,
				Type: ref.Name,
				TypeParams: TypeParam{
					Params:          lo.Map(goTypeParams, func(a goTypeExpr, _ int) param { return param{Name: a.String(), Concrete: !a.IsTypeParam} }),
					TypeConstraints: get_type_constraints(anns),
				},
				IsTypeParam: false,
			}
		},
		nil,
	)
	return _type
}

func get_type_constraints(anns adlast.Annotations) []string {
	jb := adl.CreateJsonDecodeBinding(adl.Texpr_TypeParamConstraintList(), adl.RESOLVER)
	lst, err := adl.GetAnnotation(anns, TypeParamConstraintListSN, jb)
	if err != nil {
		panic(err)
	}
	if lst != nil {
		return *lst
	}
	return []string{}
}

func (in *BaseGen) gotype_ref_customtype(
	decl *adlast.Decl,
	typeExpr adlast.TypeExpr,
	unionTypeParams *TypeParam,
	anns adlast.Annotations,
) goTypeExpr {
	jb := adl.CreateJsonDecodeBinding(adl.Texpr_GoCustomType(), adl.RESOLVER)
	gct, err := adl.GetAnnotation(decl.Annotations, GoCustomTypeSN, jb)
	if err != nil {
		panic(fmt.Errorf("error getting go_custom_type annotation for %v. err : %w", decl.Name, err))
	}
	pkg := gct.Gotype.Import_path[strings.LastIndex(gct.Gotype.Import_path, "/")+1:]
	spec := goimports.ImportSpec{
		Path:    gct.Gotype.Import_path,
		Name:    gct.Gotype.Pkg,
		Aliased: gct.Gotype.Pkg != pkg,
	}
	in.Imports.AddSpec(spec)
	got := goTypeExpr{
		Pkg:  gct.Gotype.Pkg,
		Type: gct.Gotype.Name,
		TypeParams: TypeParam{
			Params: lo.Map(typeExpr.Parameters, func(a adlast.TypeExpr, _ int) param {
				pt := in.goType(a, unionTypeParams, anns)
				return param{pt.String(), !pt.IsTypeParam}
			}),
			// type_constraints: get_type_constraints(anns),
			TypeConstraints: []string{},
		},
		IsTypeParam: false,
	}
	return got
}

func (in *BaseGen) PrimitiveMap(
	p string,
	params []adlast.TypeExpr,
	unionTypeParams *TypeParam,
	anns adlast.Annotations,
) goTypeExpr {
	r, has := primitiveMap[p]
	if has {
		return goTypeExpr{"", r, TypeParam{}, TypeParam{}, false}
	}
	elem := in.goType(params[0], unionTypeParams, anns)
	switch p {
	case "TypeToken":
		pkg, err := in.Cli.GoImport("adlast", in.ModuleName, &in.Imports)
		if err != nil {
			panic(err)
		}
		tp := TypeParam{
			Params: []param{{Name: elem.String(), Concrete: !elem.IsTypeParam}},
		}
		return goTypeExpr{"", pkg + "ATypeExpr", tp, tp, false}
	case "Vector":
		return goTypeExpr{"", "[]" + elem.sansTypeParam(), elem.TypeParams, TypeParam{}, elem.IsTypeParam}
	case "StringMap":
		return goTypeExpr{"", "map[string]" + elem.sansTypeParam(), elem.TypeParams, TypeParam{}, elem.IsTypeParam}
	case "Nullable":
		return goTypeExpr{"", "*" + elem.sansTypeParam(), elem.TypeParams, TypeParam{}, elem.IsTypeParam}
	}
	panic(fmt.Errorf("no such primitive '%s'", p))
}

var goKeywords = map[string]string{
	"break":       "break_",
	"default":     "default_",
	"func":        "func_",
	"interface":   "interface_",
	"select":      "select_",
	"case":        "case_",
	"defer":       "defer_",
	"go":          "go_",
	"map":         "map_",
	"struct":      "struct_",
	"chan":        "chan_",
	"else":        "else_",
	"goto":        "goto_",
	"package":     "package_",
	"switch":      "switch_",
	"const":       "const_",
	"fallthrough": "fallthrough_",
	"if":          "if_",
	"range":       "range_",
	"type":        "type_",
	"continue":    "continue_",
	"for":         "for_",
	"import":      "import_",
	"return":      "return_",
	"var":         "var_",
}

func (*BaseGen) GoEscape(n string) string {
	if g, h := goKeywords[n]; h {
		return g
	}
	return n
}

var (
	primitiveMap = map[string]string{
		"Int8":       "int8",
		"Int16":      "int16",
		"Int32":      "int32",
		"Int64":      "int64",
		"Word8":      "uint8",
		"Word16":     "uint16",
		"Word32":     "uint32",
		"Word64":     "uint64",
		"Bool":       "bool",
		"Float":      "float64",
		"Double":     "float64",
		"String":     "string",
		"ByteVector": "[]byte",
		"Void":       "struct{}",
		"Json":       "any",
		// "TypeToken":  "struct{}",
		// "`Vector<T>`":    0,
		// "`StringMap<T>`": 0,
		// "`Nullable<T>`":  0,
	}
)
