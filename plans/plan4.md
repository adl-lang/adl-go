# plan4: `govisitor` - generated visitors for ADL structs and unions

Goal: **a new goadlc sub-task that writes a `<pkg>_visitor.go` beside every
`<pkg>.go` that `gotypes` writes, giving each ADL struct and union the
visitor/`Accept` machinery that `ohm-cli generate go --generic-methods`
gives each grammar rule.**

The model is the ohm-go visitor, in its generic-methods flavour only:
`ohm-cli/docs/visitors.md` sections 3, 5 and 6 (in
`/Users/garymiller/devel/ohmjs/ohm-go`), its templates in
`ohm-cli/ruleast/templates/go_{interfaces,accepts}.tmpl`, and the
hand-converted sample in `sheafdb/samples/goohm/tree_visitor/`. The
struct-generics flavour is **not** supported: the generated code uses
methods with their own type parameters, so a module that consumes it
needs `go 1.27` or later in its `go.mod`. goadlc itself (go 1.26.4) only
emits text and is unaffected.

The generator follows the conventions plan2/plan3 established for the
other sub-tasks: a `cli.govisitor` ADL struct for its params, one
`xxxParams` Go type per `{{define}}`, one `gogen.WriteFile` call per
generated file, imports discovered while rendering.

---

## 1. Where things live

| path | what |
|---|---|
| `goadlc/adl/goadlc/cli/govisitor.adl` | params struct `cli.govisitor.GoVisitor` (section 2) |
| `goadlc/adl/goadlc/cli/gengo.adl` | gains `Nullable<GoVisitor> GoVisitor = null;` |
| `goadlc/internal/cli/govisitor/govisitor.go`, `govisitor_ast.go` | generated from the ADL (`task goadlc:gen_goadlc`) |
| `goadlc/internal/cli/govisitor/govisitor_fn.go` | `Run`, the `gogen.SubTask` methods, params construction |
| `goadlc/internal/cli/govisitor/govisitor_params.go` | the `xxxParams` types the templates render |
| `goadlc/internal/cli/templates/govisitor.tmpl` | the `{{define}}` blocks |
| `goadlc/internal/cli/gengo/gengo_fn.go` | runs `GoVisitor` after `GoTypes` |
| `adl/visit/visit.go` | runtime: `TypeCheckMethod[P, R]`, `SkipCheckName` (section 4.5) |
| `visitor_tests/` | the go 1.27.1 test module (section 6) |
| `goadlc/docs/govisitor.md` | user documentation |

The visitor file is written into the **same package** as the types file,
because `Accept` is a method on the generated struct and Go only allows
methods in the defining package. `GoVisitor.Outputdir` must therefore
equal `GoTypes.Outputdir`.

## 2. Params: `cli.govisitor.GoVisitor`

```
module cli.govisitor {

import cli.root.Root;
import cli.loader.LoadResult;
import cli.gomod.GoModResult;

struct GoVisitor {
    @SerializedName "-"
    Nullable<Root> root = null;
    @SerializedName "-"
    Nullable<LoadResult> loader = null;
    @SerializedName "-"
    Nullable<GoModResult> goMod = null;
    /// Don't run 'go fmt' on the generated files
    Bool NoGoFmt = false;
    /// The path to the Go ADL runtime import; the visitor runtime is <GoAdlPath>/visit
    String GoAdlPath = "github.com/adl-lang/adl-go/adl";
    /// ADL modules to generate visitors for. Empty means every loaded module
    /// (bundle modules such as sys.* are never generated)
    Vector<String> Modules = [];
    /// Omit the visit.TypeCheckMethod call from each Accept
    Bool SkipTypeCheckMethod = false;
    /// output dir; must be the same as GoTypes.Outputdir
    String Outputdir;
};

};
```

The set of modules with visitors is what decides whether a field is
*descended* (section 4.3): a reference to a decl in a module outside the
set is a leaf.

## 3. What is generated, by decl kind

