package gojson_test

import (
	"bytes"
	"math"
	"testing"

	"github.com/adl-lang/adl-go/adl"
	"github.com/adl-lang/adl-go/adl/gojson"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/adl/sys/types"
)

// same holds the runtime's bytes for v to package adl's.
func same[T any](t *testing.T, texpr adlast.ATypeExpr[T], v T, enc gojson.Encoder[T]) {
	t.Helper()
	var (
		want bytes.Buffer
		got  []byte
		err  error
	)
	var binding = adl.CreateJsonEncodeBinding(texpr, adl.RESOLVER)
	if err = binding.Encode(&want, v); err != nil {
		t.Fatal(err)
	}
	if got, err = gojson.Marshal(&v, enc); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want.Bytes(), got) {
		t.Errorf("adl wrote %s, gojson %s", want.Bytes(), got)
	}
}

func TestStrings(t *testing.T) {
	for _, s := range []string{
		"", "plain", `quo"te`, `back\slash`, "tab\tnew\nline\r", "\x00\x1f\x7f",
		"<html>&", "é ✓ 𝄞", "  ", "bad \xff utf8",
	} {
		same(t, adlast.Texpr_String(), s, gojson.EncodeString)
		var (
			w   = gojson.NewWriter()
			got string
		)
		w.String(s)
		if err := gojson.Unmarshal(w.Bytes(), &got, gojson.DecodeString); err != nil {
			t.Fatal(err)
		}
		if want := string(bytes.ToValidUTF8([]byte(s), []byte("�"))); got != want {
			t.Errorf("read %q back as %q", want, got)
		}
	}
}

func TestFloats(t *testing.T) {
	for _, f := range []float64{0, 1, -1.5, 1e-7, 1e21, 123456789.125, math.SmallestNonzeroFloat64, math.MaxFloat64, 0.1} {
		same(t, adlast.Texpr_Double(), f, gojson.EncodeFloat)
		var (
			data, _ = gojson.Marshal(&f, gojson.EncodeFloat)
			got     float64
		)
		if err := gojson.Unmarshal(data, &got, gojson.DecodeFloat); err != nil || got != f {
			t.Errorf("%v read back as %v: %v", f, got, err)
		}
	}
}

func TestMaybeAndPair(t *testing.T) {
	var (
		encMaybe = func(w *gojson.Writer, v *types.Maybe[string]) error {
			return gojson.EncodeMaybe(w, v, gojson.EncodeString)
		}
		decMaybe = func(r *gojson.Reader, v *types.Maybe[string]) error {
			return gojson.DecodeMaybe(r, v, gojson.DecodeString)
		}
		eqMaybe = func(a, b *types.Maybe[string]) bool {
			return gojson.EqualMaybe(a, b, gojson.EqualPrim[string])
		}
	)
	same(t, adl.Texpr_Maybe(adlast.Texpr_String()), types.Make_Maybe_just("x"), encMaybe)
	same(t, adl.Texpr_Maybe(adlast.Texpr_String()), types.Make_Maybe_nothing[string](), encMaybe)
	for _, in := range []string{`"nothing"`, `{"nothing":null}`, `{"just":"x"}`, ` { "just" : "x" } `} {
		var v types.Maybe[string]
		if err := gojson.Unmarshal([]byte(in), &v, decMaybe); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
	for _, in := range []string{`"just"`, `{"other":1}`, `{"just":"x","nothing":null}`, `[]`} {
		var v types.Maybe[string]
		if err := gojson.Unmarshal([]byte(in), &v, decMaybe); err == nil {
			t.Errorf("%s: read with no error", in)
		}
	}
	var a, b = types.Make_Maybe_just("x"), types.Make_Maybe_just("y")
	if eqMaybe(&a, &b) || !eqMaybe(&a, &a) {
		t.Error("EqualMaybe")
	}
	var encPair = func(w *gojson.Writer, v *types.Pair[int64, uint64]) error {
		return gojson.EncodePair(w, v, gojson.EncodeInt64, gojson.EncodeWord64)
	}
	same(t, adl.Texpr_Pair(adlast.Texpr_Int64(), adlast.Texpr_Word64()), types.MakeAll_Pair[int64, uint64](-3, 1<<60), encPair)
}

func TestVectorsMapsNullables(t *testing.T) {
	var (
		encVec = func(w *gojson.Writer, v *[]string) error {
			return gojson.EncodeVector(w, v, gojson.EncodeString)
		}
		encMap = func(w *gojson.Writer, v *map[string]bool) error {
			return gojson.EncodeStringMap(w, v, gojson.EncodeBool)
		}
		// package adl unwraps a pointer at the top, so a Nullable is
		// held to it inside a vector.
		encNull = func(w *gojson.Writer, v *[]*string) error {
			return gojson.EncodeVector(w, v, func(w *gojson.Writer, v **string) error {
				return gojson.EncodeNullable(w, v, gojson.EncodeString)
			})
		}
		s = "s"
	)
	same(t, adlast.Texpr_Vector(adlast.Texpr_String()), []string{"a", "b"}, encVec)
	same(t, adlast.Texpr_Vector(adlast.Texpr_String()), []string{}, encVec)
	same(t, adlast.Texpr_StringMap(adlast.Texpr_Bool()), map[string]bool{"z": true, "a": false, "m": true}, encMap)
	same(t, adlast.Texpr_Vector(adlast.Texpr_Nullable(adlast.Texpr_String())), []*string{&s, nil}, encNull)
}

func TestReaderRefusals(t *testing.T) {
	for _, in := range []string{``, `"open`, `"\x"`, `{"a":1,}`, `[1 2]`, `"a" "b"`, `nul`} {
		var r = gojson.NewReader([]byte(in))
		if err := r.Skip(); err == nil {
			if err = r.End(); err == nil {
				t.Errorf("%q: read with no error", in)
			}
		}
	}
}
