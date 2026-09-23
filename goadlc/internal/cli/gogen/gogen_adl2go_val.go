package gogen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/adl-lang/adl-go/adl"
	"github.com/adl-lang/adl-go/adl/customtypes"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/adl/sys/types"

	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"

	"github.com/samber/lo"
)

func (*Generator) JsonEncode(val any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err := enc.Encode(val)
	if err != nil {
		panic(err)
	}
	return string(bytes.Trim(buf.Bytes(), "\n"))
}

func (bg *Generator) GoDeclValue(val adlast.Decl) string {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Fprintf(os.Stderr, "ERROR in GoDeclValue %v\n%v", r, string(debug.Stack()))
			panic(r)
		}
	}()
	var buf bytes.Buffer
	enc := adl.CreateJsonEncodeBinding(adl.Texpr_Decl(), adl.RESOLVER)
	err := enc.Encode(&buf, val)
	if err != nil {
		fmt.Fprintf(os.Stderr, "!!!! encode error %v\n", err)
		panic(err)
	}
	var m any
	dec := json.NewDecoder(&buf)
	// dec.UseNumber()
	err = dec.Decode(&m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "!!!! decode error %v\n", err)
		panic(err)
	}
	gvg := goval_gen{
		bg,
		[]string{},
		true,
	}
	// TODO make it so we GoValue can take both an any and a decl
	// or make it so the encoder can encode to an any
	return gvg.goValue(val.Annotations, adl.Texpr_Decl().Value, m)
}

func (bg *Generator) GoTexprValue(val adlast.TypeExpr, anns customtypes.MapMap[adlast.ScopedName, any]) string {
	// defer func() {
	// 	r := recover()
	// 	if r != nil {
	// 		fmt.Fprintf(os.Stderr, "ERROR in GoTexprValue %v\n%v", r, string(debug.Stack()))
	// 		panic(r)
	// 	}
	// }()
	var buf bytes.Buffer
	enc := adl.CreateJsonEncodeBinding(adl.Texpr_TypeExpr(), adl.RESOLVER)
	err := enc.Encode(&buf, val)
	if err != nil {
		fmt.Fprintf(os.Stderr, "!!!! encode error %v\n", err)
		panic(err)
	}
	var m any
	dec := json.NewDecoder(&buf)
	// dec.UseNumber()
	err = dec.Decode(&m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "!!!! decode error %v\n", err)
		panic(err)
	}

	// bg.genAdlAst = true
	// TODO make it so we GoValue can take both an any and a decl
	// or make it so the encoder can encode to an any
	return bg.GoValue(anns, adl.Texpr_TypeExpr().Value, m)
}

type goval_gen struct {
	*Generator
	path      []string
	genAdlAst bool
}

func (bg *Generator) GoValue(
	anns adlast.Annotations,
	te adlast.TypeExpr,
	val any,
) string {
	gvg := goval_gen{
		bg,
		[]string{},
		false,
	}
	defer func() {
		r := recover()
		if r != nil {
			fmt.Fprintf(os.Stderr, "ERROR in path %v GoValue %v\n%v", gvg.path, r, string(debug.Stack()))
			panic(r)
		}
	}()
	return gvg.goValue(anns, te, val)
}

func (bg *goval_gen) goValue(
	anns adlast.Annotations,
	te adlast.TypeExpr,
	val any,
) string {
	return adlast.Handle_TypeRef[string](
		te.TypeRef,
		func(primitive string) string {
			return bg.goValuePrimitive(anns, te, primitive, val)
		},
		func(typeParam string) string {
			// valid if the primitive is a type token
			return typeParam
			// panic("unbound typeParam " + typeParam)
		},
		func(ref adlast.ScopedName) string {
			gt := bg.GoType(te, anns)
			decl, ok := bg.Resolver(ref)
			if !ok {
				panic(fmt.Errorf("cannot resolve %v", ref))
			}
			tbind := adl.CreateDecBoundTypeParams(adl.TypeParamsFromDecl(*decl), te.Parameters)
			if adl.HasAnnotation(decl.Annotations, GoCustomTypeSN) {
				monoTe, _ := adl.SubstituteTypeBindings(tbind, te)
				return bg.goCustomType(decl, monoTe, gt, val)
			}
			bg.path = append(bg.path, decl.Name)
			return adlast.Handle_DeclType(
				decl.Type_,
				func(struct_ adlast.Struct) string {
					return bg.goStruct(struct_, tbind, gt, val)
				},
				func(union_ adlast.Union) string {
					return bg.goUnion(union_, decl.Name, tbind, gt, val)
				},
				func(type_ adlast.TypeDef) string {
					monoTe, _ := adl.SubstituteTypeBindings(tbind, type_.TypeExpr)
					return bg.goValue(decl.Annotations, monoTe, val)
				},
				func(newtype_ adlast.NewType) string {
					monoTe, _ := adl.SubstituteTypeBindings(tbind, newtype_.TypeExpr)
					return ctorParams{
						Ctor: gt.String(),
						Args: []string{bg.goValue(decl.Annotations, monoTe, val)},
					}.StringRep()
				},
				nil,
			)
		},
		nil,
	)
}