| ADL decl | M1 | M2 | M3 |
|---|---|---|---|
| non-generic `struct` | 8 interfaces, `Accept`, `DefaultAccept`, `Accept<Field>` per field | | |
| non-generic `union` | | 8 interfaces, `Accept`, `DefaultAccept` with a branch type switch | |
| `type` alias | expanded wherever it appears (never generated for) | | |
| `newtype` | leaf | leaf | 8 interfaces, `Accept`, `DefaultAccept` descending into the underlying type |
| generic `struct<T>` / `union<T>` | leaf | leaf | as the non-generic case, with the decl's type params prepended to the interfaces' |
| decl with `go_custom_type` annotation | leaf | leaf | leaf |

"Leaf" means: nothing is generated for the decl, and a field of that type
is visited as a primitive is (section 4.3).

## 4. The generated code, exactly

Written for ADL

```
module tree {
struct Atom      { String name; Bool quoted; };
struct Hierachy  { Atom node; Vector<Node> kids; };
union  Node      { Atom atom; Hierachy hierachy; Word64 index; Void none; };
struct Axis      { Vector<Node> nodes; Nullable<Atom> anchor; StringMap<Atom> named; };
};
```

which `gotypes` renders as `type Atom struct { _Atom }` with
`_Atom{ Name string; Quoted bool }` and `type Node struct { Branch NodeBranch }`
with `_Node_Atom{ V Atom }` etc.

### 4.1 File header

`tree/tree_visitor.go`, package `tree`, standard goadlc header via the
shared `"file"` template. Imports are only what the body used: `visit`
(unless `SkipTypeCheckMethod`), `slices` and `maps` when a `StringMap`
field is descended.

### 4.2 Interfaces: eight per struct or union `X`

Identical to ohm's, with the node parameter a plain `*X`:

```go
type Visitor_Atom[P, R any] interface {
	VisitAtom(node *Atom)
}
type VisitorE_Atom[P, R any] interface {
	VisitAtom(node *Atom) error
}
type VisitorP_Atom[P, R any] interface {
	VisitAtom(node *Atom, payload P)
}
type VisitorPE_Atom[P, R any] interface {
	VisitAtom(node *Atom, payload P) error
}
type VisitorR_Atom[P, R any] interface {
	VisitAtom(node *Atom) (result R)
}
type VisitorRE_Atom[P, R any] interface {
	VisitAtom(node *Atom) (result R, err error)
}
type VisitorPR_Atom[P, R any] interface {
	VisitAtom(node *Atom, payload P) (result R)
}
type VisitorPRE_Atom[P, R any] interface {
	VisitAtom(node *Atom, payload P) (result R, err error)
}
```

The name after `Visit` and after the underscore is the Go type name
(`.Name | public`, via `GoEscape`), the same identifier `gotypes` used.

### 4.3 Struct: `Accept`, `DefaultAccept`, `Accept<Field>`

```go
func (node *Atom) Accept[P, R any](visitor any, payload P) (result R, err error) {
	if v, ok := visitor.(Visitor_Atom[P, R]); ok {
		v.VisitAtom(node)
		return
	}
	if v, ok := visitor.(VisitorE_Atom[P, R]); ok {
		err = v.VisitAtom(node)
		return
	}
	if v, ok := visitor.(VisitorP_Atom[P, R]); ok {
		v.VisitAtom(node, payload)
		return
	}
	if v, ok := visitor.(VisitorPE_Atom[P, R]); ok {
		err = v.VisitAtom(node, payload)
		return
	}
	if v, ok := visitor.(VisitorR_Atom[P, R]); ok {
		result = v.VisitAtom(node)
		return
	}
	if v, ok := visitor.(VisitorRE_Atom[P, R]); ok {
		result, err = v.VisitAtom(node)
		return
	}
	if v, ok := visitor.(VisitorPR_Atom[P, R]); ok {
		result = v.VisitAtom(node, payload)
		return
	}
	if v, ok := visitor.(VisitorPRE_Atom[P, R]); ok {
		result, err = v.VisitAtom(node, payload)
		return
	}
	visit.TypeCheckMethod[P, R](visitor, "Atom", node)
	return node.DefaultAccept[P, R](visitor, payload)
}
```

There is no `this` argument and no `AssertName`: an ADL value carries no
CST node to check against. The order of the eight probes is the table
order; it is what ohm does and the tests rely on it only in that at most
one can match (a Go type has one method of a given name).

