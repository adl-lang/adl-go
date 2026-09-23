package gotypes

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/gogen"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/gomod"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/loader"

	"golang.org/x/sync/errgroup"
)

func (in *GoTypes) Run() error {
	lr := in.Loader
	gm := in.GoMod
	eg := &errgroup.Group{}
	for _, m := range lr.Modules {
		fn := thunk_gen_module(m, in, gm)
		// fn()
		eg.Go(fn)
	}
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("error generating module : %w", err)
	}
	return nil
}

func thunk_gen_module(
	m loader.NamedModule,
	in *GoTypes,
	gm *gomod.GoModResult,
) func() error {
	fn := func() error {
		modCodeGenDir := strings.Split(m.Name, ".")
		modCodeGenPkg := modCodeGenDir[len(modCodeGenDir)-1]

		oabs, err1 := filepath.Abs(in.Outputdir)
		rabs, err2 := filepath.Abs(gm.RootDir)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("error get abs dir %w %w", err1, err2)
		}
		if !strings.HasPrefix(oabs, rabs) {
			return fmt.Errorf("output dir must be inside root of go.mod out: %s root: %s", oabs, rabs)
		}
		// if in.Root.Debug {
		// 	fmt.Fprintf(os.Stderr, "out: '%s' root: '%s'\n", oabs, rabs)
		// }
		var midPath string
		if oabs != rabs {
			midPath = oabs[len(rabs)+1:]
		}
		path := in.Outputdir + "/" + strings.Join(modCodeGenDir, "/")
		declGen := &gogen.Generator{
			BaseGen: gogen.NewBaseGen(gm.ModulePath, midPath, m.Name, in, *in.Loader),
		}
		astGen := &gogen.Generator{
			BaseGen: gogen.NewBaseGen(gm.ModulePath, midPath, m.Name, in, *in.Loader),
		}
		declsNames := []string{}
		for k := range m.Module_.Decls {
			declsNames = append(declsNames, k)
		}
		slices.Sort(declsNames)
		var (
			decls []declParams
			asts  []astDeclParams
		)
		for _, k := range declsNames {
			decl := m.Module_.Decls[k]
			if gogen.GoCustomTypeAnn(decl.Annotations) == nil {
				if p, ok := makeDeclParams(declGen, decl); ok {
					decls = append(decls, p)
				}
			}
			if !in.ExcludeAst {
				asts = append(asts, makeAstDeclParams(astGen, decl))
			}
		}

		err := gogen.WriteFile(in.Root, filepath.Join(path, modCodeGenDir[len(modCodeGenDir)-1]+".go"), in.NoGoFmt,
			&gogen.FileParams{
				Pkg:      modCodeGenPkg,
				G:        declGen,
				BodyTmpl: "gotypes_decls_body",
				BodyData: declsBodyParams{Decls: decls},
			})
		if err != nil {
			return err
		}
		if !in.ExcludeAst {
			fname := modCodeGenDir[len(modCodeGenDir)-1] + "_ast.go"
			astFile := &gogen.FileParams{
				Pkg:      modCodeGenPkg,
				G:        astGen,
				BodyTmpl: "gotypes_ast_body",
				BodyData: astBodyParams{Decls: asts},
			}
			astPath := filepath.Join(path, fname)
			if _, ok := in.specialTexpr()[m.Name]; ok && in.StdLibGen {
				astFile.Pkg = "adl"
				astFile.Special = []goimports.ImportSpec{{
					Path:    filepath.Join(in.GoAdlPath, strings.ReplaceAll(m.Name, ".", "/")),
					Name:    ".",
					Aliased: true,
				}}
				astPath = filepath.Join(in.Outputdir, fname)
			}
			if err = gogen.WriteFile(in.Root, astPath, in.NoGoFmt, astFile); err != nil {
				return err
			}
		}
		return nil
	}
	return fn
}

