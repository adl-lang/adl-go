package goapi

import (
	"fmt"
	fp "path/filepath"
	"strings"

	"github.com/adl-lang/adl-go/adl/customtypes"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/gogen"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
	"github.com/samber/lo"
)

func (in *GoApi) Run() error {
	mod, ok := in.Loader.CombinedAst[in.ApiStruct.ModuleName]
	if !ok {
		return fmt.Errorf("module not found '%s", in.ApiStruct.ModuleName)
	}
	decl0, ok := mod.Decls[in.ApiStruct.Name]
	if !ok {
		return fmt.Errorf("decl not found '%s", in.ApiStruct.Name)
	}
	st, ok := decl0.Type_.Cast_struct_()
	if !ok || len(st.TypeParams) != 0 {
		return fmt.Errorf("unexpected - apiRequests is not a struct, or is a struct with generic type params")
	}
	midPath, err := goimports.MidPath(in.Outputdir, in.GoMod.RootDir)
	if err != nil {
		return err
	}
	base := gogen.NewBaseGen(
		in.GoMod.ModulePath,
		midPath,
		in.ApiStruct.ModuleName,
		in,
		*in.Loader,
	)
	body := &gogen.Generator{BaseGen: base}
	apis := &apiInstance{
		Struct:     gogen.ExpandStruct(in.Loader, st),
		ScopedName: in.ApiStruct,
		Field:      nil,
	}
	result0 := []*apiInstance{}
	visited0 := map[string]bool{}
	err = in.dfs(apis, apis, visited0, &result0)
	if err != nil {
		return err
	}
	apis0s := []srvApiParams{}
	for _, apis0 := range result0 {
		apis0s = append(apis0s, srvApiParams{
			Service:  in.makeServiceParams(body, apis0),
			Register: in.makeRegisterParams(body, apis0),
		})
	}
	modCodeGenDir := strings.Split(in.ApiStruct.ModuleName, ".")
	modCodeGenPkg := modCodeGenDir[len(modCodeGenDir)-1]
	path := fp.Join(fp.Join(in.Outputdir, fp.Join(modCodeGenDir...)), modCodeGenPkg+"_srv.go")
	return gogen.WriteFile(in.Root, path, false, &gogen.FileParams{
		Pkg:      modCodeGenPkg,
		G:        body,
		BodyTmpl: "goapi_srv_body",
		BodyData: srvBodyParams{Apis: apis0s},
	})
}

var reservedNames = []string{"C", "c", "S", "s", "V", "v"}

func (in *GoApi) dfs(root *apiInstance, apiSt *apiInstance, visited map[string]bool, result *[]*apiInstance) error {
	if lo.Contains(reservedNames, apiSt.ScopedName.Name) {
		return fmt.Errorf("error struct of type CapabilityApi name clash with type params (can't be named [C,S,V]). %s.%s", apiSt.ScopedName.ModuleName, apiSt.ScopedName.Name)
	}
	if visited[apiSt.ScopedName.Name] {
		curr, _ := lo.Find(*result, func(it *apiInstance) bool {
			return it.ScopedName.Name == apiSt.ScopedName.Name
		})
		if curr.ScopedName.ModuleName != apiSt.ScopedName.ModuleName {
			return fmt.Errorf("unexpected - (alias error) can't used two struct with the same name from different modules, use a newtype")
		}
		if root != nil {
			if root == curr {
				return fmt.Errorf("unexpected - starting struct can't also be used as a capabilty api, use a newtype")
			}
		}
		return nil
	}
	visited[apiSt.ScopedName.Name] = true
	*result = append(*result, apiSt)
	for _, fi := range apiSt.Struct.Fields {
		if ref, ok := fi.TypeExpr.TypeRef.Cast_reference(); ok {
			if ref.Name == "CapabilityApi" {
				if lo.Contains(reservedNames, fi.Name) {
					return fmt.Errorf("error fields of type CapabilityApi name clash with type params (can't be named [C,S,V]). %s.%s::%s", apiSt.ScopedName.ModuleName, apiSt.ScopedName.Name, fi.Name)
				}
				apiTe := fi.TypeExpr.Parameters[2]
				apiRef, ok := apiTe.TypeRef.Cast_reference()
				if !ok {
					return fmt.Errorf("unexpected - cap api is not a ref. TypeExpre : %v", apiTe)
				}
				decl, exist := in.Loader.Resolver(apiRef)
				if !exist {
					return fmt.Errorf("can't resolve decl. Ref : %v", apiRef)
				}
				capSt, ok := decl.Type_.Cast_struct_()
				if !ok {
					return fmt.Errorf("unexpected - cap api is not a struct. Ref : %v", apiRef)
				}
				inst0 := &apiInstance{
					Struct:     gogen.ExpandStruct(in.Loader, capSt),
					ScopedName: apiRef,
					Field:      &fi,
				}
				for _, param := range capSt.TypeParams {
					if lo.Contains(reservedNames, param) {
						return fmt.Errorf("error type param name clash for genenic API (can't be named [C,S,V]). %s.%s", apiRef.ModuleName, apiRef.Name)
					}
				}
				err := in.dfs(nil, inst0, visited, result)
				if err != nil {
					return nil
				}
			} else if ref.ModuleName != "common.http" && ref.ModuleName != "common.capability" {
				decl, exist := in.Loader.Resolver(ref)
				if !exist {
					return fmt.Errorf("can't resolve decl. Ref : %v", ref)
				}
				struct_, ok := decl.Type_.Cast_struct_()
				if !ok {
					return fmt.Errorf("unexpected - api is not a struct. Ref : %v", ref)
				}
				inst0 := &apiInstance{
					Struct:     gogen.ExpandStruct(in.Loader, struct_),
					ScopedName: ref,
					Field:      nil,
				}
				err := in.dfs(nil, inst0, visited, result)
				if err != nil {
					return nil
				}
			}
		}
	}
	return nil
}

