# plan3: the Generator's methods become templates

Goal: **the emission methods on `Generator` and `BaseGen` become `{{define}}` blocks.**
Go keeps resolution, annotation decoding and import bookkeeping; templates take over every
method whose job is to lay out Go source.

This follows [plan2](plan2.md), which put one `ExecuteTemplate` call in charge of each
generated file. That fixed the *file and decl* structure. Below the decl, emission is still
`fmt.Sprintf` in Go: [gogen_adl2go_val.go](goadlc/internal/cli/gogen/gogen_adl2go_val.go)
is 583 lines of format strings reached from four `$G.` calls in
[gotypes.tmpl](goadlc/internal/cli/templates/gotypes.tmpl).

The two gotmpl features that make this possible are already switched on and already used
once each: `tmpl_by_type` dispatches `declParams` to `decl_Struct` / `decl_Union` / ..., and
`WithDynamicScopedVars` is what lets those branch templates see `$P` and `$G`. This plan
applies the same two to the value walk, which is where the Go is.

---

## Inventory

### Moves to templates

Every one of these returns a string that is spliced into generated source, and every one is
reached (directly or through the recursion) from a template. None of them is called by Go
for any purpose other than building that string.

| method | lines | shape |
|---|---|---|
| `Generator.GoRegisterHelper` | 35 | two format strings behind an `IsStdLibGen` test |
| `Generator.GoDeclValue` | 30 | JSON round-trip, then the value walk |
| `Generator.GoTexprValue` | 28 | JSON round-trip, then the value walk |
| `Generator.GoValue` | 20 | entry point + panic context |
| `goval_gen.goValue` | 55 | dispatch on the `TypeRef` branch |
| `goval_gen.goValuePrimitive` | 70 | switch on the primitive's name |
| `goval_gen.goStruct` | 55 | field loop, plus the annotations special case |
| `goval_gen.goUnion` | 60 | branch lookup, then one ctor call |
| `Generator.goCustomType` | 45 | helper lookup, then the construction call |
| `Generator.strRep` + `texprParams.StringRep` | 30 | recursion over a `TypeExpr` |
| `custTypeConstructionParams.StringRep` | 18 | 9 lines of layout with a loop in the middle |
| `ctorParams.StringRep` | 16 | `ctor(\narg,\n)` |
| `annMapParams` / `annEntryParams.StringRep` | 12 | one map literal, one map entry |

The last four are the `StringRep()` methods plan2's N1 created by reverting templates back
into Go. That revert was right at the time - they were leaf fragments called from a Go
recursion, so a template cost a hop and bought nothing. Once the recursion around them is
itself templates, the trade reverses: they become `{{template}}` calls from a template, at
no hop, and the layout sits with the layout.

### Stays in Go

Naming these is half the plan. "Move the methods to templates" is not "move everything".

- **`GoType`, `goType`, `PrimitiveMap`, `gotype_ref_customtype`.** These return a
  `goTypeExpr`, a *structured* value that templates then navigate -
  `{{$type := $G.GoType .TypeExpr .Annotations}}` ... `{{$type.UnionTypeParams.RSide}}`.
  A template can only return text. Making these templates means either rendering
  `goTypeExpr` to a string and losing the parts, or splitting into one template per part,
  each re-running the whole resolution walk. This is computation, not layout.
- **`goTypeExpr.String` / `Complete` / `sansTypeParam`, and the `TypeParam` methods.**
  Called from inside the Go resolution walk above. As templates they would need a
  Go→template call at every use - exactly the `RenderString` hop plan2 removed.
- **`Generator.GoImport`.** Its return value is incidental; its job is the side effect on
  `Imports`. A template cannot register an import.
- **`BaseGen.GoEscape`.** A lookup in a 24-entry map. As a template it is a 24-arm
  `{{if eq}}` ladder.
- **The JSON round-trip** in `GoDeclValue` / `GoTexprValue` (`CreateJsonEncodeBinding`,
  then decode into `any`), the resolver lookups, `CreateDecBoundTypeParams` and
  `SubstituteTypeBindings`. These become methods on the value params type, called by the
  templates to get at the next node - the same arrangement as `declParams.Fields()` today.

