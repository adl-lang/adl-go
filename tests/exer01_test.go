package out_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"adl_testing/generated/exer01/simple_union"
	"adl_testing/generated/exer01/struct01"

	"github.com/adl-lang/adl-go/adl"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
)

func TestEnum(t *testing.T) {
	x := simple_union.Make_UnionOfVoids_g()
	out := &bytes.Buffer{}
	enc := adl.CreateJsonEncodeBinding(simple_union.Texpr_UnionOfVoids(), adl.RESOLVER)
	err := enc.Encode(out, x)
	if err != nil {
		t.Error(err)
	}
	dec := adl.CreateJsonDecodeBinding(simple_union.Texpr_UnionOfVoids(), adl.RESOLVER)
	var y simple_union.UnionOfVoids
	err = dec.Decode(out, &y)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(x, y) {
		t.Errorf("not equal")
	}
}

func TestUnion(t *testing.T) {
	x := simple_union.Make_UnionOfPrimitives_A(42)
	out := &bytes.Buffer{}
	enc := adl.CreateJsonEncodeBinding(simple_union.Texpr_UnionOfPrimitives(), adl.RESOLVER)
	err := enc.Encode(out, x)
	if err != nil {
		t.Error(err)
	}

	if `{"A":42}` != out.String() {
		t.Error(`{"A":42} != out.String()`)
	}
	dec := adl.CreateJsonDecodeBinding(simple_union.Texpr_UnionOfPrimitives(), adl.RESOLVER)
	var y simple_union.UnionOfPrimitives
	err = dec.Decode(out, &y)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(x, y) {
		t.Errorf("not equal")
	}
}

func TestUnions(t *testing.T) {
	xs := []simple_union.UnionOfPrimitives{
		simple_union.Make_UnionOfPrimitives_A(42),
		simple_union.Make_UnionOfPrimitives_B(41),
		simple_union.Make_UnionOfPrimitives_c(true),
		simple_union.Make_UnionOfPrimitives_d(41.01),
		simple_union.Make_UnionOfPrimitives_e("sdfadf"),
		simple_union.Make_UnionOfPrimitives_f([]string{"a", "b", "v"}),
		simple_union.Make_UnionOfPrimitives_g(),
	}
	out := &bytes.Buffer{}
	te := adlast.Texpr_Vector(simple_union.Texpr_UnionOfPrimitives())
	enc := adl.CreateJsonEncodeBinding(te, adl.RESOLVER)
	enc.Encode(out, xs)
	if out.String() != `[{"A":42},{"B":41},{"c":true},{"d":41.01},{"e":"sdfadf"},{"f":["a","b","v"]},{"g":null}]` {
		t.Error("json str !=")
	}
	dec := adl.CreateJsonDecodeBinding(te, adl.RESOLVER)
	ys := []simple_union.UnionOfPrimitives{}
	err := dec.Decode(out, &ys)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(xs, ys) {
		t.Errorf("not equal")
	}
}

func TestStruct01(t *testing.T) {
	a := "a"
	x := struct01.MakeAll_Struct01(
		41,
		"",
		map[string]any{
			"a": float64(123456),
		},
		[]string{"a", "b", "c"},
		map[string][]string{"a": {"z"}, "b": {"x"}, "sc": {"y"}},
		map[string]map[string]string{},
		map[string]map[string]*string{},
		&a,
		struct01.MakeAll_B("sfd"),
	)

	out := &bytes.Buffer{}
	enc := adl.CreateJsonEncodeBinding[struct01.Struct01](struct01.Texpr_Struct01(), adl.RESOLVER)
	enc.Encode(out, x)
	o1 := out.String()

	dec := adl.CreateJsonDecodeBinding(struct01.Texpr_Struct01(), adl.RESOLVER)
	y := struct01.Struct01{}
	err := dec.Decode(out, &y)
	if err != nil {
		t.Error(err)
	}
	// if !reflect.DeepEqual(x, y) {
	// 	t.Errorf("not equal\n%+v\n%+v\n", x, y)
	// }
	sb := strings.Builder{}
	enc2 := adl.CreateJsonEncodeBinding[struct01.Struct01](struct01.Texpr_Struct01(), adl.RESOLVER)
	enc2.Encode(&sb, x)
	o2 := sb.String()
	if o1 != o2 {
		t.Errorf("json !=")
	}
}