func (bg *Generator) goCustomType(
	decl *adlast.Decl,
	monoTe adlast.TypeExpr,
	gt goTypeExpr,
	val any,
) string {
	jb := adl.CreateJsonDecodeBinding(adl.Texpr_GoCustomType(), adl.RESOLVER)
	gct, err := adl.GetAnnotation(decl.Annotations, GoCustomTypeSN, jb)
	if err != nil {
		panic(err)
	}
	{
		pkg := gct.Gotype.Import_path[strings.LastIndex(gct.Gotype.Import_path, "/")+1:]
		spec := goimports.ImportSpec{
			Path:    gct.Gotype.Import_path,
			Name:    gct.Gotype.Pkg,
			Aliased: gct.Gotype.Pkg != pkg,
		}
		bg.Imports.AddSpec(spec)
	}

	typeExprStrs := lo.Map[adlast.TypeExpr, string](monoTe.Parameters, func(a adlast.TypeExpr, _ int) string {
		return bg.strRep(a)
	})

	helperName := gct.Helpers.Name
	if gct.Helpers.Ref != nil {
		helperName = gct.Helpers.Ref.Pkg + "." + gct.Helpers.Name
		pkg := gct.Helpers.Ref.Import_path[strings.LastIndex(gct.Helpers.Ref.Import_path, "/")+1:]
		spec := goimports.ImportSpec{
			Path:    gct.Helpers.Ref.Import_path,
			Name:    gct.Helpers.Ref.Pkg,
			Aliased: gct.Helpers.Ref.Pkg != pkg,
		}
		bg.Imports.AddSpec(spec)
	}
	return RenderString("custTypeConstructionParams", custTypeConstructionParams{
		G:                bg,
		Name:             decl.Name,
		ModuleName:       bg.ModuleName,
		TypeParams:       gt.TypeParams,
		AnyValue:         fmt.Sprintf("%+#v", val),
		CustomType:       gct.Gotype.Pkg + "." + gct.Gotype.Name,
		CustomTypeHelper: helperName,
		TypeExprStrs:     typeExprStrs,
	})
}

// strRep renders a TypeExpr as the Go source that reconstructs it.
func (bg *Generator) strRep(te adlast.TypeExpr) string {
	bg.Cli.GoImport("adlast", bg.ModuleName, &bg.Imports)
	return RenderString("texprParams", texprParams{G: bg, Te: te})
}

// texprParams renders one TypeExpr; the template recurses over Params.
type texprParams struct {
	G  *Generator
	Te adlast.TypeExpr
}

func (p texprParams) Params() []texprParams {
	out := make([]texprParams, len(p.Te.Parameters))
	for i, te := range p.Te.Parameters {
		out[i] = texprParams{G: p.G, Te: te}
	}
	return out
}

type custTypeConstructionParams struct {
	G                *Generator
	ModuleName       string
	Name             string
	TypeParams       TypeParam
	AnyValue         string
	CustomType       string
	CustomTypeHelper string
	TypeExprStrs     []string
}

