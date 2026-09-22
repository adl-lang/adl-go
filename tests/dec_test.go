package out_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"adl_testing/generated/decode/test01"

	"github.com/adl-lang/adl-go/adl"
	"github.com/adl-lang/adl-go/adl/customtypes"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/adl/sys/annotations"
	"github.com/adl-lang/adl-go/adl/sys/types"
)

func TestAnnotations(t *testing.T) {
	a := annotations.MakeAll_SerializedWithInternalTag("sadf")
	enc := adl.CreateJsonEncodeBinding(annotations.Texpr_SerializedWithInternalTag(), adl.RESOLVER)
	buf := bytes.Buffer{}
	enc.Encode(&buf, a)
	if buf.String() != `{"tag":"sadf"}` {
		t.Error()
	}
}

func TestNewTypePrim(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString("42")
	dec := adl.CreateJsonDecodeBinding(test01.Texpr_A(), adl.RESOLVER)
	var y test01.A
	err := dec.Decode(out, &y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if y != 42 {
		t.Fail()
	}
}

func TestStructOfPrim(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"a":42}`)
	dec := adl.CreateJsonDecodeBinding(test01.Texpr_B(), adl.RESOLVER)
	var y test01.B
	err := dec.Decode(out, &y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	y.A = 31
	if !reflect.DeepEqual(y, test01.Make_B(31)) {
		t.Fail()
	}
}

func TestStructOfStruct(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"b": {"a":42}, "c": {"a": 3}}`)
	dec := adl.CreateJsonDecodeBinding(test01.Texpr_C(), adl.RESOLVER)
	var y test01.C
	err := dec.Decode(out, &y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if !reflect.DeepEqual(y, test01.MakeAll_C(test01.Make_B(42), test01.Make_B(3))) {
		t.Fail()
	}
}

func TestTopLevelUnion01(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"a": 42}`)
	dec := adl.CreateJsonDecodeBinding(test01.Texpr_D(), adl.RESOLVER)
	var y test01.D
	err := dec.Decode(out, &y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if !reflect.DeepEqual(y, test01.Make_D_a(42)) {
		t.Fail()
	}
}

func TestTypeCast(t *testing.T) {
	d := &test01.D{}
	if _, ok := any(d).(adl.BranchFactory); ok {
		// fmt.Printf("D implements BranchFactory")
	} else {
		t.Errorf("D doesn't implements BranchFactory")
	}
}

func TestTopLevelUnion02(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"b": {"a":42}}`)
	dec := adl.CreateJsonDecodeBinding(test01.Texpr_D(), adl.RESOLVER)
	var y test01.D
	err := dec.Decode(out, &y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if !reflect.DeepEqual(y, test01.Make_D_b(test01.Make_B(42))) {
		t.Fail()
	}
}

func TestUnionInStruct(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"d":{"b": {"a":42}}}`)
	dec := adl.CreateJsonDecodeBinding(test01.Texpr_E(), adl.RESOLVER)
	var y test01.E
	err := dec.Decode(out, &y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if !reflect.DeepEqual(y, test01.MakeAll_E(test01.Make_D_b(test01.Make_B(42)))) {
		t.Fail()
	}
}

func TestPrims(t *testing.T) {
	_ = t
	x := int64(99)
	_ = x
	p := test01.MakeAll_F(
		1,
		2,
		3,
		4,
		5,
		6,
		7,
		8,
		true,
		1.1,
		1.2,
		"abcd",
		// NOTE the default encoding of a number is a float64
		[]any{nil, false, float64(1), map[string]any{"a": "a", "b": "sadf"}},
		[]int64{1, 2, 3},
		map[string]int64{"a": 1, "b": 2},
		&x,
	)
	buf := bytes.Buffer{}
	enc := adl.CreateJsonEncodeBinding(test01.Texpr_F(), adl.RESOLVER)
	err := enc.Encode(&buf, p)
	if err != nil {
		t.Errorf("%v", err)
	}
	// fmt.Printf("%v\n", buf.String())
	dec := adl.CreateJsonDecodeBinding(test01.Texpr_F(), adl.RESOLVER)
	pIn := test01.F{}
	err = dec.Decode(&buf, &pIn)
	if err != nil {
		t.Errorf("%v", err)
	}
	// fmt.Printf("%+v\n", pIn)
	if !reflect.DeepEqual(p, pIn) {
		t.Errorf(`out != in
