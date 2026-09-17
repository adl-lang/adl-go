package customtypes

import (
	"reflect"
	"strconv"

	"github.com/adl-lang/adl-go/adl/adljson"
)

type MapMap[K comparable, V any] map[K]V

type MapHelper struct{}

var _ adljson.CustomTypeHelper = &MapHelper{}

func (*MapHelper) Construct(objPrt any, v any, typeparamDec ...adljson.DecodeFunc) (any, error) {
	newMap := reflect.ValueOf(objPrt).Elem()
	err := mapDecodeFunc([]string{"$"}, &newMap, v, typeparamDec...)
	if err != nil {
		return newMap.Interface(), err
	}
	return newMap.Interface(), nil
}

func (*MapHelper) BuildDecodeFunc(typeparamDec ...adljson.DecodeFunc) adljson.DecodeFunc {
	return func(path []string, rval *reflect.Value, v any) error {
		return mapDecodeFunc(path, rval, v, typeparamDec...)
	}
}

func mapDecodeFunc(path []string, rval *reflect.Value, v any, typeparamDec ...adljson.DecodeFunc) error {
	// fmt.Printf("??? %+#v\n", v)
	newM := reflect.MakeMap(rval.Type())
	rv := reflect.ValueOf(v)
	l := rv.Len()
	for i := 0; i < l; i++ {
		mapentry := rv.Index(i).Elem()
		kv := mapentry.MapIndex(reflect.ValueOf("k")).Interface()
		vv := mapentry.MapIndex(reflect.ValueOf("v")).Interface()
		rv0 := reflect.New(rval.Type().Key()).Elem()
		path0 := append(path, "[key "+strconv.Itoa(i)+"]")
		err := typeparamDec[0](path0, &rv0, kv)
		if err != nil {
			return err
		}

		rv1 := reflect.New(rval.Type().Elem()).Elem()
		path1 := append(path, "[val "+strconv.Itoa(i)+"]")
		err = typeparamDec[1](path1, &rv1, vv)
		if err != nil {
			return err
		}

		newM.SetMapIndex(rv0, rv1)
	}
	rval.Set(newM)
	return nil
}

func (*MapHelper) BuildEncodeFunc(typeparamEnc ...adljson.EncoderFunc) adljson.EncoderFunc {
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
			e.WriteByte('{')
			e.WriteString(`"k":`)
			k := iter.Key()
			err := typeparamEnc[0](e, k)
			if err != nil {
				return err
			}
			e.WriteByte(',')
			e.WriteString(`"v":`)
			v := iter.Value()
			err = typeparamEnc[1](e, v)
			if err != nil {
				return err
			}
			e.WriteByte('}')
		}
		e.WriteByte(']')
		return nil
	}
	return f
}