`DefaultAccept` calls the helper of each **descended** field in
declaration order and stops at the first error. Leaf fields (section 4.3
table) still get an `Accept<Field>` helper, but `DefaultAccept` does not
call it, so the result is the last *descended* field's result rather than
ohm's "last field": `struct Labelled { Point at; String text; }` returns
`at`'s result, not the zero `R`. A struct with no descended fields returns
the zero `R`. For `Atom`, whose fields are both leaves:

```go
func (node *Atom) DefaultAccept[P, R any](visitor any, payload P) (result R, err error) {
	return
}
```

and for `Hierachy { Atom node; Vector<Node> kids; }` (with `Node` a union,
descended from M2 on):

```go
func (node *Hierachy) DefaultAccept[P, R any](visitor any, payload P) (result R, err error) {
	if result, err = node.AcceptNode[P, R](visitor, payload); err != nil {
		return
	}
	if result, err = node.AcceptKids[P, R](visitor, payload); err != nil {
		return
	}
	return
}
```

One `Accept<Field>` per field, named `Accept` + the field's Go name
(`.Name | public`). What it does depends on the field's type expression
after type aliases are expanded (`goapi.ExpandTypeAliases` - move it to
`gogen` so both sub-tasks share it):

| field type | body |
|---|---|
| primitive other than the three below, type parameter, `Void`, `Json`, `TypeToken`, reference to a leaf decl (section 3) | `// leaf: <type>` then `return` |
| reference to a struct/union in the generated set | `return node.Field.Accept[P, R](visitor, payload)` |
| `Vector<T>` | `for i := range node.Field { <visit node.Field[i] as T> }` then `return` |
| `Nullable<T>` | `if node.Field != nil { <visit (*node.Field) as T> }` then `return` |
| `StringMap<T>` | `for _, k := range slices.Sorted(maps.Keys(node.Field)) { v := node.Field[k]; <visit v as T> }` then `return` |

`<visit expr as T>` is recursive: for a descended reference it is
`if result, err = expr.Accept[P, R](visitor, payload); err != nil { return }`;
for a nested `Vector`/`Nullable`/`StringMap` it is the loop/if above again
with a fresh index name (`i`, `i2`, ...; `k`, `k2`, ...; `v`, `v2`, ...).
A container whose element is (transitively) a leaf is itself a leaf: the
whole field helper is `// leaf: Vector<String>` then `return`, never an
empty loop or an unused `v`. Classification therefore runs bottom-up:
`Vector<Nullable<String>>` is a leaf, `Vector<Nullable<Atom>>` is a
descended vector of nullables. `Nullable<T>` is visited through Go's
automatic dereference, `node.Anchor.Accept[P, R](...)`, never `(*node.Anchor)`. The one place a
`(*expr)` appears is a container or another `Nullable` under a `Nullable`
(`*[]Atom`, `**Atom`), where Go cannot range over or nil-test the pointer
itself: `for i := range *node.Nv { (*node.Nv)[i].Accept... }`. Leaf comments
name the expanded ADL type with references fully scoped
(`// leaf: sys.types.Pair<Int64, Int64>`, `// leaf: Vector<String>`). Elements are reached by index (`node.Kids[i]`)
rather than by range value so that `Accept`'s pointer receiver sees the
real element. A `StringMap` element is not addressable, so it is copied
into `v` first; keys are visited in sorted order so walks are
deterministic. Document both in `govisitor.md`.

So for `Axis`:

```go
func (node *Axis) AcceptNodes[P, R any](visitor any, payload P) (result R, err error) {
	for i := range node.Nodes {
		if result, err = node.Nodes[i].Accept[P, R](visitor, payload); err != nil {
			return
		}
	}
	return
}

func (node *Axis) AcceptAnchor[P, R any](visitor any, payload P) (result R, err error) {
	if node.Anchor != nil {
		if result, err = node.Anchor.Accept[P, R](visitor, payload); err != nil {
			return
		}
	}
	return
}

func (node *Axis) AcceptNamed[P, R any](visitor any, payload P) (result R, err error) {
	for _, k := range slices.Sorted(maps.Keys(node.Named)) {
		v := node.Named[k]
		if result, err = v.Accept[P, R](visitor, payload); err != nil {
			return
		}
	}
	return
}
```

A struct with no fields, or none descended, gets a `DefaultAccept` that just returns.

