package out_test

import (
	"bytes"
	"testing"

	"adl_testing/generated/exer02/a"
	b2 "adl_testing/generated/exer02/another/b"
	"adl_testing/generated/exer02/b"

	"github.com/adl-lang/adl-go/adl"
)

func TestExec02Encode(t *testing.T) {
	x := a.MakeAll_A(
		b.B{},
		b2.B{},
	)
	out := &bytes.Buffer{}
	enc := adl.CreateJsonEncodeBinding[a.A](a.Texpr_A(), adl.RESOLVER)
	enc.Encode(out, x)
	// fmt.Printf("%s\n", string(out.Bytes()))
	// o2, _ := json.Marshal(x)
	// fmt.Printf("%s\n", string(o2))

}