### Deleted, not moved

`Generator.ToTitle` and `Generator.JsonEncode` have no callers in Go or in any template.
`Generator.GoEscape` is a byte-for-byte copy of `BaseGen.GoEscape`, which `Generator`
already has by embedding.

---

## The one real obstacle: the recursion is a cycle

`goValue` calls `goStruct`, which calls `goValue` back for each field. There is no order in
which a frame can be converted such that its callees are already templates *and* its callers
are still Go. Either the whole cycle converts in one commit - about 450 lines, too big to
review or to bisect - or one Go→template crossing stays alive for the length of the
migration.

Take the crossing. One unexported helper in `gogen`:

```go
// render executes one template to a string. It is migration scaffolding: the
// value walk spans Go frames and template frames while plan3 is in progress,
// so something has to cross. M4 deletes it, along with the last Go frame.
func render(name string, data any) string
```

This is `RenderString` coming back, and plan2 deleted `RenderString` for good reasons. The
difference is that it comes back with a removal date and one job. M4 removes it because by
then the only Go callers of the value templates are `GoValue`, `GoDeclValue` and
`GoTexprValue`, and those are themselves invoked from `decl_Struct` and `scopedDeclParams`
- so the last two call sites collapse into `{{template "val" ...}}` and nothing crosses.

If M4 lands and `render` still has a caller, the plan has failed at its stated goal; that is
the check, not a matter of taste.

`render` is not the same thing as the `render_each` FuncMap entry M3 adds, though both end
in `Gen.ExecuteTemplate`. `render` is Go driving a walk and reaching into templates to
continue it; that direction is the indirection this plan removes, and it is temporary.
`render_each` is a template rendering its own children so it can sort them; the call starts
and ends inside the template tree, and it stays.

---

## Mechanics

Four things were verified before writing this, because the plan below depends on all four.

**Dispatch on a `TypeRef` branch.** `{{tmpl_by_type .Te.TypeRef.Branch "val" ""}}` invokes
`val_TypeRef_Primitive`, `val_TypeRef_TypeParam` or `val_TypeRef_Reference`. It is `.Branch`
and not `.Branch.V` because `primitive` and `typeParam` are both `string` branches - plan2's
rule. The branch struct is unexported but its `V` field is not, so the dispatched template
reads its payload as `{{.V}}`.

**Ambient state.** `{{$V := .}}` in the dispatching template is visible inside the
dispatched one, whose `dot` is the branch value and not the node. Same for `$G`. This is how
`decl_Struct` already reaches `$P`.

**Dynamic template names must go through a variable.** `{{template .PrimTmpl .}}` is a parse
error (`unexpected ".PrimTmpl" in template clause`); `{{$n := .PrimTmpl}}{{template $n .}}`
works. That form is what dispatches the 15 primitives onto 8 templates, with the
primitive→template map living in Go next to `primitiveMap`.