func (bg *goval_gen) goStruct(
	struct_ adlast.Struct,
	tbind []adl.TypeBinding,
	gt goTypeExpr,
	val any,
) string {
	mval := val.(map[string]any)
	ret := lo.FlatMap[adlast.Field, string](struct_.Fields, func(fld adlast.Field, _ int) []string {
		bg.path = append(bg.path, fld.Name)
		ret := []string{}
		if bg.genAdlAst && fld.Name == "annotations" {
			bg.Cli.GoImport("customtypes", bg.ModuleName, &bg.Imports)
			anns := mval[fld.SerializedName].([]any)
			annvs := []string{}
			for _, mapEntry := range anns {
				ann := mapEntry.(map[string]any)
				k := ann["k"].(map[string]any)
				v := ann["v"]
				mn := k["moduleName"]
				na := k["name"]
				//TODO write custom any -> go val func
				annvs = append(annvs, RenderString("annEntryParams", annEntryParams{ModuleName: mn, Name: na, Val: v}))
			}
			// sort so there is a determistic order for generated AST code
			sort.Strings(annvs)
			ret = append(ret, RenderString("annMapParams", annMapParams{Entries: annvs}))
			return ret
		}
		if v, ok := mval[fld.SerializedName]; ok {
			monoTe, _ := adl.SubstituteTypeBindings(tbind, fld.TypeExpr)
			fgv := bg.goValue(fld.Annotations, monoTe, v)
			ret = append(ret, fgv)
			// ret = append(ret, fmt.Sprintf(`%s: %s`, public(fld.Name), fgv))
		}
		if _, ok := mval[fld.SerializedName]; !ok {
			types.Handle_Maybe[any, any](
				fld.Default,
				func(nothing struct{}) any {
					return nil
				},
				func(just any) any {
					monoTe, _ := adl.SubstituteTypeBindings(tbind, fld.TypeExpr)
					var fgv string
					if just != nil {
						val = reflect.ValueOf(just).Interface()
						fgv = bg.goValue(fld.Annotations, monoTe, val)
					} else {
						fgv = bg.goValue(fld.Annotations, monoTe, nil)
					}
					ret = append(ret, fgv)
					// ret = append(ret, fmt.Sprintf(`%s: %s`, public(fld.Name), fgv))
					return nil
				},
				nil,
			)
		}
		return ret
	})
	return ctorParams{
		Ctor: qualify(gt.Pkg) + "MakeAll_" + gt.Type + gt.TypeParams.RSide(),
		Args: ret,
	}.StringRep()
}

func (bg *goval_gen) goUnion(
	union_ adlast.Union,
	decl_name string,
	tbind []adl.TypeBinding,
	gt goTypeExpr,
	val any,
) string {
	var (
		k string
		v any
	)
	switch t := val.(type) {
	case string:
		k = t
		v = nil
	case map[string]any:
		if len(t) != 1 {
			panic(fmt.Sprintf("expect an object with one and only element received %v - %v", len(t), t))
		}
		for k0, v0 := range t {
			k = k0
			v = v0
		}
	default:
		panic(fmt.Errorf("union: expect an object received %v '%v'", reflect.TypeOf(val), val))
	}
	var fld *adlast.Field
	for _, f0 := range union_.Fields {
		if f0.SerializedName == k {
			fld = &f0
			break
		}
	}
	if fld == nil {
		panic(fmt.Errorf("unexpected branch - no type registered '%v'", k))
	}
	bg.path = append(bg.path, fld.Name)
	monoTe, _ := adl.SubstituteTypeBindings(tbind, fld.TypeExpr)
	// f_tp := typeParam{
	// 	ps: slices.Map[adlast.TypeExpr, string](monoTe.Parameters, func(a adlast.TypeExpr) string {
	// 		return bg.GoType(a).Type
	// 	}),
	// }

	// if f_tp0, ok := fld.TypeExpr.TypeRef.Cast_typeParam(); ok {
	// 	// if f_tp0, ok := fld.TypeExpr.TypeRef.Branch.(adlast.TypeRef_TypeParam); ok {
	// 	// 	f_tp0 := f_tp0.V
	// 	ok := false
	// 	for _, x := range tbind {
	// 		if x.Name == f_tp0 {
	// 			ok = true
	// 			monoGt := bg.GoType(x.Value)
	// 			f_tp = typeParam{
	// 				ps: []string{monoGt.Type},
	// 			}
	// 			break
	// 		}
	// 	}
	// 	if !ok {
	// 		panic(fmt.Errorf("type param not found"))
	// 	}
	// }

	ctor := ctorParams{
		Ctor: qualify(gt.Pkg) + "Make_" + gt.Type + "_" + fld.Name + gt.TypeParams.RSide(),
	}
	// A Void branch takes no argument; every other branch takes one.
	if pr, isPrim := fld.TypeExpr.TypeRef.Cast_primitive(); !isPrim || pr != "Void" {
		ctor.Args = []string{bg.goValue(fld.Annotations, monoTe, v)}
	}
	return ctor.StringRep()

	// ret := []string{
	// 	fmt.Sprintf("%s%s_%s%s{\nV: %v}",
	// 		pkg,
	// 		decl_name,
	// 		public(fld.Name),
	// 		f_tp.RSide(),
	// 		bg.goValue(fld.Annotations, monoTe, v),
	// 	),
	// }
	// return fmt.Sprintf("%s{\nBranch: %s,\n}", gt.String(), strings.Join(ret, ",\n"))
}

