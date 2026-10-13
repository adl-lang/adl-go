package gogen

import (
	"bytes"
	"testing"

	"github.com/adl-lang/adl-go/adl/customtypes"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/adl/sys/types"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/templates"
)

// The regen reaches the stdlib form of this registration (adl/types_ast.go)
// but never the qualified one: that needs a go_custom_type decl outside the
// stdlib generation, which none of the regenerated ADL has. Pin both here,
// along with the import the qualified helper name registers.

// stubTask stands in for a generator sub-task, with IsStdLibGen settable so
// that both arms of the template are reachable.
type stubTask struct{ stdlib bool }

func (stubTask) GoImport(pkg, curr string, imports *goimports.Imports) (string, error) {
	return pkg + ".", nil
}
func (stubTask) ReservedImports() []goimports.ImportSpec { return nil }
func (s stubTask) IsStdLibGen() bool                     { return s.stdlib }
func (stubTask) GoAdlImportPath() string                 { return "" }

// customTypeAnn is a go_custom_type annotation in the any-shape that
// adl.GetAnnotation decodes from. helperRef is the helper's package, or ""
// for a helper in the generated package itself.
func customTypeAnn(helperName, helperRef string) adlast.Annotations {
	helpers := map[string]any{"name": helperName}
	if helperRef != "" {
		helpers["ref"] = map[string]any{
			"pkg":         helperRef,
			"import_path": "github.com/example/" + helperRef,
		}
	}
	return customtypes.MapMap[adlast.ScopedName, any]{
		GoCustomTypeSN: map[string]any{
			"gotype": map[string]any{
				"name":             "T",
				"pkg":              "pkg",
				"import_path":      "github.com/example/pkg",
				"type_constraints": []any{},
			},
			"helpers": helpers,
		},
	}
}

// decl is a minimal Decl carrying just the annotations under test.
func decl(anns adlast.Annotations) adlast.Decl {
	return adlast.MakeAll_Decl(
		"Name",
		types.Make_Maybe_nothing[uint32](),
		adlast.Make_DeclType_struct_(adlast.MakeAll_Struct(nil, nil)),
		anns,
	)
}

func TestRegisterHelperTemplate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stdlib bool
		anns   adlast.Annotations
		want   string
		spec   string // import path the render is expected to register
	}{
		{
			name: "no go_custom_type annotation renders nothing",
			anns: customtypes.MapMap[adlast.ScopedName, any]{},
			want: "",
		},
		{
			name:   "stdlib gen registers into its own RESOLVER",
			stdlib: true,
			anns:   customTypeAnn("MapHelper", ""),
			want: "\tRESOLVER.RegisterHelper(\n" +
				"\t\t\tadlast.Make_ScopedName(\"a.mod\", \"Name\"),\n" +
				"\t\t\t(*MapHelper)(nil),\n" +
				"\t\t)\n",
		},
		{
			name: "otherwise it qualifies both the resolver and the helper",
			anns: customTypeAnn("MapHelper", "helper"),
			want: "\tadl.RESOLVER.RegisterHelper(\n" +
				"\t\t\tadlast.Make_ScopedName(\"a.mod\", \"Name\"),\n" +
				"\t\t\t(*helper.MapHelper)(nil),\n" +
				"\t\t)\n",
			spec: "github.com/example/helper",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				g = &Generator{BaseGen: &BaseGen{
					Cli:     stubTask{stdlib: tc.stdlib},
					Imports: goimports.NewImports(nil, nil),
				}}
				buf bytes.Buffer
				err error
			)
			data := struct {
				G          *Generator
				ModuleName string
				Name       string
				Decl       adlast.Decl
			}{g, "a.mod", "Name", decl(tc.anns)}
			if err = templates.Gen.ExecuteTemplate(&buf, "registerHelper", data); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
			if tc.spec != "" {
				if _, ok := g.Imports.ByPath(tc.spec); !ok {
					t.Errorf("import %s was not registered", tc.spec)
				}
			}
		})
	}
}
