package govisitor

import (
	"strconv"
	"strings"

	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/gogen"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/loader"
)

// visitorBodyParams is the body of a module's _visitor.go file.
type visitorBodyParams struct {
	Decls []visitorDeclParams
}

// visitorDeclParams is one decl that gets a visitor: its eight Visitor
// interfaces, its Accept, and the DefaultAccept its kind's template renders
// (the "default_accept_" tmpl_by_type dispatch on the DeclType branch).
//
// The type-param strings are precomputed here because the payload and
// result params are renamed when they clash with the decl's own ("P" becomes
// "P2"), and because the plain `[P, R any]` spelling is not what
// TypeParam.LSide produces.
type visitorDeclParams struct {
	G    *gogen.Generator
	Decl adlast.Decl
	// Name is the Go type name gotypes gave the decl.
	Name string
	// NodeType is the receiver type: Name with the decl's type args,
	// Box[T].
	NodeType string
	// TypeParams are the decl's own type params.
	TypeParams gogen.TypeParam
	// P and R are the names of the payload and result type params.
	P string
	R string
	// IfaceTP and IfaceTA are the interfaces' type param list and type arg
	// list: the decl's params followed by P and R.
	IfaceTP string
	IfaceTA string
	// MethodTP and MethodTA are the methods' own type param and arg lists:
	// [P, R any] and [P, R].
	MethodTP string
	MethodTA string
	// SkipTypeCheckMethod omits the visit.TypeCheckMethod call from Accept.
	SkipTypeCheckMethod bool
	// Fields are a struct's fields in declaration order.
	Fields []acceptFieldParams
	// Branches are a union's branches in declaration order.
	Branches []acceptBranchParams
	// Underlying is what a newtype's DefaultAccept does with the underlying
	// value, *node; nil for a struct or union.
	Underlying visitNode
}

// UnderlyingDirect is a newtype's underlying visit when it is a plain
// Accept call (the conversion form, see makeDeclParams), which
// DefaultAccept returns directly; nil otherwise.
func (d visitorDeclParams) UnderlyingDirect() *acceptVisit {
	return direct(d.Underlying)
}

// BranchBound reports whether a union's DefaultAccept binds the branch
// value (switch b := node.Branch.(type)): only when some branch is
// descended, since Go rejects a type-switch binding no case uses.
func (d visitorDeclParams) BranchBound() bool {
	for _, b := range d.Branches {
		if b.Descended() {
			return true
		}
	}
	return false
}

// acceptFieldParams is one Accept<Field> method.
type acceptFieldParams struct {
	// Name is the field's Go name: Accept<Name> is the method and node.<Name>
	// the field.
	Name string
	// Visit is what the method does with the field's value.
	Visit visitNode
}

// Descended reports whether the field's value is visited at all, which is
// what decides whether DefaultAccept calls the field's helper. A leaf
// field still gets its Accept<Field> helper.
func (f acceptFieldParams) Descended() bool {
	return descended(f.Visit)
}

// Direct is the field's visit when it is a plain Accept call, which the
// method returns directly; nil otherwise.
func (f acceptFieldParams) Direct() *acceptVisit {
	return direct(f.Visit)
}

// acceptBranchParams is one case of a union's DefaultAccept type switch.
type acceptBranchParams struct {
	// TypeName is the branch struct's Go type name, _<Union>_<Branch>.
	TypeName string
	// Visit is what the case does with the branch value, b.V.
	Visit visitNode
}

// Descended reports whether the branch value is visited at all.
func (b acceptBranchParams) Descended() bool {
	return descended(b.Visit)
}

// Direct is the branch's visit when it is a plain Accept call, which the
// case returns directly; nil otherwise.
func (b acceptBranchParams) Direct() *acceptVisit {
	return direct(b.Visit)
}

func descended(v visitNode) bool {
	var _, leaf = v.(leafVisit)
	return !leaf
}

func direct(v visitNode) *acceptVisit {
	var (
		a  acceptVisit
		ok bool
	)
	if a, ok = v.(acceptVisit); !ok {
		return nil
	}
	return &a
}

// visitNode is one node of the <visit expr as T> recursion of plan4 section
// 4.3: what visiting a value of some type does, after alias expansion. The
// concrete type picks the template (tmpl_by_type). A container whose element
// is a leaf is itself a leaf, so a loop body is never empty.
type visitNode interface {
	// adlType is the ADL type visited, after alias expansion.
	adlType() string
}

// leafVisit does nothing but name the type in a comment.
type leafVisit struct {
	Type string
}

// acceptVisit calls the value's Accept.
type acceptVisit struct {
	// Expr is the Go expression for the value being visited.
	Expr string
	Type string
}

// vectorVisit visits each element by index.
type vectorVisit struct {
	Expr string
	Type string
	// Index is the loop variable: i, i2, ...
	Index string
	Elem  visitNode
}

// nullableVisit visits the pointee when not nil.
type nullableVisit struct {
	Expr string
	Type string
	Elem visitNode
}

// stringmapVisit visits each value in key order, via a copy.
type stringmapVisit struct {
	Expr string
	Type string
	// Key and Val are the loop variables: k, k2, ... and v, v2, ...
	Key  string
	Val  string
	Elem visitNode
}

func (v leafVisit) adlType() string      { return v.Type }
func (v acceptVisit) adlType() string    { return v.Type }
func (v vectorVisit) adlType() string    { return v.Type }
func (v nullableVisit) adlType() string  { return v.Type }
func (v stringmapVisit) adlType() string { return v.Type }

