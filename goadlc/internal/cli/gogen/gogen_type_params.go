package gogen

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	goadl "github.com/adl-lang/adl-go/adl"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/samber/lo"
)

func TypeParamsFromDecl(decl adlast.Decl) TypeParam {
	tp := adlast.Handle_DeclType[TypeParam](
		decl.Type_,
		func(struct_ adlast.Struct) TypeParam {
			return TypeParam{
				Params:          lo.Map(struct_.TypeParams, func(name string, _ int) param { return param{Name: name, Concrete: false} }),
				TypeConstraints: []string{},
				Added:           false,
			}
		},
		func(union_ adlast.Union) TypeParam {
			return TypeParam{
				Params:          lo.Map(union_.TypeParams, func(name string, _ int) param { return param{Name: name, Concrete: false} }),
				TypeConstraints: []string{},
				Added:           false,
			}
		},
		func(type_ adlast.TypeDef) TypeParam {
			return TypeParam{
				Params:          lo.Map(type_.TypeParams, func(name string, _ int) param { return param{Name: name, Concrete: false} }),
				TypeConstraints: []string{},
				Added:           false,
			}
		},
		func(newtype_ adlast.NewType) TypeParam {
			return TypeParam{
				Params:          lo.Map(newtype_.TypeParams, func(name string, _ int) param { return param{Name: name, Concrete: false} }),
				TypeConstraints: []string{},
				Added:           false,
			}
		},
		nil,
	)
	jb := goadl.CreateJsonDecodeBinding(goadl.Texpr_TypeParamConstraintList(), goadl.RESOLVER)
	lst, err := goadl.GetAnnotation(decl.Annotations, TypeParamConstraintListSN, jb)
	if err != nil {
		panic(err)
	}
	if lst != nil {
		tp.TypeConstraints = *lst
	}
	return tp
}

var TypeParamConstraintListSN = adlast.Make_ScopedName("adlc.config.go_", "TypeParamConstraintList")

type param struct {
	Name     string
	Concrete bool
}

type TypeParam struct {
	Params          []param
	TypeConstraints []string
	Added           bool
}

func (tp TypeParam) MarshalJSON() ([]byte, error) {
	return json.Marshal(tp.Params)
}

func (tp TypeParam) AddParams(newps ...string) TypeParam {
	ntp := tp
	for _, np := range newps {
		ntp = ntp.AddParam(np)
	}
	return ntp
}

func (tp TypeParam) AddParam(newp string) TypeParam {
	psMap := make(map[string]bool)
	tp0 := make([]param, len(tp.Params)+1)
	for i, p := range tp.Params {
		tp0[i] = p
		psMap[p.Name] = true
	}

	tp0[len(tp.Params)] = param{Name: newp, Concrete: false}
	if psMap[tp0[len(tp.Params)].Name] {
		n := uint64(1)
		for {
			n++
			tp0[len(tp.Params)] = param{Name: newp + strconv.FormatUint(n, 10), Concrete: false}
			if !psMap[tp0[len(tp.Params)].Name] {
				break
			}
		}
	}
	return TypeParam{
		Params:          tp0,
		TypeConstraints: tp.TypeConstraints,
		// tp.isTypeParam,
		Added: true,
	}
}
func (tp TypeParam) Has() bool {
	return (!tp.Added && len(tp.Params) != 0) || len(tp.Params) != 1
}
func (tp TypeParam) Last() string {
	if len(tp.Params) == 0 {
		return ""
	}
	return tp.Params[len(tp.Params)-1].Name
}

func (tp TypeParam) LSide() string {
	if len(tp.Params) == 0 {
		return ""
	}
	names := lo.Map[param, string](tp.Params, func(a param, i int) string { return a.Name })
	return "[" + strings.Join(lo.Map(names, func(e string, i int) string {
		if i+1 <= len(tp.TypeConstraints) {
			return e + " " + tp.TypeConstraints[i]
		}
		return e + " any"
	}), ", ") + "]"
}
func (tp TypeParam) RSide() string {
	if len(tp.Params) == 0 {
		return ""
	}
	names := lo.Map[param, string](tp.Params, func(a param, i int) string { return a.Name })
	return "[" + strings.Join(names, ",") + "]"
}
func (tp TypeParam) TexprArgs() string {
	if len(tp.Params) == 0 {
		return ""
	}
	names := lo.Map[param, string](tp.Params, func(a param, i int) string { return a.Name })
	return strings.Join(lo.Map(names, func(e string, _ int) string { return fmt.Sprintf("%s adlast.ATypeExpr[%s]", strings.ToLower(e), e) }), ", ")
}
func (tp TypeParam) TexprValues() string {
	if len(tp.Params) == 0 {
		return ""
	}
	names := lo.Map[param, string](tp.Params, func(a param, i int) string { return a.Name })
	return strings.Join(lo.Map(names, func(e string, _ int) string { return fmt.Sprintf("%s.Value", strings.ToLower(e)) }), ", ")
}

func (tp TypeParam) TpArgs() string {
	if len(tp.Params) == 0 {
		return ""
	}
	return "[any" + strings.Repeat(",any", len(tp.Params)-1) + "]"
}
