package out_test

import (
	"bytes"
	"testing"

	"adl_testing/exer02/a"
	b2 "adl_testing/exer02/another/b"
	"adl_testing/exer02/b"

	goadl "github.com/adl-lang/adl-go/adl"
)

func TestExec02Encode(t *testing.T) {
	x := a.MakeAll_A(
		b.B{},
		b2.B{},
	)
	out := &bytes.Buffer{}
	enc := goadl.CreateJsonEncodeBinding[a.A](a.Texpr_A(), goadl.RESOLVER)
	enc.Encode(out, x)
	// fmt.Printf("%s\n", string(out.Bytes()))
	// o2, _ := json.Marshal(x)
	// fmt.Printf("%s\n", string(o2))

}