func (in *GoApi) transKids(path string, apiSt *adlast.Struct) []tkid {
	return lo.FlatMap(apiSt.Fields, func(fi adlast.Field, _ int) []tkid {
		ret := []tkid{}
		if ref, ok := fi.TypeExpr.TypeRef.Cast_reference(); ok && ref.Name == "CapabilityApi" {
			apiTe := fi.TypeExpr.Parameters[2]
			apiRef, _ := apiTe.TypeRef.Cast_reference()
			decl, _ := in.Loader.Resolver(apiRef)
			capSt, _ := decl.Type_.Cast_struct_()
			kids := in.transKids(path+fi.Name+"_", &capSt)

			name := path + fi.Name
			ret = append(ret, tkid{name, &fi})
			ret = append(ret, kids...)
		}
		return ret
	})
}

// srvBodyParams is the body of a module's _srv.go file: each api's service
// interface followed by its register func.
type srvBodyParams struct {
	Apis []srvApiParams
}

type srvApiParams struct {
	Service  serviceParams
	Register registerParams
}

type apiInstance struct {
	Struct     adlast.Struct
	ScopedName adlast.ScopedName
	Field      *adlast.Field
}

func (in *GoApi) makeServiceParams(
	body *gogen.Generator,
	inst *apiInstance,
) serviceParams {
	tps := gogen.TypeParam{}
	if inst.Field != nil {
		tps = tps.AddParams("C", "S")
		tps = tps.AddParams(inst.Struct.TypeParams...)
	}
	svc := serviceParams{
		G:          body,
		Name:       inst.ScopedName.Name,
		TypeParams: tps,
		IsCap:      inst.Field != nil,
	}
	for _, fi := range inst.Struct.Fields {
		if ref, ok := fi.TypeExpr.TypeRef.Cast_reference(); ok {
			switch ref.Name {
			case "HttpPost":
				svc.Methods = append(svc.Methods, postParams{
					G:           body,
					Name:        fi.Name,
					Annotations: fi.Annotations,
					Req:         fi.TypeExpr.Parameters[0],
					Resp:        fi.TypeExpr.Parameters[1],
					IsCap:       inst.Field != nil,
				})
			case "HttpGet":
				svc.Methods = append(svc.Methods, getParams{
					G:           body,
					Name:        fi.Name,
					Annotations: fi.Annotations,
					Resp:        fi.TypeExpr.Parameters[0],
					IsCap:       inst.Field != nil,
				})
			case "CapabilityApi":
				apiTe := fi.TypeExpr.Parameters[2]
				apiRef, _ := apiTe.TypeRef.Cast_reference()
				svc.Methods = append(svc.Methods, getcapapiParams{
					G:           body,
					Name:        fi.Name,
					StructName:  apiRef.Name,
					Annotations: fi.Annotations,
					C:           fi.TypeExpr.Parameters[0],
					S:           fi.TypeExpr.Parameters[1],
					Params:      apiTe.Parameters,
				})
			default:
				rd, _ := body.Resolver(ref)
				if _, ok := rd.Type_.Cast_struct_(); ok {
					svc.Methods = append(svc.Methods, getapiParams{
						G:           body,
						Name:        fi.Name,
						StructName:  ref.Name,
						Annotations: fi.Annotations,
						Params:      fi.TypeExpr.Parameters,
					})
				}
			}
		}

	}
	return svc
}

