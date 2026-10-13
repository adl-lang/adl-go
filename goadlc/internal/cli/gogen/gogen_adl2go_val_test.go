package gogen

import (
	"fmt"
	"testing"

	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
)

// The regen-and-diff check does not reach two of these: strRep runs only for
// decls carrying a go_custom_type annotation, and annEntryParams only for a
// non-empty annotations list. Neither occurs in the ADL that the build
// regenerates, so pin them here instead.

func prim(name string) adlast.TypeExpr {
	return adlast.Make_TypeExpr(adlast.Make_TypeRef_primitive(name), []adlast.TypeExpr{})
}

func TestTexprParamsTemplate(t *testing.T) {
	for _, tc := range []struct {
		name string
		te   adlast.TypeExpr
		want string
	}{
		{
			name: "primitive",
			te:   prim("String"),
			want: `adlast.Make_TypeExpr(adlast.Make_TypeRef_primitive("String") , []adlast.TypeExpr{})`,
		},
		{
			name: "reference with one param",
			te: adlast.Make_TypeExpr(
				adlast.Make_TypeRef_reference(adlast.Make_ScopedName("a.mod", "Name")),
				[]adlast.TypeExpr{prim("Int64")},
			),
			want: `adlast.Make_TypeExpr(adlast.Make_TypeRef_reference(adlast.Make_ScopedName("a.mod", "Name")) , ` +
				`[]adlast.TypeExpr{adlast.Make_TypeExpr(adlast.Make_TypeRef_primitive("Int64") , []adlast.TypeExpr{})})`,
		},
		{
			name: "two params are comma separated",
			te: adlast.Make_TypeExpr(
				adlast.Make_TypeRef_primitive("StringMap"),
				[]adlast.TypeExpr{prim("String"), prim("Bool")},
			),
			want: `adlast.Make_TypeExpr(adlast.Make_TypeRef_primitive("StringMap") , ` +
				`[]adlast.TypeExpr{adlast.Make_TypeExpr(adlast.Make_TypeRef_primitive("String") , []adlast.TypeExpr{}),` +
				`adlast.Make_TypeExpr(adlast.Make_TypeRef_primitive("Bool") , []adlast.TypeExpr{})})`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (texprParams{Te: tc.te}).StringRep(); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestAnnEntryParamsTemplate(t *testing.T) {
	val := map[string]any{"key": "value"}
	for _, tc := range []struct {
		name string
		in   annEntryParams
		want string
	}{
		{
			name: "nil value",
			in:   annEntryParams{ModuleName: "a.mod", Name: "Ann", Val: nil},
			want: `adlast.Make_ScopedName("a.mod", "Ann"): nil`,
		},
		{
			name: "go syntax value",
			in:   annEntryParams{ModuleName: "a.mod", Name: "Ann", Val: val},
			want: fmt.Sprintf(`adlast.Make_ScopedName("%s", "%s"): %+#v`, "a.mod", "Ann", val),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.StringRep(); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestAnnMapParamsTemplate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []string
		want    string
	}{
		{"empty", nil, `customtypes.MapMap[adlast.ScopedName, any]{}`},
		{"one", []string{"a"}, `customtypes.MapMap[adlast.ScopedName, any]{a}`},
		{"two", []string{"a", "b"}, `customtypes.MapMap[adlast.ScopedName, any]{a,b}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (annMapParams{Entries: tc.entries}).StringRep(); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestCtorParamsTemplate(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   ctorParams
		want string
	}{
		{"no args", ctorParams{Ctor: "pkg.MakeAll_X[T]"}, "pkg.MakeAll_X[T]()"},
		{"one arg", ctorParams{Ctor: "Make_X_b", Args: []string{"v"}}, "Make_X_b(\nv,\n)"},
		{"two args", ctorParams{Ctor: "MakeAll_X", Args: []string{"a", "b"}}, "MakeAll_X(\na,\nb,\n)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.StringRep(); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// stubSubTask stands in for a real generator sub-task, so that StringRep's
// GoImport side effects have somewhere to go.
type stubSubTask struct{}

func (stubSubTask) GoImport(pkg, curr string, imports *goimports.Imports) (string, error) {
	return pkg + ".", nil
}
func (stubSubTask) ReservedImports() []goimports.ImportSpec { return nil }
func (stubSubTask) IsStdLibGen() bool                       { return false }
func (stubSubTask) GoAdlImportPath() string                 { return "" }

// The regen never reaches custTypeConstructionParams either - it runs only
// for a decl carrying a go_custom_type annotation. These wanted strings were
// taken from the template this method replaced, checked against it for each
// shape below before it was deleted.
func TestCustTypeConstructionStringRep(t *testing.T) {
	const head = "adljson.Unwrap(((*H)(nil)).Construct(\n\t\t&pkg.T{},\n\t\tV,\n\t\t"
	const binder1 = "adl.CreateUncheckedJsonDecodeBinding(\n\t\t\tTE1,\n\t\t\tadl.RESOLVER,\n\t\t).Binder(),\n\t\t"
	const binder2 = "adl.CreateUncheckedJsonDecodeBinding(\n\t\t\tTE2,\n\t\t\tadl.RESOLVER,\n\t\t).Binder(),\n\t\t"
	const tail = "\n\t)).(pkg.T)"

	for _, tc := range []struct {
		name  string
		exprs []string
		want  string
	}{
		{"no type exprs", nil, head + tail},
		{"one type expr", []string{"TE1"}, head + binder1 + tail},
		{"two type exprs", []string{"TE1", "TE2"}, head + binder1 + binder2 + tail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := custTypeConstructionParams{
				G:                &Generator{BaseGen: &BaseGen{Cli: stubSubTask{}}},
				CustomTypeHelper: "H",
				CustomType:       "pkg.T",
				AnyValue:         "V",
				TypeExprStrs:     tc.exprs,
			}
			if got := in.StringRep(); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}