**A template can sort what it renders**, given two FuncMap entries - `render_each`, which
renders a named template over a slice and returns the strings, and `sorted`, a sorted copy.
`{{range sorted (render_each "annEntry" .Entries)}}` renders each entry, sorts the results
and emits them, which is `sort.Strings` over rendered text with the rendering left in the
template. This is what keeps M3 from having to change a sort key; see
[Two sorts, not one](#two-sorts-not-one).

There is an initialization trap: `render_each` refers to `templates.Gen`, and `Gen`'s
initializer refers to `render_each`, which Go rejects - `initialization cycle for Gen`.
`Gen` therefore becomes a plain `var` assigned in `init()`, where the cycle analysis does
not reach.

### The node type

One params struct carries a node of the value walk, with methods returning the child nodes:

```go
// ValParams is one node of the value walk: Val, an ADL value decoded to any,
// typed by Te, with the annotations in scope.
type ValParams struct {
    G    *Generator
    Anns adlast.Annotations
    Te   adlast.TypeExpr
    Val  any
    st   *valState // path (panic context) and genAdlAst, shared down the walk
}
```

Methods are the Go half: `.Param 0` for an element node, `.Elems` for a vector's, `.Entries`
for a string map's sorted entries, `.Decl` for the resolved decl with its type bindings
applied. Each returns nodes, not strings. The templates do the joining, the commas and the
braces.

---

## Milestones

| | Milestone | Exit criteria |
|---|---|---|
| **M1** | Registration leaf | `GoRegisterHelper` gone; one `importSpecFor` helper replaces 4 copies; `ToTitle` / `JsonEncode` / duplicate `GoEscape` gone |
| **M2** | The value spine | `goValue` and `goValuePrimitive` gone; `ValParams` + `val*` templates land; `render` introduced |
| **M3** | The aggregates | `goStruct`, `goUnion`, `goCustomType`, `strRep` and the four `StringRep` methods gone; `sorted` and `render_each` in the FuncMap; no sort key changed |
| **M4** | Entry points | `GoValue` / `GoDeclValue` / `GoTexprValue` gone; `render` deleted; `gogen_adl2go_val.go` is data preparation only |

Each milestone leaves `git diff` empty after a regen.

### M1 - registration leaf

`GoRegisterHelper` is the one emission method that is already a leaf: the template calls it,
and it calls nothing that has to stay in Go. So it converts with no scaffolding, and it
settles the split the rest of the plan repeats - **the annotation read and the import side
effect stay in Go as methods; the two format strings become one template.**

- `Generator.RegisterHelperName(decl) string` returns the helper's Go name, registering the
  import when the helper lives in another package, and `""` when the decl carries no
  `go_custom_type` annotation.
- `Generator.IsStdLibGen() bool` forwards to `Cli`, so the template can choose between
  `RESOLVER.RegisterHelper` and `adl.RESOLVER.RegisterHelper`.
- A `registerHelper` define holds both forms.

The import-spec construction (`path[LastIndex(path,"/")+1:]`, then an `ImportSpec` with
`Aliased: pkg != last`) appears four times - in `GoCustomTypeSpec`, `GoRegisterHelper`,
`gotype_ref_customtype` and `goCustomType`. One `importSpecFor(importPath, pkg)` replaces
all four. M3 needs it twice more.

Note a behaviour change that is not visible in the output: `GoRegisterHelper` returned the
annotation-decode error and the template aborted on it; `GoCustomTypeAnn`, which replaces
that decode, panics. Both are fatal and both name the decl.

### M2 - the value spine

`goValue` becomes `val` plus three `val_TypeRef_*` defines; `goValuePrimitive` becomes eight
`val_prim_*` defines reached by dynamic name. `goStruct`, `goUnion` and `goCustomType` stay
in Go for this milestone and are reached from `val_TypeRef_Reference` as methods that return
strings; they in turn reach back into the templates through `render`.

The `path []string` that `goval_gen` carries exists only to name the failing field in a
panic message. It survives as `valState`, shared by pointer down the walk, so the panic
context does not get worse as frames move.

`goValuePrimitive`'s `StringMap` case sorts its entries by map key, so that sort stays in
the Go method that hands the template its entries - the key is the key either way, and
nothing changes. It is the *other* sort, in `goStruct`, that needs M3's FuncMap entries.

### M3 - the aggregates

`goStruct`, `goUnion` and `goCustomType` become templates, and with them the four
`StringRep` leaves. After this the walk is template-to-template except at its three entry
points, and `render` has three callers.

`goStruct` carries the one branch worth care: when `genAdlAst` is set and the field is named
`annotations`, it renders the annotation map instead of a value. That flag rides on
`valState`, set by the `GoDeclValue` entry.

That map is also where today's `sort.Strings` over rendered entries lives, so M3 is where
`sorted` and `render_each` join the FuncMap in
[templates.go](goadlc/internal/cli/templates/templates.go) beside `public` and `lower`:

```
{{define "annMap"}}customtypes.MapMap[adlast.ScopedName, any]{
{{- range $i, $e := sorted (render_each "annEntry" .Entries) -}}
{{if $i}},{{end}}{{$e}}
{{- end -}}
}{{end}}
```

The Go method hands over the entries unsorted, in the order the decoded annotations come
in; the template renders each one and sorts the results. That is `sort.Strings(annvs)`
exactly as it reads today, only with the rendering on the template side of the line - so
no key changes and the regen has nothing to catch.

`Gen` becomes a `var` assigned in `init()` at the same time, for the cycle reason under
[Mechanics](#mechanics).

### M4 - entry points

`decl_Struct`'s `{{$G.GoValue .Annotations .TypeExpr .Just}}` and `scopedDeclParams`'s
`{{$G.GoDeclValue .Decl}}` become `{{template "val" ...}}`, with the JSON round-trip exposed
as a method returning the decoded `any`. `render` is deleted. What is left in
`gogen_adl2go_val.go` is the node type and its child-selecting methods.

---

## Hazards

**Whitespace.** Value emission lands inside a file that `WriteFile` runs through
`go/format`, so blank lines and indentation wash out - but only if the file parses. When
`format.Source` fails, `WriteFile` writes the unformatted bytes and prints to stderr, which
shows up as a regen diff rather than a silent pass. Inside a string literal nothing washes
out; `json.Marshal` produces those and its result must stay a single action.

**Coverage.** The regen reaches more of this than
[gogen_adl2go_val_test.go](goadlc/internal/cli/gogen/gogen_adl2go_val_test.go)'s header
claims: custom-type construction and `strRep` are exercised by
`tests/generated/decode/test01/test01.go`, annotation maps by ten `_ast.go` files. One path
is genuinely uncovered - `GoRegisterHelper`'s non-stdlib branch, `adl.RESOLVER.Register-
Helper`, which needs a `go_custom_type` decl outside the stdlib generation. The stdlib
branch is covered (`adl/types_ast.go`).

**The tests pin four methods M3 deletes.** `gogen_adl2go_val_test.go` calls `StringRep` on
`texprParams`, `annEntryParams`, `annMapParams` and `custTypeConstructionParams` directly.
M3 must rewrite them against `templates.Gen.ExecuteTemplate`, not drop them.

---

## Two sorts, not one

The value walk sorts in two places, and they sort different things. `goValuePrimitive`
sorts a string map's entries **by map key**. `goStruct` renders the annotation entries and
then `sort.Strings` the **rendered text**. Conflating them is how this turns into a bug:
move the annotation sort onto `(moduleName, name)` and the generated AST can reorder;
move the string-map sort onto rendered text and it certainly does.

So each keeps what it has. The key sort stays in Go, because the template is handed entries
and the key is the key either way. The text sort moves into the template, because
`render_each` and `sorted` let a template render its children and then order them - which
is the same `sort.Strings` over the same strings, just evaluated on the other side of the
line.

The earlier draft of this plan had the annotation sort switching to a `(moduleName, name)`
key, with an argument that the two orders agree for every input the regen produces. The
argument holds - the rendered line starts with `adlast.Make_ScopedName("<mod>", "<name>")`,
`"` sorts below every character valid in an ADL identifier, and a map cannot hold a key
twice - but it is an argument where there does not need to be one. Ten generated `_ast.go`
files carry non-empty annotation maps, so the regen would have caught a mistake here; not
making one is better.

---

## Verification loop

Unchanged from plan2 - goadlc is self-hosting, so the regen is an exact check:

```sh
task gen                          # gen_texpr, gen_adlast, gen_goadlc, gen_stdlib, gen_exer
task tests:gen_api
git diff --exit-code              # generated output must be byte-identical
cd tests && go test -count=1 ./...
cd goadlc && go test -count=1 ./...
```

`adlc` must be on `PATH`; the flake pins 1.2.3.

Every milestone here must leave `git diff` empty - unlike plan2's N3, none of these changes
the structure of a generated file, only what produces each fragment of it.

Break-testing per milestone is still worth it: rename a define and confirm the failure names
it. Under `WithDynamicScopedVars` the parser no longer checks that a variable is in scope,
so a typo'd `$V` is a nil at execution time rather than a parse error - which makes the
regen diff, not the parse, the thing that catches it.
