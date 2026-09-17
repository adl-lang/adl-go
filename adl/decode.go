package goadl

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"

	. "github.com/adl-lang/adl-go/adl/adljson"
	adlast "github.com/adl-lang/adl-go/adl/sys/adlast"
)

type JsonDecodeBinder[T any] struct {
	binding DecodeFunc
}

type JsonDecodeBinderUnchecked struct {
	binding DecodeFunc
}

func CreateJsonDecodeBinding[T any](
	texpr adlast.ATypeExpr[T],
	dres Resolver,
) JsonDecodeBinder[T] {
	return JsonDecodeBinder[T]{
		binding: buildDecodeBinding(dres, texpr.Value),
	}
}

func CreateUncheckedJsonDecodeBinding(
	texpr adlast.TypeExpr,
	dres Resolver,
) JsonDecodeBinderUnchecked {
	return JsonDecodeBinderUnchecked{
		binding: buildDecodeBinding(dres, texpr),
	}
}

func (binder JsonDecodeBinderUnchecked) Binder() DecodeFunc {
	return binder.binding
}

func (jdb *JsonDecodeBinder[T]) Decode(
	r io.Reader,
	dst *T,
) error {
	// for now encode into a Go any and pull pieces out into ADL
	var src any
	jd := json.NewDecoder(r)
	// jd.UseNumber()
	err := jd.Decode(&src)
	if err != nil {
		return err
	}
	return jdb.DecodeFromAny(src, dst)
}

func (jdb *JsonDecodeBinderUnchecked) Decode(
	r io.Reader,
	dst any,
) error {
	// for now encode into a Go any and pull pieces out into ADL
	var src any
	jd := json.NewDecoder(r)
	err := jd.Decode(&src)
	if err != nil {
		return err
	}
	return jdb.DecodeFromAny(src, dst)
}

func (jdb *JsonDecodeBinder[T]) DecodeFromAny(
	src any,
	dst *T,
) error {
	v := unwrap(reflect.ValueOf(dst))
	path := []string{"$"}
	return jdb.binding(path, &v, src)
}

func (jdb *JsonDecodeBinderUnchecked) DecodeFromAny(
	src any,
	dst any,
) error {
	v := unwrap(reflect.ValueOf(dst))
	path := []string{"$"}
	return jdb.binding(path, &v, src)
}

func texprDecKey(
	te adlast.TypeExpr,
) string {
	sb := strings.Builder{}
	sb.Grow(100)
	var recurse func(te adlast.TypeExpr)
	recurse = func(te adlast.TypeExpr) {
		adlast.Handle_TypeRef[*struct{}](
			te.TypeRef,
			func(primitive string) *struct{} {
				sb.WriteString(primitive + ":")
				if len(te.Parameters) == 1 {
					recurse(te.Parameters[0])
				}
				return nil
			},
			func(typeParam string) *struct{} {
				panic(fmt.Errorf("%s", typeParam))
			},
			func(reference adlast.ScopedName) *struct{} {
				sb.WriteString(reference.ModuleName + "." + reference.Name + "::")
				for i := range te.Parameters {
					if i != 0 {
						sb.WriteString(",")
					}
					recurse(te.Parameters[i])
				}
				return nil
			},
			nil,
		)
	}
	recurse(te)
	return sb.String()
}

var decoderCache sync.Map // map[reflect.Type]DecodeFunc

func buildDecodeBinding(
	dres Resolver,
	texpr adlast.TypeExpr,
) DecodeFunc {
	key := texprDecKey(texpr)
	// taken from golang stdlib src/encoding/json/encode.go
	if fi, ok := decoderCache.Load(key); ok {
		return fi.(DecodeFunc)
	}
	// To deal with recursive types, populate the map with an
	// indirect func before we build it. This type waits on the
	// real func (f) to be ready and then calls it. This indirect
	// func is only used for recursive types.
	var (
		wg sync.WaitGroup
		f  DecodeFunc
	)
	wg.Add(1)
	fi, loaded := decoderCache.LoadOrStore(key, DecodeFunc(func(path []string, rval *reflect.Value, v any) error {
		wg.Wait()
		return f(path, rval, v)
	}))
	if loaded {
		return fi.(DecodeFunc)
	}

	// Compute the real encoder and replace the indirect func with it.
	f = buildNewDecodeBinding(dres, texpr)
	wg.Done()
	decoderCache.Store(key, f)
	return f
}

