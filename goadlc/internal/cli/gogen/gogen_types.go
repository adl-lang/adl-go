package gogen

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"

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
