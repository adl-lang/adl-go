package goadl

import (
	"encoding"
	"encoding/base64"
	gojson "encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	. "github.com/adl-lang/adl-go/adl/adljson"
	adlast "github.com/adl-lang/adl-go/adl/sys/adlast"
)

type JsonEncodeBinder[T any] struct {
	binding EncoderFunc
}

type JsonEncodeBinderUncheck struct {
	binding EncoderFunc
}

func CreateJsonEncodeBinding[T any](
	texpr adlast.ATypeExpr[T],
	dres Resolver,
) JsonEncodeBinder[T] {
	binding := buildEncodeBinding(dres, texpr.Value)
	return JsonEncodeBinder[T]{
		binding: binding,
	}
}

func CreateUncheckedJsonEncodeBinding(
	texpr adlast.TypeExpr,
	dres Resolver,
) JsonEncodeBinderUncheck {
	binding := buildEncodeBinding(dres, texpr)
	return JsonEncodeBinderUncheck{
		binding: binding,
	}
}

func unwrap(rv reflect.Value) reflect.Value {
	k := rv.Kind()
	if k == reflect.Pointer {
		return unwrap(rv.Elem())
	}
	if k == reflect.Interface {
		return unwrap(reflect.ValueOf(rv.Interface()))
	}
	return rv
}

func (enc *JsonEncodeBinder[T]) Encode(w io.Writer, v T) error {
	es := &EncodeState{}
	rv := reflect.ValueOf(v)
	rv = unwrap(rv)
	err := enc.binding(es, rv)
	if err != nil {
		return err
	}
	b := es.Bytes()
	_, err = w.Write(b)
	return err
}

func (enc *JsonEncodeBinderUncheck) Encode(w io.Writer, v any) error {
	es := &EncodeState{}
	rv := reflect.ValueOf(v)
	rv = unwrap(rv)
	err := enc.binding(es, rv)
	if err != nil {
		return err
	}
	b := es.Bytes()
	_, err = w.Write(b)
	return err
}

var encoderCache sync.Map // map[reflect.Type]decodeFunc