func buildNewDecodeBinding(
	dres Resolver,
	texpr adlast.TypeExpr,
) DecodeFunc {
	return adlast.Handle_TypeRef[DecodeFunc](
		texpr.TypeRef,
		func(primitive string) DecodeFunc {
			return primitiveDecodeBinding(dres, primitive, texpr.Parameters)
		},
		func(typeParam string) DecodeFunc {
			panic(fmt.Errorf("typeParam:%v", typeParam))
		},
		func(reference adlast.ScopedName) DecodeFunc {
			ast := dres.Resolve(reference)
			if ast == nil {
				panic(fmt.Errorf("cannot find %v", reference))
			}
			if ast.Decl.Type_.Branch == nil {
				panic(fmt.Errorf("nil branch %v\n%+v", reference, ast))

			}
			tbind := CreateDecBoundTypeParams(TypeParamsFromDecl(ast.Decl), texpr.Parameters)
			// custom types
			if helper, has := dres.ResolveHelper(reference); has {
				typeparamDec := make([]DecodeFunc, len(texpr.Parameters))
				for i := range texpr.Parameters {
					monoTe, _ := SubstituteTypeBindings(tbind, texpr.Parameters[i])
					typeparamDec[i] = buildDecodeBinding(dres, monoTe)
				}
				return helper.BuildDecodeFunc(typeparamDec...)
			}
			return adlast.Handle_DeclType[DecodeFunc](
				ast.Decl.Type_,
				func(struct_ adlast.Struct) DecodeFunc {
					return structDecodeBinding(dres, struct_, tbind)
				},
				func(union_ adlast.Union) DecodeFunc {
					if isEnum(union_) {
						return enumDecodeBinding(dres, union_)
					}
					return unionDecodeBinding(dres, union_, tbind)
				},
				func(type_ adlast.TypeDef) DecodeFunc {
					monoTe, _ := SubstituteTypeBindings(tbind, type_.TypeExpr)
					return buildDecodeBinding(dres, monoTe)
				},
				func(newtype_ adlast.NewType) DecodeFunc {
					monoTe, _ := SubstituteTypeBindings(tbind, newtype_.TypeExpr)
					// TODO different default values
					return buildDecodeBinding(dres, monoTe)
				},
				nil,
			)
		},
		nil,
	)
}

