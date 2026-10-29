package gojson

import (
	"slices"

	"github.com/adl-lang/adl-go/adl/sys/types"
)

// Encoder writes one value of T.
type Encoder[T any] func(w *Writer, v *T) error

// Decoder reads one value of T into v.
type Decoder[T any] func(r *Reader, v *T) error

// Equal is whether two values of T are the same ADL value.
type Equal[T any] func(a, b *T) bool

// Marshal is v as JSON, by enc.
func Marshal[T any](v *T, enc Encoder[T]) ([]byte, error) {
	var w = NewWriter()
	if err := enc(w, v); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// Unmarshal reads data, one value and nothing after it, into v by dec.
func Unmarshal[T any](data []byte, v *T, dec Decoder[T]) error {
	var r = NewReader(data)
	if err := dec(r, v); err != nil {
		return err
	}
	return r.End()
}

// The primitives. ADL's Float and Double are both float64 in Go.

func EncodeString(w *Writer, v *string) error { w.String(*v); return nil }
func EncodeBool(w *Writer, v *bool) error     { w.Bool(*v); return nil }
func EncodeInt8(w *Writer, v *int8) error     { w.Int(int64(*v)); return nil }
func EncodeInt16(w *Writer, v *int16) error   { w.Int(int64(*v)); return nil }
func EncodeInt32(w *Writer, v *int32) error   { w.Int(int64(*v)); return nil }
func EncodeInt64(w *Writer, v *int64) error   { w.Int(*v); return nil }
func EncodeWord8(w *Writer, v *uint8) error   { w.Uint(uint64(*v)); return nil }
func EncodeWord16(w *Writer, v *uint16) error { w.Uint(uint64(*v)); return nil }
func EncodeWord32(w *Writer, v *uint32) error { w.Uint(uint64(*v)); return nil }
func EncodeWord64(w *Writer, v *uint64) error { w.Uint(*v); return nil }
func EncodeFloat(w *Writer, v *float64) error { w.Float(*v); return nil }
func EncodeVoid(w *Writer, v *struct{}) error { w.Null(); return nil }
func EncodeBytes(w *Writer, v *[]byte) error  { w.Bytes64(*v); return nil }

func DecodeString(r *Reader, v *string) error {
	var err error
	*v, err = r.String()
	return err
}

func DecodeBool(r *Reader, v *bool) error {
	var err error
	*v, err = r.Bool()
	return err
}

func DecodeInt8(r *Reader, v *int8) error {
	var n, err = r.Int(8)
	*v = int8(n)
	return err
}

func DecodeInt16(r *Reader, v *int16) error {
	var n, err = r.Int(16)
	*v = int16(n)
	return err
}

func DecodeInt32(r *Reader, v *int32) error {
	var n, err = r.Int(32)
	*v = int32(n)
	return err
}

func DecodeInt64(r *Reader, v *int64) error {
	var err error
	*v, err = r.Int(64)
	return err
}

func DecodeWord8(r *Reader, v *uint8) error {
	var n, err = r.Uint(8)
	*v = uint8(n)
	return err
}

func DecodeWord16(r *Reader, v *uint16) error {
	var n, err = r.Uint(16)
	*v = uint16(n)
	return err
}

func DecodeWord32(r *Reader, v *uint32) error {
	var n, err = r.Uint(32)
	*v = uint32(n)
	return err
}

func DecodeWord64(r *Reader, v *uint64) error {
	var err error
	*v, err = r.Uint(64)
	return err
}

func DecodeFloat(r *Reader, v *float64) error {
	var err error
	*v, err = r.Float()
	return err
}

// DecodeVoid reads any value and keeps none, as package adl does.
func DecodeVoid(r *Reader, v *struct{}) error {
	return r.Skip()
}

func DecodeBytes(r *Reader, v *[]byte) error {
	var err error
	*v, err = r.Bytes64()
	return err
}

// EqualPrim is == over a comparable primitive.
func EqualPrim[T comparable](a, b *T) bool {
	return *a == *b
}

// EqualBytes is whether two byte vectors hold the same bytes.
func EqualBytes(a, b *[]byte) bool {
	return slices.Equal(*a, *b)
}

// EqualVoid is always true: Void has one value.
func EqualVoid(a, b *struct{}) bool {
	return true
}

// Vector<T> is a Go slice.

func EncodeVector[T any](w *Writer, v *[]T, enc Encoder[T]) error {
	var s = *v
	w.Byte('[')
	for i := range s {
		if i > 0 {
			w.Byte(',')
		}
		if err := enc(w, &s[i]); err != nil {
			return err
		}
	}
	w.Byte(']')
	return nil
}

// DecodeVector reads an array; an empty one is an empty slice, not nil,
// as package adl makes it.
func DecodeVector[T any](r *Reader, v *[]T, dec Decoder[T]) error {
	var s = make([]T, 0)
	if err := r.Array(func() error {
		var x T
		if err := dec(r, &x); err != nil {
			return err
		}
		s = append(s, x)
		return nil
	}); err != nil {
		return err
	}
	*v = s
	return nil
}

// EqualVector is element-wise: a nil slice and an empty one are equal,
// since ADL has one empty vector.
func EqualVector[T any](a, b *[]T, eq Equal[T]) bool {
	var x, y = *a, *b
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if !eq(&x[i], &y[i]) {
			return false
		}
	}
	return true
}

