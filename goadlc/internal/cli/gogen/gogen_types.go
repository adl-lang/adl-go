package gogen

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/loader"
)

type SnResolver func(sn adlast.ScopedName) (*adlast.Decl, bool)

type SubTask interface {
	GoImport(pkg, currModuleName string, imports *goimports.Imports) (string, error)
	ReservedImports() []goimports.ImportSpec
	IsStdLibGen() bool
	GoAdlImportPath() string
}

type BaseGen struct {
	Cli        SubTask
	Resolver   SnResolver
	ModulePath string
	MidPath    string
	ModuleName string
	Imports    goimports.Imports
}

type Generator struct {
	*BaseGen
}

// // sign used by templates
// type BaseGenerator interface {
// 	GoType(typeExpr adlast.TypeExpr, anns customtypes.MapMap[adlast.ScopedName, any]) goTypeExpr
// 	PrimitiveMap(p string, params []adlast.TypeExpr, unionTypeParams *TypeParam, anns customtypes.MapMap[adlast.ScopedName, any]) goTypeExpr
// 	goType(typeExpr adlast.TypeExpr, unionTypeParams *TypeParam, anns customtypes.MapMap[adlast.ScopedName, any]) goTypeExpr
// 	gotype_ref_customtype(decl *adlast.Decl, typeExpr adlast.TypeExpr, unionTypeParams *TypeParam, anns customtypes.MapMap[adlast.ScopedName, any]) goTypeExpr
// }

// type GoGenerator interface {
// 	GoDeclValue(val adlast.Decl) string
// 	GoEscape(n string) string
// 	GoImport(s string) (string, error)
// 	GoRegisterHelper(moduleName string, decl adlast.Decl) (string, error)
// 	GoTexprValue(val adlast.TypeExpr, anns customtypes.MapMap[adlast.ScopedName, any]) string
// 	GoValue(anns customtypes.MapMap[adlast.ScopedName, any], te adlast.TypeExpr, val any) string
// 	JsonEncode(val any) string
// 	ToTitle(s string) string
// 	goCustomType(decl *adlast.Decl, monoTe adlast.TypeExpr, gt goTypeExpr, val any) string
// 	strRep(te adlast.TypeExpr) string
// }

func (in *Generator) GoImport(s string) (string, error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Fprintf(os.Stderr, "ERROR in GoImport %v\n%v", r, string(debug.Stack()))
			panic(r)
		}
	}()
	return in.Cli.GoImport(s, in.ModuleName, &in.Imports)
}

// mustImport is GoImport for callers that treat an unknown package as a bug,
// matching what a template does when it calls .GoImport.
func (in *Generator) mustImport(pkg string) string {
	var (
		qualifier string
		err       error
	)
	if qualifier, err = in.GoImport(pkg); err != nil {
		panic(err)
	}
	return qualifier
}

func (in *Generator) ToTitle(s string) string {
	return strings.ToTitle(s)
}

func (in *Generator) GoEscape(n string) string {
	if g, h := goKeywords[n]; h {
		return g
	}
	return n
}

func NewBaseGen(
	modulePath string,
	midPath string,
	moduleName string,
	in SubTask,
	loader loader.LoadResult,
) *BaseGen {
	imports := goimports.NewImports(
		in.ReservedImports(),
		loader.BundleMaps,
	)
	return &BaseGen{
		Cli:        in,
		Resolver:   loader.Resolver,
		ModulePath: modulePath,
		MidPath:    midPath,
		ModuleName: moduleName,
		Imports:    imports,
	}
}

func renderPanic(params any, err error) {
	data, _ := json.Marshal(params)
	fmt.Fprintf(os.Stderr, "error executing template -- type: %T\nerror: %v\n%s", params, err, string(data))
	panic(err)
}