func primitiveDecodeBinding(
	dres Resolver,
	primitive string,
	typeExpr []adlast.TypeExpr,
) DecodeFunc {
	switch primitive {
	case "TypeToken":
		return func(path []string, rval *reflect.Value, v any) error {
			if v == nil {
				// leave the zero value ATypeExpr[x] as is
				return nil
			}
			return fmt.Errorf("TypeToken can only be null, found %v", v)
		}
	case "Int8", "Int16", "Int32", "Int64",
		"Word8", "Word16", "Word32", "Word64",
		"Bool",
		"Float",
		"Double",
		"String":
		return func(path []string, rval *reflect.Value, v any) error {
			ro := reflect.ValueOf(v)
			if !ro.CanConvert(rval.Type()) {
				return fmt.Errorf("path: %v, received value cannot be convert to expected type. expected %s:%v received type:'%v' val:'%+#v'",
					path, rval.Type(), primitive, ro.Kind(), ro.Interface())
			}
			ro = ro.Convert(rval.Type())
			rval.Set(ro)
			return nil
		}
	// case "ByteVector":
	case "Void":
		return func(path []string, rval *reflect.Value, v any) error {
			return nil
		}
	case "Json":
		return func(path []string, rval *reflect.Value, v any) error {
			if v == nil {
				return nil
			}
			rval.Set(reflect.ValueOf(v))
			return nil
		}
	case "Vector":
		elementBinding := buildDecodeBinding(dres, typeExpr[0])
		return func(path []string, rval *reflect.Value, v any) error {
			rv := reflect.ValueOf(v)
			l := rv.Len()
			newSlice := reflect.MakeSlice(rval.Type(), l, l)
			for i := 0; i < l; i++ {
				rv0 := newSlice.Index(i)
				path0 := append(path, "["+strconv.Itoa(i)+"]")
				err := elementBinding(path0, &rv0, rv.Index(i).Interface())
				if err != nil {
					return err
				}
			}
			rval.Set(newSlice)

			return nil
		}
	case "StringMap":
		elementBinding := buildDecodeBinding(dres, typeExpr[0])
		return func(path []string, rval *reflect.Value, v any) error {
			newM := reflect.MakeMap(rval.Type())
			vT := rval.Type().Elem()
			m := reflect.ValueOf(v)
			iter := m.MapRange()
			for iter.Next() {
				k := iter.Key()
				v0 := iter.Value()
				rv0 := reflect.New(vT).Elem()
				path0 := append(path, k.String()+":")
				err := elementBinding(path0, &rv0, v0.Interface())
				if err != nil {
					return err
				}
				newM.SetMapIndex(k, rv0)
			}
			rval.Set(newM)
			return nil
		}
	case "Nullable":
		elementBinding := buildDecodeBinding(dres, typeExpr[0])
		return func(path []string, rval *reflect.Value, v any) error {
			if v == nil {
				return nil
			}
			var rv0 reflect.Value
			//
			if rval.IsNil() {
				rv0 = reflect.New(rval.Type().Elem()).Elem()
			} else {
				rv0 = rval.Elem()
			}
			err := elementBinding(path, &rv0, v)
			if err != nil {
				return err
			}
			rval.Set(rv0.Addr())
			return nil
		}
	}
	panic("unimplemented")
}

func structDecodeBinding(
	dres Resolver,
	struct_ adlast.Struct,
	tbind []TypeBinding,
) DecodeFunc {
	fieldJB := make([]DecodeFunc, 0, len(struct_.Fields))
	for _, field := range struct_.Fields {
		monoTe, _ := SubstituteTypeBindings(tbind, field.TypeExpr)
		jb := buildDecodeBinding(dres, monoTe)
		fieldJB = append(fieldJB, jb)
	}
	return func(path []string, rval *reflect.Value, val any) error {
		r_impl := rval.Field(0)
		rval = &r_impl
		switch typ := val.(type) {
		case map[string]any:
			usedFld := map[string]struct{}{}
			for i, f := range struct_.Fields {
				if v0, ok := typ[f.SerializedName]; f.SerializedName != "-" && ok {
					usedFld[f.SerializedName] = struct{}{}
					rv0 := rval.Field(i)
					path0 := append(path, f.Name)
					err := fieldJB[i](path0, &rv0, v0)
					if err != nil {
						return err
					}
					continue
				}
				if _, ok := f.Default.Cast_nothing(); ok {
					return fmt.Errorf("path %v, required field missing '%v'", path, f.SerializedName)
				}
				// set from default field value
				rv := reflect.ValueOf(f.Default.Branch).Field(0)
				rv0 := rval.Field(i)
				path0 := append(path, f.Name)
				err := fieldJB[i](path0, &rv0, rv.Interface())
				if err != nil {
					return err
				}
			}
			if len(typ) != len(usedFld) {
				names := []string{}
				for k := range typ {
					if _, ok := usedFld[k]; !ok {
						names = append(names, k)
					}
				}
				return fmt.Errorf("path %v - unexpected fields '%v'", path, names)
			}
			return nil
		}
		panic(fmt.Errorf("path %v, struct: expect an object received %v '%v'", path, reflect.TypeOf(val), val))
	}
}

