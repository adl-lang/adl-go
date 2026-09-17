package adljson

import (
	"bytes"
	"reflect"
)

type CustomTypeHelper interface {
	BuildEncodeFunc(typeparamEnc ...EncoderFunc) EncoderFunc
	BuildDecodeFunc(typeparamDec ...DecodeFunc) DecodeFunc
	Construct(objPrt any, v any, typeparamDec ...DecodeFunc) (any, error)
}

type EncodeState struct {
	bytes.Buffer // accumulated output
}

type EncoderFunc func(e *EncodeState, v reflect.Value) error

// type DecodeState struct {
// 	V    reflect.Value
// 	Path ctxPath
// }

// type ctxPath []string

// func (cp ctxPath) String() string {
// 	return "[" + strings.Join(cp, ",") + "]"
// }

type DecodeFunc func(path []string, rv *reflect.Value, v any) error

func Unwrap[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}
