package customtypes

import (
	"reflect"
	"strconv"

	"github.com/adl-lang/adl-go/adl/adljson"
)

type MapSet[A comparable] map[A]struct{}
type SetHelper struct{}

var _ adljson.CustomTypeHelper = &SetHelper{}

func (*SetHelper) Construct(objPrt any, v any, typeparamDec ...adljson.DecodeFunc) (any, error) {
	newMap := reflect.ValueOf(objPrt).Elem()
	err := setDecodeFunc([]string{"$"}, &newMap, v, typeparamDec...)
	if err != nil {
		return newMap.Interface(), err
	}
	return newMap.Interface(), nil
}

func (*SetHelper) BuildDecodeFunc(typeparamDec ...adljson.DecodeFunc) adljson.DecodeFunc {
	return func(path []string, rval *reflect.Value, v any) error {
		return setDecodeFunc(path, rval, v, typeparamDec...)
	}
}

func setDecodeFunc(path []string, rval *reflect.Value, v any, typeparamDec ...adljson.DecodeFunc) error {
	newM := reflect.MakeMap(rval.Type())
	rv := reflect.ValueOf(v)
	l := rv.Len()
	for i := 0; i < l; i++ {
		v0 := rv.Index(i).Interface()
		rv0 := reflect.New(rval.Type().Key()).Elem()
		path0 := append(path, "["+strconv.Itoa(i)+"]")
		err := typeparamDec[0](path0, &rv0, v0)
		if err != nil {
			return err
		}
		newM.SetMapIndex(rv0, reflect.ValueOf(struct{}{}))
	}
	rval.Set(newM)
	return nil
}

func (*SetHelper) BuildEncodeFunc(typeparamEnc ...adljson.EncoderFunc) adljson.EncoderFunc {
	var f adljson.EncoderFunc = func(e *adljson.EncodeState, v reflect.Value) error {
		e.WriteByte('[')
		iter := v.MapRange()
		rest := false
		for iter.Next() {
			if rest {
				e.WriteByte(',')
			} else {
				rest = true
			}
			k := iter.Key()
			err := typeparamEnc[0](e, k)
			if err != nil {
				return err
			}
		}
		e.WriteByte(']')
		return nil
	}
	return f
}