func enumDecodeBinding(
	dres Resolver,
	union_ adlast.Union,
) DecodeFunc {
	decMap := make(map[string]DecodeFunc)
	for _, f := range union_.Fields {
		if f.SerializedName == "-" {
			continue
		}
		bf := buildDecodeBinding(dres, adlast.Texpr_Void().Value)
		if bf == nil {
			panic(fmt.Errorf("DecodeFunc == nil - %#+v", f.TypeExpr))
		}
		decMap[f.SerializedName] = bf
	}
	return func(path []string, rval *reflect.Value, v any) error {
		var (
			key string
			val any
		)
		switch t := v.(type) {
		case string:
			key = t
			val = nil
		case map[string]any:
			if len(t) != 1 {
				return fmt.Errorf("path: %v, expect an object with one and only element received %v", path, len(t))
			}
			for k0, v0 := range t {
				key = k0
				val = v0
			}
		default:
			return fmt.Errorf("path: %v, union: expect an object received %v '%v'", path, reflect.TypeOf(v), v)
		}

		if bf, ok := decMap[key]; ok {
			var (
				vn      reflect.Value
				factory BranchFactory
				ok      bool
			)
			if rval.CanAddr() {
				factory, ok = rval.Addr().Interface().(BranchFactory)
			}
			if ok {
				branch, err := factory.MakeNewBranch(key)
				if err != nil {
					return fmt.Errorf("path: %v, unexpected branch - no type in branch factory '%v'", path, key)
				}
				vn = reflect.ValueOf(branch)
			} else {
				return fmt.Errorf("path: %v, MakeNewBranch not implemented '%v'", path, rval.Type())
			}
			rv0 := vn.Elem().Field(0)
			path0 := append(path, key)
			err := bf(path0, &rv0, val)
			if err != nil {
				return err
			}
			r0 := *rval // for top level Elem() is already called
			r0 = r0.Field(0)
			// r0 = r0.Field(0)
			r0.Set(vn.Elem())
			return nil
		} else {
			return fmt.Errorf("path: %v, unexpected branch '%v'", path, key)
		}
	}
}

func unionDecodeBinding(
	dres Resolver,
	union_ adlast.Union,
	tbind []TypeBinding,
) DecodeFunc {
	decMap := make(map[string]DecodeFunc)
	for _, f := range union_.Fields {
		if f.SerializedName == "-" {
			continue
		}
		monoTe, _ := SubstituteTypeBindings(tbind, f.TypeExpr)
		bf := buildDecodeBinding(dres, monoTe)
		if bf == nil {
			panic(fmt.Errorf("DecodeFunc == nil - %#+v", f.TypeExpr))
		}
		decMap[f.SerializedName] = bf
	}
	return func(path []string, rval *reflect.Value, v any) error {
		var (
			key string
			val any
		)

		switch t := v.(type) {
		case string:
			key = t
			val = nil
		case map[string]any:
			if len(t) != 1 {
				return fmt.Errorf("path: %v, expect an object with one and only element received %v", path, len(t))
			}
			for k0, v0 := range t {
				key = k0
				val = v0
			}
		default:
			return fmt.Errorf("path: %v, union: expect an object received %v '%v'", path, reflect.TypeOf(v), v)
		}

		if bf, ok := decMap[key]; ok {
			var (
				vn      reflect.Value
				factory BranchFactory
				ok      bool
			)
			if rval.CanAddr() {
				factory, ok = rval.Addr().Interface().(BranchFactory)
			}
			if ok {
				branch, err := factory.MakeNewBranch(key)
				if err != nil {
					return fmt.Errorf("path: %v, unexpected branch - no type in branch factory '%v'", path, key)
				}
				vn = reflect.ValueOf(branch)
			} else {
				return fmt.Errorf("path: %v, MakeNewBranch not implemented '%v'", path, rval.Type())
			}
			rv0 := vn.Elem().Field(0)
			path0 := append(path, key)
			err := bf(path0, &rv0, val)
			if err != nil {
				return err
			}
			r0 := *rval // for top level Elem() is already called
			r0 = r0.Field(0)
			// r0 = r0.Field(0)
			r0.Set(vn.Elem())
			return nil
		} else {
			return fmt.Errorf("path: %v, unexpected branch '%v'", path, key)
		}

	}
}
