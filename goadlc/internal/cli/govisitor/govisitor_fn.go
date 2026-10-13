package govisitor

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/gogen"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/loader"

	"golang.org/x/sync/errgroup"
)

// Run writes one <pkg>_visitor.go per module in the generated set, beside
// the <pkg>.go that gotypes wrote. A module with no generated decl (struct
// or union) gets no file.
func (in *GoVisitor) Run() error {
	var (
		eg        = &errgroup.Group{}
		midPath   string
		mods      []loader.NamedModule
		generated map[adlast.ScopedName]bool
		err       error
	)
	if midPath, err = goimports.MidPath(in.Outputdir, in.GoMod.RootDir); err != nil {
		return err
	}
	if mods, err = in.moduleSet(); err != nil {
		return err
	}
	generated = generatedDecls(in.Loader, mods)
	for _, m := range mods {
		eg.Go(in.thunkGenModule(m, midPath, generated))
	}
	if err = eg.Wait(); err != nil {
		return fmt.Errorf("error generating visitor module : %w", err)
	}
	return nil
}

// moduleSet is the modules visitors are generated for: those named in
// Modules, or every loaded module when it is empty. Bundle modules are never
// loaded, so they are never in the set.
func (in *GoVisitor) moduleSet() ([]loader.NamedModule, error) {
	var (
		byName = map[string]loader.NamedModule{}
		mods   []loader.NamedModule
		m      loader.NamedModule
		ok     bool
	)
	if len(in.Modules) == 0 {
		return in.Loader.Modules, nil
	}
	for _, m := range in.Loader.Modules {
		byName[m.Name] = m
	}
	for _, name := range in.Modules {
		if m, ok = byName[name]; !ok {
			return nil, fmt.Errorf("GoVisitor.Modules names '%s', which is not a loaded module (bundle modules such as sys.* are never generated)", name)
		}
		mods = append(mods, m)
	}
	return mods, nil
}

// generatedDecls is the set of decls that get an Accept, which is what
// decides whether a reference to one is descended or a leaf: the structs,
// unions and newtypes of the module set, generic or not, without a
// go_custom_type annotation. A reference to one from outside the set (a
// bundle decl such as sys.types.Pair) is a leaf.
func generatedDecls(lr *loader.LoadResult, mods []loader.NamedModule) map[adlast.ScopedName]bool {
	var generated = map[adlast.ScopedName]bool{}
	for _, m := range mods {
		for name, decl := range m.Module_.Decls {
			if isGenerated(lr, decl) {
				generated[adlast.Make_ScopedName(m.Name, name)] = true
			}
		}
	}
	return generated
}

// isGenerated reports whether decl gets a visitor: a struct, union or
// newtype without a go_custom_type annotation. Type params do not matter:
// a generic decl's own params are prepended to its interfaces' and its
// receiver is the instantiated type. A type alias is never generated for;
// it is expanded wherever it appears.
//
// A newtype is generated only when Go lets it have methods, which rules out
// a newtype whose underlying Go type is a pointer (Nullable<...>), an
// interface (Json) or a bare type param, directly or through further
// aliases and newtypes: `type MaybeAtom *Atom` is a defined pointer type,
// and Go rejects any receiver whose base type is one.
func isGenerated(lr *loader.LoadResult, decl adlast.Decl) bool {
	var (
		nt adlast.NewType
		ok bool
	)
	if gogen.GoCustomTypeAnn(decl.Annotations) != nil {
		return false
	}
	if _, ok = decl.Type_.Cast_struct_(); ok {
		return true
	}
	if _, ok = decl.Type_.Cast_union_(); ok {
		return true
	}
	if nt, ok = decl.Type_.Cast_newtype_(); ok {
		return !methodless(lr, nt.TypeExpr)
	}
	return false
}

// methodless reports whether te's Go type cannot be a method receiver's
// base type: a pointer, an interface, or a type param.
func methodless(lr *loader.LoadResult, te adlast.TypeExpr) bool {
	var (
		prim string
		ref  adlast.ScopedName
		decl *adlast.Decl
		nt   adlast.NewType
		ok   bool
	)
	te = gogen.ExpandTypeAliases(lr, te)
	if prim, ok = te.TypeRef.Cast_primitive(); ok {
		return prim == "Nullable" || prim == "Json"
	}
	if _, ok = te.TypeRef.Cast_typeParam(); ok {
		return true
	}
	if ref, ok = te.TypeRef.Cast_reference(); !ok {
		return false
	}
	if decl, ok = lr.Resolver(ref); !ok {
		return false
	}
	if gogen.GoCustomTypeAnn(decl.Annotations) != nil {
		return false
	}
	if nt, ok = decl.Type_.Cast_newtype_(); ok {
		return methodless(lr, nt.TypeExpr)
	}
	return false
}