// ctorParams renders a call to a generated constructor: MakeAll_X for a
// struct, Make_X_branch for a union, or a newtype conversion. No args
// renders as "()"; otherwise each arg goes on its own line.
type ctorParams struct {
	Ctor string
	Args []string
}

func (p ctorParams) StringRep() string {
	sb := &strings.Builder{}
	sb.WriteString(p.Ctor)
	sb.WriteString("(")
	for i, a := range p.Args {
		if i == 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(a)
		sb.WriteString(",")
		sb.WriteString("\n")
	}
	sb.WriteString(")")
	return sb.String()
}

// annMapParams renders the annotations map of a generated AST decl.
type annMapParams struct {
	Entries []string
}

// annEntryParams renders one entry of that map. Entries are rendered before
// being sorted, so that the generated AST is deterministic.
type annEntryParams struct {
	ModuleName any
	Name       any
	Val        any
}

func (p annEntryParams) IsNil() bool { return p.Val == nil }

// GoSyntax is the annotation value in Go-syntax representation.
func (p annEntryParams) GoSyntax() string { return fmt.Sprintf("%+#v", p.Val) }

// qualify turns a package name into the prefix used to reference it, and is
// empty for the package being generated.
func qualify(pkg string) string {
	if pkg == "" {
		return ""
	}
	return pkg + "."
}

func (bg *goval_gen) goValuePrimitive(
	anns adlast.Annotations,
	te adlast.TypeExpr,
	primitive string,
	val any,
) string {
	// if val == nil {
	// 	panic(fmt.Errorf("!!! primitive: %v %+#v", primitive, te))
	// }
	switch primitive {
	case "TypeToken":
		pkg, err := bg.Cli.GoImport("adlast", bg.ModuleName, &bg.Imports)
		if err != nil {
			panic(err)
		}
		// return bg.GoTexprValue(te.Parameters[0], anns)
		gt := bg.GoType(te.Parameters[0], anns)
		return fmt.Sprintf("%sMake_ATypeExpr[%s](%s)", pkg, gt, bg.GoTexprValue(te.Parameters[0], anns))
	case "Int8", "Int16", "Int32", "Int64",
		"Word8", "Word16", "Word32", "Word64",
		"Bool", "Float", "Double":
		return fmt.Sprintf("%v", val)
	case "String":
		by, _ := json.Marshal(val)
		return string(by)
	// case "ByteVector":
	case "Void":
		return "struct{}{}"
	case "Json":
		//TODO write custom any -> go val func
		if val == nil {
			return "nil"
		}
		return fmt.Sprintf("%+#v", val)
	case "Vector":
		rv := reflect.ValueOf(val)
		vs := make([]string, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			bg.path = append(bg.path, fmt.Sprintf("[%d]", i))
			v := rv.Index(i)
			vs[i] = bg.goValue(anns, te.Parameters[0], v.Interface())
		}
		if len(vs) == 0 {
			return fmt.Sprintf("[]%s{}", bg.GoType(te.Parameters[0], anns))
		}
		vss := strings.Join(vs, ",\n")
		return fmt.Sprintf("[]%s{\n%s,\n}", bg.GoType(te.Parameters[0], anns), vss)
	case "StringMap":
		m := val.(map[string]any)
		vs := make(kvBy, 0, len(m))
		for k, v := range m {
			vs = append(vs, kv{k, bg.goValue(anns, te.Parameters[0], v)})
		}
		if len(vs) == 0 {
			return fmt.Sprintf("map[string]%s{}", bg.GoType(te.Parameters[0], anns))
		}
		sort.Sort(vs)
		return fmt.Sprintf("map[string]%s{\n%s,\n}", bg.GoType(te.Parameters[0], anns), vs)
	case "Nullable":
		if val == nil {
			return "nil"
		}
		gl, _ := bg.Cli.GoImport("adl", bg.ModuleName, &bg.Imports)
		return gl + "Addr(" + bg.goValue(anns, te.Parameters[0], val) + ")"
	}
	panic("Unknown GoValuePrimitive")
}

type kv struct {
	k string
	v string
}

type kvBy []kv

func (kv kv) String() string {
	return fmt.Sprintf(`"%s" : %s`, kv.k, kv.v)
}
func (elems kvBy) String() string {
	var b strings.Builder
	// b.Grow(n)
	b.WriteString(elems[0].String())
	for _, s := range elems[1:] {
		b.WriteString(",\n")
		b.WriteString(s.String())
	}
	return b.String()
}

func (a kvBy) Len() int           { return len(a) }
func (a kvBy) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a kvBy) Less(i, j int) bool { return a[i].k < a[j].k }
