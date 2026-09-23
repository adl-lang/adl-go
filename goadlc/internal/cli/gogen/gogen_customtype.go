package gogen

import (
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

// importSpecFor is the import that referencing pkg at importPath requires.
// The name is an alias whenever it differs from the path's last segment.
func importSpecFor(importPath, pkg string) goimports.ImportSpec {
	last := importPath[strings.LastIndex(importPath, "/")+1:]
	return goimports.ImportSpec{
		Path:    importPath,
		Name:    pkg,
		Aliased: pkg != last,
	}
}

// GoCustomTypeSpec is the import that referencing the custom type requires.
func GoCustomTypeSpec(gct *go_.GoCustomType) goimports.ImportSpec {
	return importSpecFor(gct.Gotype.Import_path, gct.Gotype.Pkg)
}

// HelperName is the Go name of a custom type's helper, qualified when the
// helper lives in another package - in which case calling this registers
// that package's import.
func (in *Generator) HelperName(gct *go_.GoCustomType) string {
	if gct.Helpers.Ref == nil {
		return gct.Helpers.Name
	}
	in.Imports.AddSpec(importSpecFor(gct.Helpers.Ref.Import_path, gct.Helpers.Ref.Pkg))
	return gct.Helpers.Ref.Pkg + "." + gct.Helpers.Name
}

// RegisterHelperName is the helper the "registerHelper" template registers
// for decl, or "" when the decl carries no go_custom_type annotation.
func (in *Generator) RegisterHelperName(decl adlast.Decl) string {
	var gct *go_.GoCustomType
	if gct = GoCustomTypeAnn(decl.Annotations); gct == nil {
		return ""
	}
	return in.HelperName(gct)
}

// IsStdLibGen reports whether this is the generation of the adl stdlib
// itself, which registers into its own RESOLVER rather than adl's.
func (in *Generator) IsStdLibGen() bool {
	return in.Cli.IsStdLibGen()
}
