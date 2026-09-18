package gogen

import (
	"fmt"
	"strings"

	"github.com/adl-lang/adl-go/adl"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
)

var GoCustomTypeSN = adlast.Make_ScopedName(
	"adlc.config.go_",
	"GoCustomType",
)

func (in *Generator) GoRegisterHelper(moduleName string, decl adlast.Decl) (string, error) {
	jb := adl.CreateJsonDecodeBinding(adl.Texpr_GoCustomType(), adl.RESOLVER)
	gct, err := adl.GetAnnotation(decl.Annotations, GoCustomTypeSN, jb)
	if err != nil {
		return "", err
	}
	if gct == nil {
		return "", nil
	}
	helperName := gct.Helpers.Name
	if gct.Helpers.Ref != nil {
		helperName = gct.Helpers.Ref.Pkg + "." + gct.Helpers.Name
		pkg := gct.Helpers.Ref.Import_path[strings.LastIndex(gct.Helpers.Ref.Import_path, "/")+1:]
		spec := goimports.ImportSpec{
			Path:    gct.Helpers.Ref.Import_path,
			Name:    gct.Helpers.Ref.Pkg,
			Aliased: gct.Helpers.Ref.Pkg != pkg,
		}
		in.Imports.AddSpec(spec)
	}
	// if this gets into trouble use in.GoImport
	if in.Cli.IsStdLibGen() {
		return fmt.Sprintf(`	RESOLVER.RegisterHelper(
			adlast.Make_ScopedName("%s", "%s"),
			(*%s)(nil),
		)
`, moduleName, decl.Name, helperName), nil
	}
	return fmt.Sprintf(`	adl.RESOLVER.RegisterHelper(
			adlast.Make_ScopedName("%s", "%s"),
			(*%s)(nil),
		)
`, moduleName, decl.Name, helperName), nil
}