// StringMap<T> is a Go map, written with its keys in order.

func EncodeStringMap[T any](w *Writer, v *map[string]T, enc Encoder[T]) error {
	var (
		m    = *v
		keys = make([]string, 0, len(m))
	)
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	w.Byte('{')
	for i, k := range keys {
		if i > 0 {
			w.Byte(',')
		}
		w.Key(k)
		var x = m[k]
		if err := enc(w, &x); err != nil {
			return err
		}
	}
	w.Byte('}')
	return nil
}

func DecodeStringMap[T any](r *Reader, v *map[string]T, dec Decoder[T]) error {
	var m = map[string]T{}
	if err := r.Object(func(key string) error {
		var x T
		if err := dec(r, &x); err != nil {
			return err
		}
		m[key] = x
		return nil
	}); err != nil {
		return err
	}
	*v = m
	return nil
}

func EqualStringMap[T any](a, b *map[string]T, eq Equal[T]) bool {
	var x, y = *a, *b
	if len(x) != len(y) {
		return false
	}
	for k, xv := range x {
		var yv, ok = y[k]
		if !ok || !eq(&xv, &yv) {
			return false
		}
	}
	return true
}

// Nullable<T> is a Go pointer.

func EncodeNullable[T any](w *Writer, v **T, enc Encoder[T]) error {
	if *v == nil {
		w.Null()
		return nil
	}
	return enc(w, *v)
}

func DecodeNullable[T any](r *Reader, v **T, dec Decoder[T]) error {
	if r.Null() {
		*v = nil
		return nil
	}
	var x = new(T)
	if err := dec(r, x); err != nil {
		return err
	}
	*v = x
	return nil
}

func EqualNullable[T any](a, b **T, eq Equal[T]) bool {
	if *a == nil || *b == nil {
		return *a == nil && *b == nil
	}
	return eq(*a, *b)
}

// sys.types.Maybe<T>: a union of nothing, Void, and just, T. It is no
// enum, so nothing is written {"nothing":null}, as package adl writes it.

func EncodeMaybe[T any](w *Writer, v *types.Maybe[T], enc Encoder[T]) error {
	if just, ok := v.Cast_just(); ok {
		w.Raw(`{"just":`)
		if err := enc(w, &just); err != nil {
			return err
		}
		w.Byte('}')
		return nil
	}
	if _, ok := v.Cast_nothing(); ok {
		w.Raw(`{"nothing":null}`)
		return nil
	}
	return Incomplete("sys.types.Maybe")
}