func (in *GoApi) makeRegisterParams(
	body *gogen.Generator,
	inst *apiInstance,
) registerParams {
	ann := customtypes.MapMap[adlast.ScopedName, any]{}
	var (
		vte *adlast.TypeExpr
	)
	tps := gogen.TypeParam{}
	if inst.Field != nil {
		// set CapabilityApi::V params as we want the generic version
		inst.Field.TypeExpr.Parameters[2].Parameters = lo.Map[string, adlast.TypeExpr](inst.Struct.TypeParams, func(item string, index int) adlast.TypeExpr {
			return adlast.Make_TypeExpr(
				adlast.Make_TypeRef_typeParam(item),
				[]adlast.TypeExpr{},
			)
		})
		ann = inst.Field.Annotations
		vte = &inst.Field.TypeExpr.Parameters[2]
		tps = tps.AddParams("C", "S")
		tps = tps.AddParams(inst.Struct.TypeParams...)
	}
	tkids := in.transKids("", &inst.Struct)
	reg := registerParams{
		G:           body,
		CapModule:   "common.capability",
		Name:        inst.ScopedName.Name,
		TypeParams:  tps,
		IsCap:       inst.Field != nil,
		V:           vte,
		Annotations: ann,
		CapApis:     tkids,
	}
	for _, fi := range inst.Struct.Fields {
		if ref, ok := fi.TypeExpr.TypeRef.Cast_reference(); ok {
			switch ref.Name {
			case "HttpPost":
				reg.Regs = append(reg.Regs, regpostParams{
					G:      body,
					Module: ref.ModuleName,
					Name:   fi.Name,
					IsCap:  inst.Field != nil,
				})
			case "HttpGet":
				reg.Regs = append(reg.Regs, reggetParams{
					G:      body,
					Module: ref.ModuleName,
					Name:   fi.Name,
					IsCap:  inst.Field != nil,
				})
			case "CapabilityApi":
				apiTe := fi.TypeExpr.Parameters[2]
				apiRef, _ := apiTe.TypeRef.Cast_reference()
				decl, _ := in.Loader.Resolver(apiRef)
				capSt, _ := decl.Type_.Cast_struct_()
				tkid := []tkid{{fi.Name, &fi}}
				tkid = append(tkid, in.transKids(fi.Name+"_", &capSt)...)
				reg.Regs = append(reg.Regs, regcapapiParams{
					G:          body,
					StructName: apiRef.Name,
					Module:     ref.ModuleName,
					Name:       fi.Name,
					Kids:       tkid,
				})
			default:
				rd, _ := body.Resolver(ref)
				if _, ok := rd.Type_.Cast_struct_(); ok {
					reg.Regs = append(reg.Regs, regapiParams{
						G:          body,
						StructName: ref.Name,
						Module:     ref.ModuleName,
						Name:       fi.Name,
					})
				}
			}
		}
	}
	return reg
}

func (in *GoApi) ReservedImports() []goimports.ImportSpec {
	return []goimports.ImportSpec{
		{Path: "net/http"},
		{Path: "context"},
		// {Path: in.GoAdlCommonImport + "/common/http", Name: "http2", Aliased: true},
		// {Path: in.GoAdlCommonImport + "/common/capability"},
	}
}

// GoImport implements gogen.SubTask.
func (bg *GoApi) IsStdLibGen() bool {
	return false
}

// GoImport implements gogen.SubTask.
func (bg *GoApi) GoAdlImportPath() string {
	return "github.com/adl-lang/adl-go/adl"
}

// GoImport implements gogen.SubTask.
func (in *GoApi) GoImport(pkg string, currModuleName string, imports *goimports.Imports) (string, error) {
	if strings.Contains(pkg, ".") {
		parts := strings.Split(pkg, ".")
		spec := goimports.ImportSpec{
			Name: parts[len(parts)-1],
			Path: in.GoAdlCommonImport + "/" + strings.Join(parts, "/"),
		}
		if spec0, ok := imports.ByPath(spec.Path); ok {
			return spec0.Name + ".", nil
		}
		return imports.AddSpec(spec) + ".", nil
	}
	if spec, ok := imports.ByName(pkg); !ok {
		return "", fmt.Errorf("unknown import %s", pkg)
	} else {
		imports.AddPath(spec.Path)
		return spec.Name + ".", nil
	}
}