func (in *GoVisitor) thunkGenModule(
	m loader.NamedModule,
	midPath string,
	generated map[adlast.ScopedName]bool,
) func() error {
	return func() error {
		var (
			modCodeGenDir = strings.Split(m.Name, ".")
			modCodeGenPkg = modCodeGenDir[len(modCodeGenDir)-1]
			dir           = filepath.Join(in.Outputdir, filepath.Join(modCodeGenDir...))
			gen           = &gogen.Generator{
				BaseGen: gogen.NewBaseGen(in.GoMod.ModulePath, midPath, m.Name, in, *in.Loader),
			}
			// probe spells types the file never names, so that doing so
			// does not register imports on gen (see makeDeclParams).
			probe = &gogen.Generator{
				BaseGen: gogen.NewBaseGen(in.GoMod.ModulePath, midPath, m.Name, in, *in.Loader),
			}
			cls = &classifier{
				lr:        in.Loader,
				generated: generated,
			}
			declNames []string
			decls     []visitorDeclParams
		)
		for k := range m.Module_.Decls {
			declNames = append(declNames, k)
		}
		slices.Sort(declNames)
		for _, k := range declNames {
			var decl = m.Module_.Decls[k]
			if !isGenerated(in.Loader, decl) {
				continue
			}
			decls = append(decls, in.makeDeclParams(gen, probe, cls, decl))
		}
		if len(decls) == 0 {
			return nil
		}
		return gogen.WriteFile(in.Root, filepath.Join(dir, modCodeGenPkg+"_visitor.go"), in.NoGoFmt,
			&gogen.FileParams{
				Pkg:      modCodeGenPkg,
				G:        gen,
				BodyTmpl: "govisitor_body",
				BodyData: visitorBodyParams{
					Decls: decls,
				},
			})
	}
}

// makeDeclParams builds the params of one generated struct, union or
// newtype decl.
//
// A union's Go type name is GoEscape'd, as decl_Union in gotypes.tmpl does
// and decl_Struct and decl_NewType do not; its branch type names are
// _<Union>_<Branch> with the branch name public but not escaped, followed
// by the type args of whichever of the union's type params the branch type
// mentions (_Opt_Some[T] beside _Opt_None), again as gotypes emits them.
// That suffix is GoType's UnionTypeParams, asked of probe rather than gen:
// GoType registers the import of every cross-module type it spells, and
// the visitor file names none of a branch's types.
//
// A newtype's DefaultAccept visits the underlying value, (*node), as a
// struct field would be (plan4 section 4.6). The one difference is a
// newtype over a generated decl: `type Wrapped Atom` has none of Atom's
// methods, so node is converted to the underlying Go type first,
// (*tree.Atom)(node).Accept(...). That spelling is gen's GoType, so the
// cross-module import it needs is registered.
func (in *GoVisitor) makeDeclParams(
	gen *gogen.Generator,
	probe *gogen.Generator,
	cls *classifier,
	decl adlast.Decl,
) visitorDeclParams {
	var (
		name       = goPublic(decl.Name)
		tp         = gogen.TypeParamsFromDecl(decl)
		all        = tp.AddParams("P", "R")
		pr         = gogen.TypeParam{}.AddParams(all.Params[len(all.Params)-2].Name, all.Last())
		st         adlast.Struct
		un         adlast.Union
		nt         adlast.NewType
		ok         bool
		fields     []acceptFieldParams
		branches   []acceptBranchParams
		underlying visitNode
	)
	if st, ok = decl.Type_.Cast_struct_(); ok {
		for _, f := range st.Fields {
			var fname = goPublic(f.Name)
			fields = append(fields, acceptFieldParams{
				Name:  fname,
				Visit: cls.visit("node."+fname, f.TypeExpr, 0, 0),
			})
		}
	} else if un, ok = decl.Type_.Cast_union_(); ok {
		name = goPublic(gen.GoEscape(decl.Name))
		for _, f := range un.Fields {
			branches = append(branches, acceptBranchParams{
				TypeName: "_" + name + "_" + goPublic(f.Name) + probe.GoType(f.TypeExpr, f.Annotations).UnionTypeParams.RSide(),
				Visit:    cls.visit("b.V", f.TypeExpr, 0, 0),
			})
		}
	} else if nt, ok = decl.Type_.Cast_newtype_(); ok {
		underlying = cls.visit("*node", nt.TypeExpr, 0, 0)
		if _, ok = underlying.(acceptVisit); ok {
			underlying = acceptVisit{
				Expr: "(*" + gen.GoType(nt.TypeExpr, decl.Annotations).String() + ")(node)",
				Type: underlying.adlType(),
			}
		}
	}
	return visitorDeclParams{
		G:                   gen,
		Decl:                decl,
		Name:                name,
		NodeType:            name + typeArgList(tp),
		TypeParams:          tp,
		P:                   pr.Params[0].Name,
		R:                   pr.Params[1].Name,
		IfaceTP:             typeParamList(all),
		IfaceTA:             typeArgList(all),
		MethodTP:            typeParamList(pr),
		MethodTA:            typeArgList(pr),
		SkipTypeCheckMethod: in.SkipTypeCheckMethod,
		Fields:              fields,
		Branches:            branches,
		Underlying:          underlying,
	}
}

func (in *GoVisitor) ReservedImports() []goimports.ImportSpec {
	return []goimports.ImportSpec{
		{
			Path: "slices",
		},
		{
			Path: "maps",
		},
		{
			Path:    in.GoAdlPath + "/visit",
			Aliased: false,
			Name:    "visit",
		},
	}
}

func (in *GoVisitor) GoImport(pkg string, currModuleName string, imports *goimports.Imports) (string, error) {
	var (
		spec goimports.ImportSpec
		ok   bool
	)
	if spec, ok = imports.ByName(pkg); !ok {
		return "", fmt.Errorf("unknown import %s", pkg)
	}
	imports.AddPath(spec.Path)
	return spec.Name + ".", nil
}

func (in *GoVisitor) IsStdLibGen() bool {
	return false
}

func (in *GoVisitor) GoAdlImportPath() string {
	return in.GoAdlPath
}