// classifier answers what visiting a type expression does, for one module's
// generation.
type classifier struct {
	lr *loader.LoadResult
	// generated holds the decls that get an Accept.
	generated map[adlast.ScopedName]bool
}

// visit classifies te, as the value expr, with nesting depths of vectors and
// string maps already in scope (they choose the loop variable names).
//
// A reference to a generated decl is an Accept call whatever its type
// args: Box<Atom> is descended, and so is Box<T> inside a generic decl,
// where Go resolves the instantiation. A type parameter itself is a leaf,
// as is a reference to a decl outside the generated set, and a container
// of a leaf is a leaf.
//
// expr is a selector or index expression, except for a newtype's
// underlying value, *node, which the helpers below parenthesise before
// indexing or dereferencing it.
func (c *classifier) visit(expr string, te adlast.TypeExpr, vecs int, maps int) visitNode {
	var (
		prim   string
		ref    adlast.ScopedName
		elem   visitNode
		accept acceptVisit
		ok     bool
	)
	te = gogen.ExpandTypeAliases(c.lr, te)
	if prim, ok = te.TypeRef.Cast_primitive(); ok {
		switch prim {
		case "Vector":
			var index = suffixed("i", vecs)
			elem = c.visit(indexed(expr, index), te.Parameters[0], vecs+1, maps)
			if _, ok = elem.(leafVisit); ok {
				return leafOf(prim, elem)
			}
			return vectorVisit{
				Expr:  expr,
				Type:  prim + "<" + elem.adlType() + ">",
				Index: index,
				Elem:  elem,
			}
		case "Nullable":
			elem = c.visit("(*"+expr+")", te.Parameters[0], vecs, maps)
			if _, ok = elem.(leafVisit); ok {
				return leafOf(prim, elem)
			}
			if accept, ok = elem.(acceptVisit); ok && !strings.HasPrefix(expr, "*") {
				// a method call dereferences the pointer itself; not so
				// through a newtype's defined pointer type, where *node is
				// the Nullable and (**node) the value
				accept.Expr = expr
				elem = accept
			}
			return nullableVisit{
				Expr: expr,
				Type: prim + "<" + elem.adlType() + ">",
				Elem: elem,
			}
		case "StringMap":
			var (
				key = suffixed("k", maps)
				val = suffixed("v", maps)
			)
			elem = c.visit(val, te.Parameters[0], vecs, maps+1)
			if _, ok = elem.(leafVisit); ok {
				return leafOf(prim, elem)
			}
			return stringmapVisit{
				Expr: expr,
				Type: prim + "<" + elem.adlType() + ">",
				Key:  key,
				Val:  val,
				Elem: elem,
			}
		}
		return leafVisit{
			Type: adlTypeString(te),
		}
	}
	if ref, ok = te.TypeRef.Cast_reference(); ok && c.generated[ref] {
		return acceptVisit{
			Expr: expr,
			Type: adlTypeString(te),
		}
	}
	return leafVisit{
		Type: adlTypeString(te),
	}
}

// indexed is expr[index], with a dereference parenthesised first so that
// *node[i] is not read as *(node[i]).
func indexed(expr string, index string) string {
	if strings.HasPrefix(expr, "*") {
		return "(" + expr + ")[" + index + "]"
	}
	return expr + "[" + index + "]"
}

// leafOf is the leaf for a container primitive whose element is a leaf.
func leafOf(prim string, elem visitNode) leafVisit {
	return leafVisit{
		Type: prim + "<" + elem.adlType() + ">",
	}
}

// suffixed is name for depth 0, then name2, name3, ...
func suffixed(name string, depth int) string {
	if depth == 0 {
		return name
	}
	return name + strconv.Itoa(depth+1)
}

// adlTypeString renders te in ADL syntax, references fully scoped.
func adlTypeString(te adlast.TypeExpr) string {
	var (
		head   string
		params []string
	)
	head = adlast.Handle_TypeRef(
		te.TypeRef,
		func(primitive string) string { return primitive },
		func(typeParam string) string { return typeParam },
		func(ref adlast.ScopedName) string { return ref.ModuleName + "." + ref.Name },
		nil,
	)
	if len(te.Parameters) == 0 {
		return head
	}
	for _, p := range te.Parameters {
		params = append(params, adlTypeString(p))
	}
	return head + "<" + strings.Join(params, ", ") + ">"
}

// goPublic is the templates' "public" func: the Go name of an ADL field or
// decl.
func goPublic(s string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// typeParamList renders tp as a type param list, [A, B any], or with its
// constraints when it has any.
func typeParamList(tp gogen.TypeParam) string {
	if len(tp.Params) == 0 {
		return ""
	}
	if len(tp.TypeConstraints) != 0 {
		return tp.LSide()
	}
	return "[" + strings.Join(typeParamNames(tp), ", ") + " any]"
}

// typeArgList renders tp as a type argument list, [A, B].
func typeArgList(tp gogen.TypeParam) string {
	if len(tp.Params) == 0 {
		return ""
	}
	return "[" + strings.Join(typeParamNames(tp), ", ") + "]"
}

func typeParamNames(tp gogen.TypeParam) []string {
	var names = make([]string, len(tp.Params))
	for i, p := range tp.Params {
		names[i] = p.Name
	}
	return names
}

// Indexed is the map element at key, parenthesised when Expr is a
// dereference.
func (v stringmapVisit) Indexed(key string) string {
	return indexed(v.Expr, key)
}