A reference to a decl in **another generated module** is descended the
same way: `node.Field.Accept[P, R](...)` is a method call on a value whose
type the types file already imported, so the visitor file needs no extra
import for it. Generation is per module, so module A's visitor file is
written without knowing whether B's has been; the `Modules` set is what
makes that consistent.

### 4.4 Union (M2): `Accept`, `DefaultAccept`

Same eight interfaces and the same `Accept` as a struct. `DefaultAccept`
switches on the branch:

```go
func (node *Node) DefaultAccept[P, R any](visitor any, payload P) (result R, err error) {
	switch b := node.Branch.(type) {
	case _Node_Atom:
		return b.V.Accept[P, R](visitor, payload)
	case _Node_Hierachy:
		return b.V.Accept[P, R](visitor, payload)
	case _Node_Index:
		// leaf: Word64
		return
	case _Node_None:
		// leaf: Void
		return
	}
	panic("unhandled branch in : Node")
}
```

The branch type names are the ones `decl_Union` in `gotypes.tmpl` emits:
`_<Union>_<Branch | public>`. `b` is a copy of the branch struct, and
`b.V` is addressable, so the pointer-receiver `Accept` applies. Branch
values of `Vector`/`Nullable`/`StringMap` type use the same recursive
`<visit b.V as T>` as struct fields.

When no branch is descended (every branch a leaf, as in
`union Colour { Void red; Void green; String custom; }`) the switch is
emitted without the binding, `switch node.Branch.(type) {`, because Go
rejects an unused `b`. The cases, their leaf comments and the trailing
`panic` are kept.

There are no per-branch `Visit` interfaces: the branch value is already a
typed node with its own `Visit<Type>`, which is the role ohm's per-case
structs play.

### 4.5 Runtime: `adl/visit`

```go
package visit

// TypeCheckMethod panics if v has a method named "Visit"+typeName whose
// signature matches none of the eight generated interfaces for P and R.
// node is the value being accepted; a Visit method written for another
// instantiation of the same generic decl (VisitBox(*Box[Atom]) while
// walking a *Box[string]) is not an error and falls through to
// DefaultAccept. Successful checks are cached per (visitor type, node
// type, P, R).
func TypeCheckMethod[P, R any](v any, typeName string, node any)

// SkipCheckName opts a visitor type out of TypeCheckMethod.
type SkipCheckName interface{ SkipCheckName() }
```

A port of `ohm/visitor_utils.go`'s `TypeCheckMethod`, `typeCheckKey`,
`typeCheckCache` and `SkipCheckName`, minus the ohm-specific wording. It
lives in the `adl` module (go 1.26.4 is enough; it is a generic function,
not a generic method) so that generated code only ever imports from
`GoAdlPath`. The package is called `visit`, not `visitor`, because the
generated `Accept` has a parameter named `visitor`.

### 4.6 Generics and newtypes (M3)

For `struct Pair<A, B>`, the Go type is `Pair[A, B any]`. Methods on it
are `func (node *Pair[A, B]) Accept[P, R any](...)`; the interfaces are
`Visitor_Pair[A, B, P, R any] interface { VisitPair(node *Pair[A, B]) }`,
with `gogen.TypeParam.AddParams("P", "R")` producing the list (it
renames on clash, so an ADL type param called `P` becomes `P2`... check
what `AddParam` does and make the template use `.Last`-style accessors
rather than hard-coded `P`/`R` if that bites). A field whose type is a
type parameter is a leaf. `TypeCheckMethod` is called with the bare
decl name and the node, which lets it tell instantiations apart.

A newtype `newtype Path = Vector<Atom>;` is `type Path []Atom`; its
`DefaultAccept` is `<visit (*node) as Vector<Atom>>` with `node`
dereferenced. Its interfaces and `Accept` are as for a struct.

## 5. Generator structure

`govisitor_fn.go`:

- `Run`: as `gotypes.Run`, one goroutine per module in the set, each
  calling `gogen.WriteFile` once with `BodyTmpl: "govisitor_body"`.
  Decls are visited in sorted name order (as `gotypes` does), skipping
  the kinds section 3 lists as leaf for the current milestone.
