package gogen

import (
	"github.com/adl-lang/adl-go/adl/sys/adlast"
)

// Field decorates adlast.Field with the predicates the templates need.
//
// These would be methods on adlast.Field itself, but adlast is generated
// into the adl module, so goadlc cannot attach methods to it. Wrapping is
// the next best thing: the embedded adlast.Field keeps Name, TypeExpr,
// Annotations and friends reachable, and the predicates below sit alongside
// them rather than being precomputed into a params struct.
type Field struct {
	adlast.Field
}

// WrapFields decorates each of a decl's fields.
func WrapFields(fields []adlast.Field) []Field {
	out := make([]Field, len(fields))
	for i, f := range fields {
		out[i] = Field{f}
	}
	return out
}

// IsVoid reports whether the field's type is the Void primitive.
func (f Field) IsVoid() bool {
	pr, ok := f.TypeExpr.TypeRef.Cast_primitive()
	return ok && pr == "Void"
}

// IsTypeToken reports whether the field's type is the TypeToken primitive.
func (f Field) IsTypeToken() bool {
	pr, ok := f.TypeExpr.TypeRef.Cast_primitive()
	return ok && pr == "TypeToken"
}

// HasDefault reports whether the field declares a default value.
func (f Field) HasDefault() bool {
	_, ok := f.Default.Cast_just()
	return ok
}

// Just is the field's declared default value, or nil when it declares none.
func (f Field) Just() any {
	just, _ := f.Default.Cast_just()
	return just
}
