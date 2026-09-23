package gogen

import (
	"fmt"
	"strings"

	"github.com/adl-lang/adl-go/adl"
	"github.com/adl-lang/adl-go/adl/adlc/config/go_"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
)

var GoCustomTypeSN = adlast.Make_ScopedName(
	"adlc.config.go_",
	"GoCustomType",
)

// GoCustomTypeAnn returns the go_custom_type annotation, or nil when the
// decl carries none.
func GoCustomTypeAnn(anns adlast.Annotations) *go_.GoCustomType {
	var (
		jb  = adl.CreateJsonDecodeBinding(adl.Texpr_GoCustomType(), adl.RESOLVER)
		gct *go_.GoCustomType
		err error
	)
	if gct, err = adl.GetAnnotation(anns, GoCustomTypeSN, jb); err != nil {
		panic(err)
	}
	return gct
}

// GoCustomTypeSpec is the import that referencing the custom type requires.
func GoCustomTypeSpec(gct *go_.GoCustomType) goimports.ImportSpec {
	pkg := gct.Gotype.Import_path[strings.LastIndex(gct.Gotype.Import_path, "/")+1:]
	return goimports.ImportSpec{
		Path:    gct.Gotype.Import_path,
		Name:    gct.Gotype.Pkg,
		Aliased: gct.Gotype.Pkg != pkg,
	}
}

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
