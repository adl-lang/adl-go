// Package visit is the runtime support for the visitors goadlc generates
// (the govisitor sub-task). Generated Accept methods call TypeCheckMethod
// just before falling through to DefaultAccept.
package visit

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
)

// SkipCheckName is a marker interface that opts a visitor type out of the
// runtime method-signature validation performed by [TypeCheckMethod].
//
//	var _ visit.SkipCheckName = (*MyVisitor)(nil)
//
//	func (*MyVisitor) SkipCheckName() {}
type SkipCheckName interface {
	SkipCheckName()
}

// typeCheckKey identifies one TypeCheckMethod invocation. Successful checks
// are cached per (visitor type, node type, P, R); a mismatch is never
// cached and panics every time.
type typeCheckKey struct {
	visitor reflect.Type
	node    reflect.Type
	payload reflect.Type
	result  reflect.Type
}

var (
	typeCheckMu    sync.RWMutex
	typeCheckCache = map[typeCheckKey]struct{}{}
)

// TypeCheckMethod panics if v has a method named "Visit"+typeName whose
// signature matches none of the eight generated Visitor interfaces for P
// and R. It catches the common mistake of declaring a Visit method with
// the wrong payload or result type, which would otherwise be silently
// skipped by Accept's interface probes.
//
// node is the value being accepted (a *X, or *X[A, B] for a generic decl).
// A Visit method whose node parameter is another instantiation of the
// same generic decl (VisitBox(*Box[Atom], ...) while walking a *Box[string])
// is not a mistake: one visitor type can only have one VisitBox, so that
// instantiation simply falls through to DefaultAccept, and the check
// passes.
//
// A nil v, or one implementing [SkipCheckName], is not checked.
func TypeCheckMethod[P, R any](v any, typeName string, node any) {
	var (
		key      typeCheckKey
		done     bool
		meth     reflect.Method
		exist    bool
		file     string
		line     int
		received string
		nodeType = reflect.TypeOf(node)
	)
	if _, skip := v.(SkipCheckName); skip {
		return
	}
	if v == nil {
		return
	}
	key = typeCheckKey{
		visitor: reflect.TypeOf(v),
		node:    nodeType,
		payload: reflect.TypeFor[P](),
		result:  reflect.TypeFor[R](),
	}
	typeCheckMu.RLock()
	_, done = typeCheckCache[key]
	typeCheckMu.RUnlock()
	if done {
		return
	}
	if meth, exist = reflect.TypeOf(v).MethodByName("Visit" + typeName); exist && !otherInstantiation(meth.Type, nodeType) {
		received = fmt.Sprintf("%v", meth.Type)
		_, file, line, _ = runtime.Caller(2)
		panic(fmt.Sprintf(`Visit%[1]s: found a method by name, but its signature matches none of the Visitor*_%[1]s[%[2]v, %[3]v] interfaces.
  expected one of
    Visit%[1]s(node %[7]v)
    Visit%[1]s(node %[7]v) error
    Visit%[1]s(node %[7]v, payload %[2]v)
    Visit%[1]s(node %[7]v, payload %[2]v) error
    Visit%[1]s(node %[7]v) (result %[3]v)
    Visit%[1]s(node %[7]v) (result %[3]v, err error)
    Visit%[1]s(node %[7]v, payload %[2]v) (result %[3]v)
    Visit%[1]s(node %[7]v, payload %[2]v) (result %[3]v, err error)
  received
    %[4]s
  for the likely call site see
    %[5]s:%[6]d
  Implement visit.SkipCheckName on the visitor to disable this check.`,
			typeName, key.payload, key.result, received, file, line, nodeType))
	}
	typeCheckMu.Lock()
	typeCheckCache[key] = struct{}{}
	typeCheckMu.Unlock()
}

// otherInstantiation reports whether meth (a method type, receiver first)
// takes as its node parameter a different instantiation of the generic
// decl that nodeType is an instantiation of: the same pointer-to-named
// base type, with different type arguments.
func otherInstantiation(meth reflect.Type, nodeType reflect.Type) bool {
	var param reflect.Type
	if meth.NumIn() < 2 {
		return false
	}
	param = meth.In(1)
	if param == nodeType {
		return false
	}
	if param.Kind() != reflect.Pointer || nodeType.Kind() != reflect.Pointer {
		return false
	}
	return genericBase(param.Elem()) != "" && genericBase(param.Elem()) == genericBase(nodeType.Elem())
}

// genericBase is the package path and name of a named generic type with
// its type arguments stripped, or "" for a type that is not an
// instantiation of a generic type.
func genericBase(t reflect.Type) string {
	var (
		name = t.Name()
		cut  = strings.IndexByte(name, '[')
	)
	if cut < 0 {
		return ""
	}
	return t.PkgPath() + "." + name[:cut]
}
