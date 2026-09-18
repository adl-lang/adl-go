package out_test

import (
	"bytes"
	"testing"

	"adl_testing/exer06/tttest"

	"adl_testing/diff"

	"github.com/adl-lang/adl-go/adl"
)

func TestTypeTokenEncode(t *testing.T) {
	z := tttest.Make_Z()
	enc := adl.CreateJsonEncodeBinding(tttest.Texpr_Z(), adl.RESOLVER)
	buf := bytes.Buffer{}
	err := enc.Encode(&buf, z)
	if err != nil {
		t.Error(err)
	}
	a := buf.String()
	dec := adl.CreateJsonDecodeBinding(tttest.Texpr_Z(), adl.RESOLVER)
	var z2 tttest.Z
	err = dec.Decode(&buf, &z2)
	if err != nil {
		t.Error(err)
	}

	buf2 := bytes.Buffer{}
	err = enc.Encode(&buf2, z2)
	if err != nil {
		t.Error(err)
	}

	// note is it not true that !reflect.DeepEquals(z, z2) since TypeToken is encoded as null

	a2 := buf2.String()
	if a != a2 {
		a := []byte(a)
		a2 := []byte(a2)
		t.Error(string(diff.Diff("z", a, "z2", a2)))
	}
}
