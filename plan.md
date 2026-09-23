# Simplification plan: `goadlc`

Scope: the `goadlc` code generator (`goadlc/internal/`), measured against the
`ohm-cli/ruleast` generator in `ohmjs/ohm-go` as a worked reference for the same problem
shape - walk an ADL/AST tree, emit Go.

## The core finding

**`goadlc` imported `github.com/millergarym/gotmpl/text/template` and used none of its
four extensions** - no `tmpl_by_type`, no dynamic scoping, no dynamic template names, no
void FuncMap funcs. The dependency was a drop-in for stock `text/template` and bought
nothing.

Worse, `goadlc` had **re-implemented `tmpl_by_type` in Go** - `TemplateRenderer.Render`
reflected on the params type name and stripped `"Params"` off it to pick a template. So
dispatch lived in Go, and every dispatch point needed a Go call site plus a bespoke
struct. That is where the 20 `xxxParams` structs come from.

`ruleast` has zero params structs. The ADL-generated AST node *is* the template data;
ambient state rides on dynamic-scoped vars; dispatch is
`{{tmpl_by_type .Branch.V "" "_Suffix"}}`.

**The dispatch half of that finding is now fixed** (M0 below). The data half - the params
structs, and the `fmt.Sprintf` emission they feed - is not.

---

## Milestones

| | Milestone | Exit criteria | Size |
|---|---|---|---|
| **M0** | **Real dispatch, one template set** | `tmpl_by_type` does the dispatch; one parsed set; regen byte-identical | **done** |
| **M1** | Helper methods on the ADL AST types | `fieldParams`' derived fields exist as methods; no behaviour change | ~60 lines |
| **M2** | Collapse the decl params structs | `generalDeclV3`'s `Handle_DeclType` ladder gone; 4 structs become 1 | ~150 lines |
| **M3** | Emission moves into templates | `goStruct` / `goUnion` / `goValuePrimitive` / `strRep` are templates | ~350 lines |