func (in *GoTypes) ReservedImports() []goimports.ImportSpec {
	return []goimports.ImportSpec{
		{Path: "encoding/json"},
		{Path: "reflect"},
		{Path: "strings"},
		{Path: "fmt"},
		{Path: in.GoAdlPath, Aliased: false, Name: "adl"},
		{Path: in.GoAdlPath + "/sys/adlast", Aliased: false, Name: "adlast"},
		{Path: in.GoAdlPath + "/adljson", Aliased: false, Name: "adljson"},
		{Path: in.GoAdlPath + "/customtypes", Aliased: false, Name: "customtypes"},
	}
}

func (in *GoTypes) specialTexpr() map[string]struct{} {
	return map[string]struct{}{
		"sys.adlast":      {},
		"sys.types":       {},
		"adlc.config.go_": {},
	}
}

func (bg *GoTypes) GoImport(pkg string, currModuleName string, imports *goimports.Imports) (string, error) {
	if _, ok := bg.specialTexpr()[currModuleName]; ok && bg._GoTypes.StdLibGen && pkg == "adl" {
		return "", nil
	}
	if spec, ok := imports.ByName(pkg); !ok {
		return "", fmt.Errorf("unknown import %s", pkg)
	} else {
		imports.AddPath(spec.Path)
		return spec.Name + ".", nil
	}
}

func (bg *GoTypes) IsStdLibGen() bool {
	return bg._GoTypes.StdLibGen
}

func (bg *GoTypes) GoAdlImportPath() string {
	return bg._GoTypes.GoAdlPath
}

// declsBodyParams is the body of a module's types file.
type declsBodyParams struct {
	Decls []declParams
}

// astBodyParams is the body of a module's _ast.go file.
type astBodyParams struct {
	Decls []astDeclParams
}

// astDeclParams is one decl's contribution to the _ast.go file: its Texpr_
// func, which a generic type alias does not get, and its AST_ registration,
// which every decl gets.
type astDeclParams struct {
	Texpr *aTexprParams
	Reg   scopedDeclParams
}

// makeDeclParams builds the params for one decl of the types file. ok is
// false for a generic type alias, which generates nothing: go has no
// "type X[A any] = ...".
func makeDeclParams(in *gogen.Generator, decl adlast.Decl) (p declParams, ok bool) {
	if typ, isAlias := decl.Type_.Cast_type_(); isAlias && len(typ.TypeParams) != 0 {
		return declParams{}, false
	}
	return declParams{
		G:          in,
		Decl:       decl,
		Name:       decl.Name,
		TypeParams: gogen.TypeParamsFromDecl(decl),
	}, true
}

// makeAstDeclParams builds the params for one decl of the _ast.go file.
func makeAstDeclParams(body *gogen.Generator, decl adlast.Decl) astDeclParams {
	return astDeclParams{
		Texpr: makeATexprParams(body, decl),
		Reg: scopedDeclParams{
			G:          body,
			ModuleName: body.ModuleName,
			Name:       decl.Name,
			Decl:       decl,
			TypeParams: gogen.TypeParamsFromDecl(decl),
		},
	}
}

// makeATexprParams returns nil for a generic type alias, which gets no
// Texpr_ func.
func makeATexprParams(body *gogen.Generator, decl adlast.Decl) *aTexprParams {
	if typ, isAlias := decl.Type_.Cast_type_(); isAlias && len(typ.TypeParams) != 0 {
		return nil
	}
	type_name := decl.Name
	tp := gogen.TypeParamsFromDecl(decl)
	if gct := gogen.GoCustomTypeAnn(decl.Annotations); gct != nil {
		body.Imports.AddSpec(gogen.GoCustomTypeSpec(gct))
		type_name = gct.Gotype.Pkg + "." + gct.Gotype.Name
		tp.TypeConstraints = gct.Gotype.Type_constraints
	}
	return &aTexprParams{
		G:          body,
		ModuleName: body.ModuleName,
		Name:       decl.Name,
		TypeName:   type_name,
		TypeParams: tp,
	}
}
