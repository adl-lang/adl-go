package out_test

import (
	"adl_testing/generated/exer01/struct01"
	"bytes"
	"testing"

	"github.com/adl-lang/adl-go/adl"
)

func TestXxx(t *testing.T) {
	a := "a"
	x := struct01.Make_Struct01(
		41,
		"",
		map[string]any{
			"a": 1234567890,
		},
		[]string{"a", "b", "c"},
		map[string][]string{"a": {"z"}, "b": {"x"}, "c": {"y"}},
		map[string]map[string]string{},
		map[string]map[string]*string{},
		&a,
		struct01.Make_B(
			"sfd",
		),
	)

	// v := reflect.ValueOf(x)
	// f0 := v.Field(0)
	// fmt.Printf("%v\n", f0.IsZero())
	// f2 := v.Field(2)
	// fmt.Printf("%v\n", f2.IsZero())

	out := &bytes.Buffer{}
	enc := adl.CreateJsonEncodeBinding[struct01.Struct01](struct01.Texpr_Struct01(), adl.RESOLVER)
	enc.Encode(out, x)
	// fmt.Printf("%s\n", string(out.Bytes()))
}