func DecodeMaybe[T any](r *Reader, v *types.Maybe[T], dec Decoder[T]) error {
	var branch, inObject, err = r.Union()
	if err != nil {
		return err
	}
	switch branch {
	case "nothing":
		if inObject {
			if err = r.Skip(); err != nil {
				return err
			}
		}
		*v = types.Make_Maybe_nothing[T]()
	case "just":
		if !inObject {
			return NeedsValue("sys.types.Maybe", branch)
		}
		var x T
		if err = dec(r, &x); err != nil {
			return err
		}
		*v = types.Make_Maybe_just(x)
	default:
		return UnknownBranch("sys.types.Maybe", branch)
	}
	return r.UnionEnd(inObject)
}

func EqualMaybe[T any](a, b *types.Maybe[T], eq Equal[T]) bool {
	var (
		x, xok = a.Cast_just()
		y, yok = b.Cast_just()
	)
	if xok || yok {
		return xok && yok && eq(&x, &y)
	}
	_, xok = a.Cast_nothing()
	_, yok = b.Cast_nothing()
	return xok == yok
}

// sys.types.Pair<T1,T2>: a struct of v1 and v2.

func EncodePair[T1, T2 any](w *Writer, v *types.Pair[T1, T2], enc1 Encoder[T1], enc2 Encoder[T2]) error {
	w.Raw(`{"v1":`)
	if err := enc1(w, &v.V1); err != nil {
		return err
	}
	w.Raw(`,"v2":`)
	if err := enc2(w, &v.V2); err != nil {
		return err
	}
	w.Byte('}')
	return nil
}

func DecodePair[T1, T2 any](r *Reader, v *types.Pair[T1, T2], dec1 Decoder[T1], dec2 Decoder[T2]) error {
	var seen1, seen2 bool
	if err := r.Object(func(key string) error {
		switch key {
		case "v1":
			seen1 = true
			return dec1(r, &v.V1)
		case "v2":
			seen2 = true
			return dec2(r, &v.V2)
		}
		return Unexpected("sys.types.Pair", key)
	}); err != nil {
		return err
	}
	if !seen1 {
		return Missing("sys.types.Pair", "v1")
	}
	if !seen2 {
		return Missing("sys.types.Pair", "v2")
	}
	return nil
}

func EqualPair[T1, T2 any](a, b *types.Pair[T1, T2], eq1 Equal[T1], eq2 Equal[T2]) bool {
	return eq1(&a.V1, &b.V1) && eq2(&a.V2, &b.V2)
}

// sys.types.Either<T1,T2>: a union of left, T1, and right, T2.

func EncodeEither[T1, T2 any](w *Writer, v *types.Either[T1, T2], enc1 Encoder[T1], enc2 Encoder[T2]) error {
	if left, ok := v.Cast_left(); ok {
		w.Raw(`{"left":`)
		if err := enc1(w, &left); err != nil {
			return err
		}
		w.Byte('}')
		return nil
	}
	if right, ok := v.Cast_right(); ok {
		w.Raw(`{"right":`)
		if err := enc2(w, &right); err != nil {
			return err
		}
		w.Byte('}')
		return nil
	}
	return Incomplete("sys.types.Either")
}

func DecodeEither[T1, T2 any](r *Reader, v *types.Either[T1, T2], dec1 Decoder[T1], dec2 Decoder[T2]) error {
	var branch, inObject, err = r.Union()
	if err != nil {
		return err
	}
	if !inObject {
		return NeedsValue("sys.types.Either", branch)
	}
	switch branch {
	case "left":
		var x T1
		if err = dec1(r, &x); err != nil {
			return err
		}
		*v = types.Make_Either_left[T1, T2](x)
	case "right":
		var x T2
		if err = dec2(r, &x); err != nil {
			return err
		}
		*v = types.Make_Either_right[T1, T2](x)
	default:
		return UnknownBranch("sys.types.Either", branch)
	}
	return r.UnionEnd(inObject)
}

func EqualEither[T1, T2 any](a, b *types.Either[T1, T2], eq1 Equal[T1], eq2 Equal[T2]) bool {
	var (
		xl, xlok = a.Cast_left()
		yl, ylok = b.Cast_left()
		xr, xrok = a.Cast_right()
		yr, yrok = b.Cast_right()
	)
	switch {
	case xlok && ylok:
		return eq1(&xl, &yl)
	case xrok && yrok:
		return eq2(&xr, &yr)
	}
	return false
}
