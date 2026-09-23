package gogen

import (
	"fmt"
	"testing"

	"github.com/adl-lang/adl-go/adl/sys/adlast"
)

// The regen-and-diff check does not reach these two templates: strRep runs
// only for decls carrying a go_custom_type annotation, and annEntryParams
// only for a non-empty annotations list. Neither occurs in the ADL that the
// build regenerates, so pin them here instead. The wanted strings are the
// output of the fmt.Sprintf calls these templates replaced.

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
			if got := RenderString("texprParams", texprParams{Te: tc.te}); got != tc.want {
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
			if got := RenderString("annEntryParams", tc.in); got != tc.want {
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
			if got := RenderString("annMapParams", annMapParams{Entries: tc.entries}); got != tc.want {
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