- `SubTask` methods copied from `gotypes`: `ReservedImports` returns
  `slices`, `maps` and `{Path: GoAdlPath + "/visit", Name: "visit"}`;
  `GoImport` is the plain `ByName` lookup; `IsStdLibGen` false;
  `GoAdlImportPath` returns `GoAdlPath`.
- A module's visitor file is written only when the module has at least one
  generated decl; a module of aliases alone gets no file.
- `Run` returns an error if `Modules` names a module that was not loaded
  or that came from a bundle (`sys.*`). It does not validate `Outputdir`
  against `GoTypes.Outputdir` (GoTypes may be absent when only visitors
  are regenerated).
- A `generated map[adlast.ScopedName]bool` (or a func) that answers
  "does this reference get an `Accept`", computed once from the module
  set and the decl kinds, and consulted by the params builders.

`govisitor_params.go`: `bodyParams{Decls []declParams}`, `declParams{G,
Name, TypeParams, Kind, Fields []fieldParams}` or similar, and a small
tree type for the `<visit expr as T>` recursion
(`visitParams{Kind: leaf|accept|vector|nullable|stringmap, Expr string,
Elem *visitParams, Depth int}`). Keep Go for resolution and classification
(which kind a type expression is, after alias expansion) and templates
for layout, per plan3's split.

`govisitor.tmpl`: `govisitor_body`, `visitor_interfaces`,
`visitor_accept`, `struct_default_accept`, `union_default_accept`,
`accept_field`, `visit_te` (recursive).

## 6. The test module: `visitor_tests/`

A separate module, `go 1.27.1`, listed in `go.work` (whose own `go`
line is raised to 1.27.1; module language versions are unaffected):

```
visitor_tests/
  go.mod                 module adl_visitor_tests, go 1.27.1
  Taskfile.local.yml     gen: go run ../goadlc/main.go -cfg ./cfgs/visitor.cfg.json ; test: go test ./...
  cfgs/visitor.cfg.json  Loader over adl/**/*.adl, GoTypes + GoVisitor both to Outputdir "generated"
  adl/<milestone>/...    the ADL inputs
  generated/...          goadlc output, committed
  *_test.go              the tests
```

Root `Taskfile.yml` gains an include for it and `gen`/`test` entries.

Tests are behavioural, not golden-file: they compile the generated code
and run visitors over hand-built values. The sheafdb `tree.adl`
(`sheafdb/samples/goohm/tree.adl`) is a good M2 input, because its
visitor (`ast/build.go`) is what this generator is ultimately for.

## 7. Milestones

Each milestone has an implementer (generator + templates + runtime) and a
tester (ADL inputs, expected behaviour, tests) working concurrently. The
tester writes tests to this spec; where the generator is not ready yet
the tester hand-writes the file the generator must produce for its ADL
input, runs the tests against that, and swaps in the generated file when
it exists. Differences between the two are reported, not silently
resolved on either side.

**M0 - scaffolding** (done before the agents start): this document,
`govisitor.adl`, the `gengo.adl` field, regenerated `internal/cli`, a
`govisitor_fn.go` whose `Run` returns nil, `gengo_fn.go` wiring,
`adl/visit`, the empty `visitor_tests` module and workspace entry.

**M1 - structs**: section 4.2 and 4.3 for non-generic structs, including
`Vector`/`Nullable`/`StringMap` nesting, alias expansion, cross-module
references, leaf references. Tests: visit counting with `P` and `R` flowing
(a depth payload, a count result), the `E` variants stopping the walk at
the first error, `DefaultAccept` fall-through, `TypeCheckMethod` panicking
on a `P`/`R` mismatch and honouring `SkipCheckName`, the same node visited
with two `(P, R)` pairs, `StringMap` order.

**M2 - unions**: section 4.4. Tests over `tree.adl`: a printer
(`any`, `string`) and a counter sharing the one set of nodes; a void and a
primitive branch as leaves.

**M3 - generics and newtypes**: section 4.6. Tests: `struct Box<T>` with a
`T` field and a `Vector<T>` field visited with concrete and leaf `T`;
a newtype over a `Vector` of a struct.

After M3: adopt in `sheafdb/samples/goohm` (`adl.cfg.json` gains a
`GoVisitor` entry and `ast/` gets a visitor over the ADL tree) - a
follow-up, not part of this plan.