pOut:%+#v
pIn :%+#v
`, p, pIn)
	}
	// fmt.Printf("out == in\npOut:%+v\npIn :%+v\n", p, pIn)

	buf2 := bytes.Buffer{}
	enc2 := adl.CreateJsonEncodeBinding(test01.Texpr_F(), adl.RESOLVER)
	err = enc2.Encode(&buf2, p)
	if err != nil {
		t.Errorf("%v", err)
	}
	// fmt.Printf("%v\n", buf2.String())
}

func TestStructRecurse(t *testing.T) {
	out := &bytes.Buffer{}
	out.WriteString(`{"a":[{"a":[]}]}`)
	dec := adl.CreateJsonDecodeBinding(test01.Texpr_G(), adl.RESOLVER)
	var y test01.G
	err := dec.Decode(out, &y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}

	expect := test01.MakeAll_G([]test01.G{
		test01.MakeAll_G([]test01.G{}),
	})
	if !reflect.DeepEqual(y, expect) {
		t.Errorf("expect != received\nexpect  :%+#v\nreceived:%+#v\n", expect, y)
	}

	buf := bytes.Buffer{}
	enc := adl.CreateJsonEncodeBinding(test01.Texpr_G(), adl.RESOLVER)
	err = enc.Encode(&buf, y)
	if err != nil {
		t.Fatalf("err : %v", err)
	}
	if buf.String() != `{"a":[{"a":[]}]}` {
		t.Errorf("expect != received\nreceived%v", buf.String())
	}
}

func TestAdlAst(t *testing.T) {
	_ = t
	cj, err := os.Open("combined.json")
	if err != nil {
		t.Fatalf("can't read combined.json err:%v", err)
	}
	dec := adl.CreateJsonDecodeBinding(adlast.Texpr_StringMap(adl.Texpr_Module()), adl.RESOLVER)
	var ast map[string]adlast.Module
	err = dec.Decode(cj, &ast)
	if err != nil {
		t.Errorf("%v", err)
	}
	// fmt.Printf("%+v\n", ast)
	buf := bytes.Buffer{}
	enc := adl.CreateJsonEncodeBinding(adlast.Texpr_StringMap(adl.Texpr_Module()), adl.RESOLVER)
	err = enc.Encode(&buf, ast)
	if err != nil {
		t.Fatalf("err:%v", err)
	}

	cj.Seek(0, 0)
	d0 := json.NewDecoder(cj)
	d1 := json.NewDecoder(&buf)
	var a0, a1 any
	d0.Decode(&a0)
	d1.Decode(&a1)
	if reflect.DeepEqual(a0, a1) {
		t.Errorf("decode -> encode doesn't produce the same json")
	}

	// ibuf := bytes.Buffer{}
	// err = json.Indent(&ibuf, buf.Bytes(), ``, `    `)
	// if err != nil {
	// 	t.Fatalf("indent err:%v", err)
	// }
	// cj0, err := os.Create("combined_out.json")
	// if err != nil {
	// 	t.Fatalf("can't create combined_out.json err:%v", err)
	// }
	// cj0.Write(ibuf.Bytes())
	// cj0.Close()
}

func TestUnchecked(t *testing.T) {
	t.Run("struct-struct", func(t *testing.T) {
		var x = test01.MakeAll_Int(5)
		var y = test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := adl.CreateUncheckedJsonEncodeBinding(texpr, adl.RESOLVER)
		err := enc.Encode(&buf, x)
		if err != nil {
			t.Fatal(err)
		}

		dec := adl.CreateUncheckedJsonDecodeBinding(texpr, adl.RESOLVER)
		err = dec.Decode(&buf, &y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("&struct-struct", func(t *testing.T) {
		var x = test01.MakeAll_Int(5)
		var y = test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := adl.CreateUncheckedJsonEncodeBinding(texpr, adl.RESOLVER)
		err := enc.Encode(&buf, &x)
		if err != nil {
			t.Fatal(err)
		}

		dec := adl.CreateUncheckedJsonDecodeBinding(texpr, adl.RESOLVER)
		err = dec.Decode(&buf, &y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("struct-any", func(t *testing.T) {
		var x = test01.MakeAll_Int(5)
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := adl.CreateUncheckedJsonEncodeBinding(texpr, adl.RESOLVER)
		err := enc.Encode(&buf, x)
		if err != nil {
			t.Fatal(err)
		}

		dec := adl.CreateUncheckedJsonDecodeBinding(texpr, adl.RESOLVER)
		err = dec.Decode(&buf, y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(&x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("&struct-any", func(t *testing.T) {
		var x = test01.MakeAll_Int(5)
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := adl.CreateUncheckedJsonEncodeBinding(texpr, adl.RESOLVER)
		err := enc.Encode(&buf, &x)
		if err != nil {
			t.Fatal(err)
		}

		dec := adl.CreateUncheckedJsonDecodeBinding(texpr, adl.RESOLVER)
		err = dec.Decode(&buf, y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(&x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("any&-any", func(t *testing.T) {
		var z = test01.MakeAll_Int(5)
		var x any = &z
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := adl.CreateUncheckedJsonEncodeBinding(texpr, adl.RESOLVER)
		err := enc.Encode(&buf, x)
		if err != nil {
			t.Fatal(err)
		}

		dec := adl.CreateUncheckedJsonDecodeBinding(texpr, adl.RESOLVER)
		err = dec.Decode(&buf, y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("any-any", func(t *testing.T) {
		var z = test01.MakeAll_Int(5)
		var x any = z
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := adl.CreateUncheckedJsonEncodeBinding(texpr, adl.RESOLVER)
		err := enc.Encode(&buf, x)
		if err != nil {
			t.Fatal(err)
		}

		dec := adl.CreateUncheckedJsonDecodeBinding(texpr, adl.RESOLVER)
		err = dec.Decode(&buf, y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, reflect.ValueOf(y).Elem().Interface()) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("&any-any", func(t *testing.T) {
		var z = test01.MakeAll_Int(5)
		var x any = z
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := adl.CreateUncheckedJsonEncodeBinding(texpr, adl.RESOLVER)
		err := enc.Encode(&buf, &x)
		if err != nil {
			t.Fatal(err)
		}

		dec := adl.CreateUncheckedJsonDecodeBinding(texpr, adl.RESOLVER)
		err = dec.Decode(&buf, y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, reflect.ValueOf(y).Elem().Interface()) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
	t.Run("&any-&any", func(t *testing.T) {
		var z = test01.MakeAll_Int(5)
		var x any = &z
		var y any = &test01.Int{}
		var texpr = test01.Texpr_Int().Value

		buf := bytes.Buffer{}
		enc := adl.CreateUncheckedJsonEncodeBinding(texpr, adl.RESOLVER)
		err := enc.Encode(&buf, &x)
		if err != nil {
			t.Fatal(err)
		}

		dec := adl.CreateUncheckedJsonDecodeBinding(texpr, adl.RESOLVER)
		err = dec.Decode(&buf, &y)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Errorf("not equal\n%+v\n%+v", x, y)
		}
	})
}

func TestEncDec(t *testing.T) {
	// str := "abc"
	testCases := []struct {
		desc  string
		texpr adlast.TypeExpr
		x     any
		y     any
	}{
		{
			desc:  "Int",
			texpr: test01.Texpr_Int().Value,
			x:     test01.MakeAll_Int(5),
			y:     &test01.Int{},
		},
		{
			desc:  "UInt",
			texpr: test01.Texpr_Uint().Value,
			x:     test01.MakeAll_Uint(5),
			y:     &test01.Uint{},
		},
		{
			desc:  "Bool",
			texpr: test01.Texpr_Bool().Value,
			x:     test01.MakeAll_Bool(true),
			y:     &test01.Bool{},
		},
		{
			desc:  "Unit",
			texpr: test01.Texpr_Unit().Value,
			x:     test01.MakeAll_Unit(),
			y:     &test01.Unit{},
		},
		{
			desc:  "NullableString",
			texpr: test01.Texpr_NullableString().Value,
			x:     test01.MakeAll_NullableString(adl.Addr("abc")),
			y:     &test01.NullableString{},
		},
		{
			desc:  "StringMapString",
			texpr: test01.Texpr_StringMapString().Value,
			x:     test01.MakeAll_StringMapString(map[string]string{"a": "sdf"}),
			y:     &test01.StringMapString{},
		},
		{
			desc:  "VectorString",
			texpr: test01.Texpr_VectorString().Value,
			x:     test01.MakeAll_VectorString([]string{"a", "sdf"}),
			y:     &test01.VectorString{},
		},
		{
			desc:  "Json01",
			texpr: test01.Texpr_Json().Value,
			x:     test01.MakeAll_Json(map[string]any{"a": "sdf"}),
			y:     &test01.Json{},
		},
		{
			desc:  "Json02",
			texpr: test01.Texpr_Json().Value,
			x:     test01.MakeAll_Json([]any{true, float64(1), float64(1.1), nil}),
			y:     &test01.Json{},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			buf := bytes.Buffer{}
			enc := adl.CreateUncheckedJsonEncodeBinding(tC.texpr, adl.RESOLVER)
			err := enc.Encode(&buf, tC.x)
			if err != nil {
				t.Fatal(err)
			}

			dec := adl.CreateUncheckedJsonDecodeBinding(tC.texpr, adl.RESOLVER)
			err = dec.Decode(&buf, tC.y)
			if err != nil {
				t.Fatal(err)
			}
			x := tC.x
			y := reflect.ValueOf(tC.y).Elem().Interface()
			if !reflect.DeepEqual(x, y) {
				t.Errorf("not equal\n%#+v\n%#+v", x, y)
			}
		})
	}
}

func TestSetTest(t *testing.T) {
	st := test01.MakeAll_SetTest(
		customtypes.MapSet[string]{
			"a": {},
			"b": {},
			"c": {},
		},
	)
	// for i := 0; i < 8; i++ {
	// 	buf := bytes.Buffer{}
	// 	enc := adl.CreateJsonEncodeBinding( test01.Texpr_SetTest(), adl.RESOLVER)
	// 	// enc := adl.CreateJsonEncodeBinding( adl.Texpr_Set(adl.Texpr_String()), adl.RESOLVER)
	// 	enc.Encode(st)
	// 	fmt.Printf("%s\n", buf.String())
	// }
	buf := bytes.Buffer{}
	enc := adl.CreateJsonEncodeBinding(test01.Texpr_SetTest(), adl.RESOLVER)
	// enc := adl.CreateJsonEncodeBinding( adl.Texpr_Set(adl.Texpr_String()), adl.RESOLVER)
	enc.Encode(&buf, st)
	dec := adl.CreateJsonDecodeBinding(test01.Texpr_SetTest(), adl.RESOLVER)
	st2 := test01.SetTest{}
	err := dec.Decode(&buf, &st2)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(st, st2) {
		t.Errorf("!=")
	}
	// fmt.Printf("%+#v\n", st2)
}

func TestSetTestDef(t *testing.T) {
	st := test01.SetTest{}
	dec := adl.CreateJsonDecodeBinding(test01.Texpr_SetTest(), adl.RESOLVER)
	sr := strings.NewReader(`{}`)
	err := dec.Decode(sr, &st)
	if err != nil {
		t.Error(err)
	}
	st2 := test01.Make_SetTest()
	if !reflect.DeepEqual(st, st2) {
		t.Errorf("!=\n%#v\n%#v\n", st, st2)
	}
}
func TestHasDefault(t *testing.T) {
	jb := adl.CreateJsonDecodeBinding(test01.Texpr_HasDefault(), adl.RESOLVER)
	sr := strings.NewReader(`{}`)
	x := test01.HasDefault{}
	err := jb.Decode(sr, &x)
	if err != nil {
		t.Error(err)
	}
	x2 := test01.Make_HasDefault()
	if !reflect.DeepEqual(x, x2) {
		t.Errorf("!=")
	}
}

func TestNoDefault(t *testing.T) {
	jb := adl.CreateJsonDecodeBinding(test01.Texpr_NoDefault(), adl.RESOLVER)
	sr := strings.NewReader(`{}`)
	x := test01.NoDefault{}
	err := jb.Decode(sr, &x)
	if err == nil {
		t.Error("Expecting an error, missing not default field")
	}
}

func TestMapTest(t *testing.T) {
	jb := adl.CreateJsonDecodeBinding(test01.Texpr_MapTest(), adl.RESOLVER)
	sr := strings.NewReader(`{}`)
	x := test01.MapTest{}
	err := jb.Decode(sr, &x)
	if err != nil {
		t.Error(err)
	}
	x2 := test01.Make_MapTest()
	if !reflect.DeepEqual(x, x2) {
		t.Errorf("!=")
	}
	x3 := test01.MakeAll_MapTest(
		customtypes.MapMap[string, int64]{"a": 1},
	)
	if !reflect.DeepEqual(x, x3) {
		t.Errorf("!=")
	}
}

func TestGenericF(t *testing.T) {
	x := test01.MakeAll_GenericF[string]("hw")
	ejb := adl.CreateJsonEncodeBinding(test01.Texpr_GenericF(adlast.Texpr_String()), adl.RESOLVER)
	buf := bytes.Buffer{}
	err := ejb.Encode(&buf, x)
	if err != nil {
		t.Error(err)
	}
	djb := adl.CreateJsonDecodeBinding(test01.Texpr_GenericF(adlast.Texpr_String()), adl.RESOLVER)
	dst := test01.GenericF[string]{}
	err = djb.Decode(&buf, &dst)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(x, dst) {
		t.Errorf("!=")
	}

}

func TestMaybe(t *testing.T) {
	enc_bind := adl.CreateJsonEncodeBinding(adl.Texpr_Maybe(adlast.Texpr_String()), adl.RESOLVER)
	src := types.Make_Maybe_nothing[string]()
	buf := bytes.Buffer{}
	err := enc_bind.Encode(&buf, src)
	if err != nil {
		t.Error(err)
	}
	dec_bind := adl.CreateJsonDecodeBinding(adl.Texpr_Maybe(adlast.Texpr_String()), adl.RESOLVER)
	dst := types.Maybe[string]{}
	err = dec_bind.Decode(&buf, &dst)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(src, dst) {
		t.Errorf("!=")
	}
}

func TestZeroMaybe(t *testing.T) {
	fn := func(x types.Maybe[*string]) int {
		return types.Handle_Maybe[*string, int](
			x,
			func(nothing struct{}) int {
				// fmt.Println("nothing")
				return 0
			},
			func(just *string) int {
				// fmt.Println("just")
				return 1
			},
			func() int {
				// fmt.Println("default")
				return 2
			},
		)
	}
	if y := fn(types.Maybe[*string]{}); y != 2 {
		t.Errorf("expected default %v", y)
	}
	if y := fn(types.Make_Maybe_just[*string](nil)); y != 1 {
		t.Errorf("expected just %v", y)
	}
}