func texprEncKey(
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

func buildEncodeBinding(
	dres Resolver,
	texpr adlast.TypeExpr,
) EncoderFunc {
	key := texprEncKey(texpr)
	// taken from golang stdlib src/encoding/json/encode.go
	if fi, ok := encoderCache.Load(key); ok {
		// fmt.Printf("encoderCache found %v : %v\n", key, texpr)
		return fi.(EncoderFunc)
	}
	// To deal with recursive types, populate the map with an
	// indirect func before we build it. This type waits on the
	// real func (f) to be ready and then calls it. This indirect
	// func is only used for recursive types.
	var (
		wg sync.WaitGroup
		f  EncoderFunc
	)
	wg.Add(1)
	fi, loaded := encoderCache.LoadOrStore(key, EncoderFunc(func(e *EncodeState, v reflect.Value) error {
		wg.Wait()
		return f(e, v)
	}))
	if loaded {
		return fi.(EncoderFunc)
	}

	// Compute the real encoder and replace the indirect func with it.
	f = buildNewEncodeBinding(dres, texpr)
	wg.Done()
	encoderCache.Store(key, f)
	return f
}

func buildNewEncodeBinding(
	dres Resolver,
	texpr adlast.TypeExpr,
) EncoderFunc {
	return adlast.Handle_TypeRef[EncoderFunc](
		texpr.TypeRef,
		func(primitive string) EncoderFunc {
			return primitiveEncodeBinding(dres, primitive, texpr.Parameters)
		},
		func(typeParam string) EncoderFunc {
			panic(fmt.Errorf("typeParam:%v", typeParam))
		},
		func(reference adlast.ScopedName) EncoderFunc {
			ast := dres.Resolve(reference)
			if ast == nil {
				panic(fmt.Sprintf("can't resolve reference %v", reference))
			}
			tbind := CreateDecBoundTypeParams(TypeParamsFromDecl(ast.Decl), texpr.Parameters)
			// custom types
			if helper, has := dres.ResolveHelper(reference); has {
				typeparamEnc := make([]EncoderFunc, len(texpr.Parameters))
				for i := range texpr.Parameters {
					monoTe, _ := SubstituteTypeBindings(tbind, texpr.Parameters[i])
					typeparamEnc[i] = buildEncodeBinding(dres, monoTe)
				}
				return helper.BuildEncodeFunc(typeparamEnc...)
			}
			return adlast.Handle_DeclType[EncoderFunc](
				ast.Decl.Type_,
				func(struct_ adlast.Struct) EncoderFunc {
					return structEncodeBinding(dres, struct_, tbind)
				},
				func(union_ adlast.Union) EncoderFunc {
					if isEnum(union_) {
						return enumEncodeBinding()
					}
					return unionEncodeBinding(dres, union_, tbind)
				},
				func(type_ adlast.TypeDef) EncoderFunc {
					monoTe, _ := SubstituteTypeBindings(tbind, type_.TypeExpr)
					return buildEncodeBinding(dres, monoTe)
				},
				func(newtype_ adlast.NewType) EncoderFunc {
					monoTe, _ := SubstituteTypeBindings(tbind, newtype_.TypeExpr)
					return buildEncodeBinding(dres, monoTe)
				},
				nil,
			)
		},
		nil,
	)
}

func structEncodeBinding(
	dres Resolver,
	struct_ adlast.Struct,
	tbind []TypeBinding,
) EncoderFunc {
	fieldJB := make([]EncoderFunc, 0)
	for _, field := range struct_.Fields {
		monoTe, _ := SubstituteTypeBindings(tbind, field.TypeExpr)
		jb := buildEncodeBinding(dres, monoTe)
		fieldJB = append(fieldJB, jb)
	}
	return func(e *EncodeState, rval reflect.Value) error {
		rv := rval.Field(0)
		next := byte('{')
		for i := range struct_.Fields {
			f := struct_.Fields[i]
			if f.SerializedName == "-" {
				continue
			}
			fe := fieldJB[i]
			e.WriteByte(next)
			next = ','
			e.WriteString(`"` + f.SerializedName + `":`)
			err := fe(e, rv.Field(i))
			if err != nil {
				return err
			}
		}
		if next == '{' {
			e.WriteString("{}")
		} else {
			e.WriteByte('}')
		}
		return nil
	}
}

func enumEncodeBinding() EncoderFunc {
	return func(e *EncodeState, v reflect.Value) error {
		key := reflect.TypeOf(v.Field(0).Interface()).Field(0).Tag.Get("branch")
		e.WriteString(`"`)
		e.WriteString(string(key))
		e.WriteString(`"`)
		return nil
	}
}

type boundEncField struct {
	EncoderFunc EncoderFunc
	field       adlast.Field
}

func unionEncodeBinding(
	dres Resolver,
	union_ adlast.Union,
	tbind []TypeBinding,
) EncoderFunc {
	encMap := make(map[string]boundEncField)
	for _, f := range union_.Fields {
		monoTe, _ := SubstituteTypeBindings(tbind, f.TypeExpr)
		encMap[f.SerializedName] = boundEncField{
			buildEncodeBinding(dres, monoTe),
			f,
		}
	}

	return func(e *EncodeState, v reflect.Value) error {
		if v.Field(0).IsNil() {
			return fmt.Errorf("cannot encode incomplete value")
		}
		key := reflect.TypeOf(v.Field(0).Interface()).Field(0).Tag.Get("branch")
		e.WriteString(`{"`)
		e.WriteString(string(key))
		e.WriteString(`":`)
		if bf, ok := encMap[key]; ok {
			err := bf.EncoderFunc(e, reflect.ValueOf(v.Field(0).Interface()).Field(0))
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("missing encoding. key: %v", key)
		}
		e.WriteString(`}`)
		return nil
	}
}

func primitiveEncodeBinding(
	dres Resolver,
	ptype string,
	params []adlast.TypeExpr,
) EncoderFunc {
	switch ptype {
	case "TypeToken":
		return func(e *EncodeState, v reflect.Value) error {
			e.WriteString("null")
			return nil
		}
	case "Int8", "Int16", "Int32", "Int64":
		return func(e *EncodeState, v reflect.Value) error {
			b := e.AvailableBuffer()
			b = strconv.AppendInt(b, v.Int(), 10)
			e.Write(b)
			return nil
		}
	case "Word8", "Word16", "Word32", "Word64":
		return func(e *EncodeState, v reflect.Value) error {
			b := e.AvailableBuffer()
			b = strconv.AppendUint(b, v.Uint(), 10)
			e.Write(b)
			return nil
		}
	case "Bool":
		return func(e *EncodeState, v reflect.Value) error {
			b := e.AvailableBuffer()
			b = strconv.AppendBool(b, v.Bool())
			e.Write(b)
			return nil
		}
	case "Float":
		return float64Encoder
	case "Double":
		return float64Encoder
	case "String":
		return func(e *EncodeState, v reflect.Value) error {
			e.Write(appendString(e.AvailableBuffer(), v.String(), false))
			return nil
		}
	case "ByteVector":
		return encodeByteSlice
	case "Void":
		return func(e *EncodeState, v reflect.Value) error {
			e.WriteString("null")
			return nil
		}
	case "Json":
		return func(e *EncodeState, v reflect.Value) error {
			if v.IsZero() {
				e.WriteString("null")
				return nil
			}
			b, _ := gojson.Marshal(v.Interface())
			e.Write(b)
			return nil
		}
	case "Vector":
		elementBinding := buildEncodeBinding(dres, params[0])
		return func(e *EncodeState, v reflect.Value) error {
			e.WriteByte('[')
			n := v.Len()
			for i := 0; i < n; i++ {
				if i > 0 {
					e.WriteByte(',')
				}
				err := elementBinding(e, v.Index(i))
				if err != nil {
					return err
				}
			}
			e.WriteByte(']')
			return nil
		}
	case "StringMap":
		elementBinding := buildEncodeBinding(dres, params[0])
		// TODO depends on struct generated for StringMap
		return func(e *EncodeState, v reflect.Value) error {
			switch v.Kind() {
			// case reflect.Array, reflect.Slice:
			// 	e.WriteByte('{')
			// 	n := v.Len()
			// 	for i := 0; i < n; i++ {
			// 		if i > 0 {
			// 			e.WriteByte(',')
			// 		}
			// 		e.WriteString(`"` + v.String() + `"`)
			// 		e.WriteByte(':')
			// 		err := elementBinding(e, v.Index(i).Field(1))
			// 		if err != nil {
			// 			return err
			// 		}
			// 	}
			// 	e.WriteByte('}')
			// 	return nil
			case reflect.Map:
				en := stringMapEncoder{elemEnc: elementBinding}
				return en.encode(e, v)
			}
			return fmt.Errorf("unable to handle stringmap encoded as a %v", v.Kind())
		}
	case "Nullable":
		elementBinding := buildEncodeBinding(dres, params[0])
		return func(e *EncodeState, v reflect.Value) error {
			if v.IsNil() {
				e.WriteString("null")
				return nil
			}
			// depends on how Nullable is encoded
			return elementBinding(e, v.Elem())
		}
	}
	return nil
}

type floatEncoder int // number of bits

func (bits floatEncoder) encode(e *EncodeState, v reflect.Value) error {
	f := v.Float()
	// if math.IsInf(f, 0) || math.IsNaN(f) {
	// 	e.error(&UnsupportedValueError{v, strconv.FormatFloat(f, 'g', -1, int(bits))})
	// }

	// Convert as if by ES6 number to string conversion.
	// This matches most other JSON generators.
	// See golang.org/issue/6384 and golang.org/issue/14135.
	// Like fmt %g, but the exponent cutoffs are different
	// and exponents themselves are not padded to two digits.
	b := e.AvailableBuffer()
	// b = mayAppendQuote(b, opts.quoted)
	abs := math.Abs(f)
	fmt := byte('f')
	// Note: Must use float32 comparisons for underlying float32 value to get precise cutoffs right.
	if abs != 0 {
		if bits == 64 && (abs < 1e-6 || abs >= 1e21) || bits == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21) {
			fmt = 'e'
		}
	}
	b = strconv.AppendFloat(b, f, fmt, -1, int(bits))
	if fmt == 'e' {
		// clean up e-09 to e-9
		n := len(b)
		if n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	// b = mayAppendQuote(b, opts.quoted)
	e.Write(b)
	return nil
}

var (
	float32Encoder = (floatEncoder(32)).encode
	float64Encoder = (floatEncoder(64)).encode
)

func appendString[Bytes []byte | string](dst []byte, src Bytes, escapeHTML bool) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(src); {
		if b := src[i]; b < utf8.RuneSelf {
			if htmlSafeSet[b] || (!escapeHTML && safeSet[b]) {
				i++
				continue
			}
			dst = append(dst, src[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				// This encodes bytes < 0x20 except for \b, \f, \n, \r and \t.
				// If escapeHTML is set, it also escapes <, >, and &
				// because they can lead to security holes when
				// user-controlled strings are rendered into JSON
				// and served to some browsers.
				dst = append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		// TODO(https://go.dev/issue/56948): Use generic utf8 functionality.
		// For now, cast only a small portion of byte slices to a string
		// so that it can be stack allocated. This slows down []byte slightly
		// due to the extra copy, but keeps string performance roughly the same.
		n := len(src) - i
		if n > utf8.UTFMax {
			n = utf8.UTFMax
		}
		c, size := utf8.DecodeRuneInString(string(src[i : i+n]))
		if c == utf8.RuneError && size == 1 {
			dst = append(dst, src[start:i]...)
			dst = append(dst, `\ufffd`...)
			i += size
			start = i
			continue
		}
		// U+2028 is LINE SEPARATOR.
		// U+2029 is PARAGRAPH SEPARATOR.
		// They are both technically valid characters in JSON strings,
		// but don't work in JSONP, which has to be evaluated as JavaScript,
		// and can lead to security holes there. It is valid JSON to
		// escape them, so we do so unconditionally.
		// See https://en.wikipedia.org/wiki/JSON#Safety.
		if c == '\u2028' || c == '\u2029' {
			dst = append(dst, src[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, src[start:]...)
	dst = append(dst, '"')
	return dst
}

func encodeByteSlice(e *EncodeState, v reflect.Value) error {
	if v.IsNil() {
		e.WriteString("null")
		return nil
	}

	s := v.Bytes()
	b := e.AvailableBuffer()
	b = append(b, '"')
	b = base64.StdEncoding.AppendEncode(b, s)
	b = append(b, '"')
	e.Write(b)
	return nil
}

const hex = "0123456789abcdef"

type stringMapEncoder struct {
	elemEnc EncoderFunc
}

func (me stringMapEncoder) encode(e *EncodeState, v reflect.Value) error {
	if v.IsNil() {
		e.WriteString("{}")
		return nil
	}
	e.WriteByte('{')

	// Extract and sort the keys.
	var (
		sv  = make([]reflectWithString, v.Len())
		mi  = v.MapRange()
		err error
	)
	for i := 0; mi.Next(); i++ {
		if sv[i].ks, err = resolveKeyName(mi.Key()); err != nil {
			// e.error(fmt.Errorf("json: encoding error for type %q: %q", v.Type().String(), err.Error()))
			panic(fmt.Errorf("json: encoding error for type %q: %q", v.Type().String(), err.Error()))

		}
		sv[i].v = mi.Value()
	}
	slices.SortFunc(sv, func(i, j reflectWithString) int {
		return strings.Compare(i.ks, j.ks)
	})

	for i, kv := range sv {
		if i > 0 {
			e.WriteByte(',')
		}
		e.Write(appendString(e.AvailableBuffer(), kv.ks, false))
		e.WriteByte(':')
		me.elemEnc(e, kv.v)
	}
	e.WriteByte('}')
	return nil
}

type reflectWithString struct {
	v  reflect.Value
	ks string
}

func resolveKeyName(k reflect.Value) (string, error) {
	if k.Kind() == reflect.String {
		return k.String(), nil
	}
	if tm, ok := k.Interface().(encoding.TextMarshaler); ok {
		if k.Kind() == reflect.Pointer && k.IsNil() {
			return "", nil
		}
		buf, err := tm.MarshalText()
		return string(buf), err
	}
	switch k.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(k.Uint(), 10), nil
	}
	panic("unexpected map key type")
}

func isEnum(union adlast.Union) bool {
	for _, field := range union.Fields {
		isv := adlast.Handle_TypeRef[bool](
			field.TypeExpr.TypeRef,
			func(primitive string) bool {
				return primitive == "Void"
			},
			func(typeParam string) bool {
				return false
			},
			func(reference adlast.ScopedName) bool {
				return false
			},
			nil,
		)
		if !isv {
			return false
		}
	}
	return true
}