M1 is a strict prerequisite for M2, and M2 for M3. Each milestone must leave
`git diff` empty after a regen (see [Verification loop](#verification-loop)).

### M0 - what landed

- The 19 loose template files under three per-package `templates/` directories became
  three `.tmpl` files of `{{define}}` blocks in
  [goadlc/internal/cli/templates/](goadlc/internal/cli/templates/): `gengo.tmpl`,
  `gotypes.tmpl`, `genapi.tmpl` - the `ruleast` layout.
- One parsed set and one FuncMap, in the new leaf package
  [templates.go](goadlc/internal/cli/templates/templates.go), with
  `WithDynamicScopedVars()` enabled. A template in one file can now invoke a template in
  another.
- `TemplateRenderer.Render`'s reflect-and-strip shim is replaced by gotmpl's real
  `tmpl_by_type`, in the `render` define. Each define is named after the Go `xxxParams`
  type it renders, so the mapping is stated rather than encoded as a hidden
  `len("Params")` slice.
- `TemplateRenderer` no longer carries a `*template.Template`; the uncalled
  `RenderTemplate` is gone; the throwaway `Generator` in `goCustomType` (built only to get
  a fresh buffer) is now `gogen.RenderString`; the two duplicate `public()` copies are
  gone; `*_embed.go` became `*_params.go`.

Net **-126/+84** lines of Go, with generated output byte-identical.

Separately, the dead code found during planning - `internal/diff`, `internal/root`,
`root.Config`, the unreachable `capget` / `cappost` templates - has been removed.

### Where that leaves the ratio

| | templates | hand-written generation Go |
|---|---|---|
| `ruleast` | 704 lines | ~500 lines |
| `goadlc` **now** | 293 lines | 2118 lines |

M0 was structural, not volumetric: it fixed *how* dispatch happens without moving any
emission out of Go. M1-M3 are what close the gap.

---

## 1. Helper methods on the ADL AST types (M1)

`fieldParams` ([gotypes_params.go:39-47](goadlc/internal/cli/gotypes/gotypes_params.go#L39-L47))
embeds `adlast.Field` and bolts on four derived values:

```go
type fieldParams struct {
	adlast.Field
	DeclName   string
	G          *gogen.Generator
	HasDefault bool
	Just       any
	IsVoid     bool
}
```

`HasDefault`, `Just` and `IsVoid` are computed by
[makeFieldParam](goadlc/internal/cli/gotypes/gotypes_fn.go#L279) on every field of every
decl, purely so a stock template can reach them. Make them methods instead, in a `gogen`
helpers file - not in generated `adlast` code:

- `func (f Field) IsVoid() bool` - the `Cast_primitive() == "Void"` test
- `func (f Field) HasDefault() bool` / `func (f Field) Just() any` - the
  `types.Handle_Maybe` split
- `func (s Struct) ContainsTypeToken() bool` -
  [containsTypeToken](goadlc/internal/cli/gotypes/gotypes_fn.go#L315-L322) already has
  the body

`gogen.TypeParam`
([gogen_type_params.go:65-158](goadlc/internal/cli/gogen/gogen_type_params.go#L65-L158))
is already this pattern done right - `LSide`, `RSide`, `TexprArgs`, `Last` - and
`ruleast` uses it throughout (`IsNode`, `GoType`, `Descr`, `RuleName`). Extend it rather
than inventing a parallel struct.

This milestone changes no templates and no output; it only moves the derivations. Land it
on its own so M2's diff is readable.

## 2. Collapse the decl params structs (M2)

With M1's methods in place, dispatch can move off the params struct and onto the AST.
`adlast.DeclType` branch payloads are `adlast.Struct` / `adlast.Union` / `adlast.TypeDef`
/ `adlast.NewType`, all named types, so:

```
{{tmpl_by_type .Type_.Branch.V "decl_" ""}}
```

dispatches to `decl_Struct`, `decl_Union`, `decl_TypeDef`, `decl_NewType`. That removes
the four-closure `adlast.Handle_DeclType` ladder in
[generalDeclV3](goadlc/internal/cli/gotypes/gotypes_fn.go#L164-L223) and the four
`Rr.Render(...)` call sites, and `structParams` / `unionParams` / `typeAliasParams` /
`newTypeParams` collapse into one `declParams`.

Inside `decl_Struct`, `dot` is the `adlast.Struct`, so `.Fields` is `[]adlast.Field` -
which is why M1 has to land first. Everything else the template needs (`$G`, the decl
name, the `TypeParams`) comes from dynamic-scoped vars bound once above the dispatch:

```
{{define "declParams"}}{{$G := .G}}{{$name := .Name}}{{$TypeParams := .TypeParams -}}
{{tmpl_by_type .Decl.Type_.Branch.V "decl_" ""}}
{{- end}}
```

That is the idiom `ruleast` uses in
[go_types.tmpl](../../ohmjs/ohm-go/ohm-cli/ruleast/templates/go_types.tmpl) with
`{{$G := .}}` / `{{$GMR := .GrammarNode}}`.

### Gotcha, found while implementing M0

`adlast.TypeRef` has **two branches carrying `string`** (`primitive` and `typeParam`), so
`{{tmpl_by_type .TypeRef.Branch.V ...}}` resolves *both* to the template name `string` -
it cannot tell them apart.

Dispatch on `.Branch` instead of `.Branch.V`. The generated wrapper types are named and
distinct - [adlast.go:626-633](adl/sys/adlast/adlast.go#L626-L633) defines
`_TypeRef_Primitive`, `_TypeRef_TypeParam`, `_TypeRef_Reference` - so
`{{tmpl_by_type .TypeRef.Branch "" "_tmpl"}}` works, with the payload reachable as `.V`
inside the callee rather than as `dot`.

Rule of thumb: **`.Branch.V` when the branch payload types are distinct; `.Branch` when
they are not.** `DeclType` is fine on `.Branch.V`; `TypeRef` is not.

### The goapi params structs

`goapi` holds 10 of the 20 params structs. They are *not* part of this milestone: they
feed templates whose opening and closing delimiters are split between the template and
`body.Rr.Buf.WriteString("}\n")` in Go, so converting them means restructuring how a
`goapi` file is assembled. Leave them until M3 is done and that restructuring can be
judged on its own.

## 3. Move the emission into templates (M3)

[gogen_adl2go_val.go](goadlc/internal/cli/gogen/gogen_adl2go_val.go) is 509 lines, and the
bulk of it builds Go source with `fmt.Sprintf` and `strings.Join`, layout included
(`"%sMakeAll_%s%s(\n%s,\n)"`). That is a template's job, and `tmpl_by_type` on
`DeclType` / `TypeRef.Branch` is the dispatch it needs:

- [`goStruct`](goadlc/internal/cli/gogen/gogen_adl2go_val.go#L252)
- [`goUnion`](goadlc/internal/cli/gogen/gogen_adl2go_val.go#L324)
- [`goValuePrimitive`](goadlc/internal/cli/gogen/gogen_adl2go_val.go#L418)
- [`strRep`](goadlc/internal/cli/gogen/gogen_adl2go_val.go#L220)

**Be selective.** The split that matters is *emission* vs *resolution*:

| stays in Go | moves to templates |
|---|---|
| `goType` / `PrimitiveMap` ([gogen_adl2go.go](goadlc/internal/cli/gogen/gogen_adl2go.go)) | `goStruct`, `goUnion`, `goValuePrimitive` |
| `SubstituteTypeBindings`, `CreateDecBoundTypeParams` | `strRep` |
| `goimports` bookkeeping | |
| the `dfs` / `transKids` graph walk in `goapi` | |

`goType` returns a structured `goTypeExpr` consumed by other Go code, not a string of
output - it belongs where it is. Pushing it into a template would be the same mistake in
the opposite direction.

This is the one milestone where output *layout* can legitimately shift, since templates
lay out whitespace differently from `Sprintf`. Everything is run through `go/format` in
[WriteFile](goadlc/internal/cli/gogen/gogen_writefile.go#L57), so it should mostly wash
out; if a diff appears, `gofmt` both sides before judging it.

---

## Verification loop

`goadlc` is **self-hosting** - `goadlc/cfgs/goadlc.v2.cfg.json` regenerates goadlc's own
`internal/cli/*/*.go` and `*_ast.go` from `goadlc/adl/`, and `adlast.v2.cfg.json`
regenerates the `adlast` types goadlc itself consumes. So every step has an exact check:

```sh
task gen                          # gen_texpr, gen_adlast, gen_goadlc, gen_stdlib, gen_exer
task tests:gen_api                # exercises the goapi templates
git diff --exit-code              # generated output must be byte-identical
cd tests && go test -count=1 ./...
```

M0 held this at every step, including a deliberate break-and-restore to prove the
`tmpl_by_type` dispatch was load-bearing:

```
template: gengo.tmpl:10:36: executing "render" at <.>:
  template "structParams" (for type gotypes.structParams) not defined
```

M1 and M2 must leave `git diff` empty. M3 is the only milestone allowed to move
whitespace, per the note above.

`adlc` must be on `PATH` or every config fails at the "generate individual ast files"
step. The flake pins 1.2.3; `nix develop` provides it.

The self-hosting property also means **a mistake is load-bearing**: a broken `goadlc`
regenerates a broken `adlast`, which `goadlc` then compiles against. Work on a branch you
can reset.

## What this is not

Not an argument that everything should become a template. `ruleast` is the right reference
for *dispatch and emission*; it is not a reference for type resolution, because it has
none - Ohm rules map to Go types by naming convention, whereas ADL has generics, type
aliases, newtypes, custom types and binding substitution. That machinery
([gogen_adl2go.go](goadlc/internal/cli/gogen/gogen_adl2go.go),
`adl.SubstituteTypeBindings`) is the part of `goadlc` that has no `ruleast` counterpart,
and it should stay in Go.
