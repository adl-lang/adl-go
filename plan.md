# Simplification plan: `goadlc`

Scope: the `goadlc` code generator (`goadlc/internal/`), measured against the
`ohm-cli/ruleast` generator in `ohmjs/ohm-go` as a worked reference for the same problem
shape - walk an ADL/AST tree, emit Go.

## The core finding

**`goadlc` imports `github.com/millergarym/gotmpl/text/template` and uses none of its four
extensions.** Verified by grep: no `tmpl_by_type`, no `WithDynamicScopedVars`, no dynamic
template names, no void FuncMap funcs. Today the dependency is a drop-in replacement for
stock `text/template` and buys nothing.

Worse, `goadlc` has **re-implemented `tmpl_by_type` in Go**.
[gogen_types.go:111-123](goadlc/internal/cli/gogen/gogen_types.go#L111-L123):

```go
func (tr *TemplateRenderer) Render(params any) {
	typeName := reflect.TypeOf(params).Name()
	name := typeName[:len(typeName)-len("Params")]
	err := tr.Tmpl.ExecuteTemplate(&tr.Buf, name, params)
	...
}
```

That is `{{tmpl_by_type . "" ""}}` with a hardcoded `"Params"` suffix, executed from Go
instead of from the template - so dispatch lives in Go, and every dispatch point needs a
Go call site plus a bespoke struct.

### The consequence, in numbers

| | templates | hand-written generation Go |
|---|---|---|
| `ruleast` | 704 lines | ~500 lines |
| `goadlc` | 222 lines | ~2160 lines |

`ruleast` puts ~58% of the generator in templates; `goadlc` puts ~10%. The other 90% is
`fmt.Sprintf` emitting Go source
([gogen_adl2go_val.go](goadlc/internal/cli/gogen/gogen_adl2go_val.go) is 515 lines of it)
plus **20 `xxxParams` structs** whose only job is to repackage the ADL AST into something
a stock template can reach.

`ruleast` has zero params structs. The ADL-generated AST node *is* the template data;
ambient state (`$G`, the grammar) rides on dynamic-scoped vars; dispatch is
`{{tmpl_by_type .Branch.V "" "_Suffix"}}`.

---

## 1. Use `tmpl_by_type`; delete `TemplateRenderer.Render`

Replace the reflect-and-strip-`Params` shim with the real action. The ADL-generated union
shape makes this direct - `adlast.DeclType` branch payloads are
`adlast.Struct` / `adlast.Union` / `adlast.TypeDef` / `adlast.NewType`, all named types:

```
{{tmpl_by_type .Type_.Branch.V "decl_" ""}}
```

dispatches to `decl_Struct`, `decl_Union`, `decl_TypeDef`, `decl_NewType`. That removes
the four-closure `adlast.Handle_DeclType` ladder in `generalDeclV3`
([gotypes_fn.go:164-223](goadlc/internal/cli/gotypes/gotypes_fn.go#L164-L223)) and the
four corresponding `Rr.Render(...)` call sites.

### Gotcha worth knowing before you start

`adlast.TypeRef` has **two branches carrying `string`** (`primitive` and `typeParam`), so
`{{tmpl_by_type .TypeRef.Branch.V ...}}` resolves *both* to the template name `string` -
it cannot distinguish them.

Dispatch on `.Branch` instead of `.Branch.V`. The generated wrapper types are named and
distinct - [adlast.go:626-633](adl/sys/adlast/adlast.go#L626-L633) defines
`_TypeRef_Primitive`, `_TypeRef_TypeParam`, `_TypeRef_Reference` - so
`{{tmpl_by_type .TypeRef.Branch "" "_tmpl"}}` works. The payload is then `.V` inside the
callee rather than `dot`.

Rule of thumb: **`.Branch.V` when branch payload types are distinct; `.Branch` when they
are not.**

## 2. Delete the 20 `*Params` structs

They exist only because stock templates give you exactly one slot (`dot`), so shared state
has to be smuggled in. Every one of them re-carries `G *gogen.Generator` alongside a
hand-picked slice of AST. Enabling `WithDynamicScopedVars()` removes the reason they
exist:

```
{{$G := .}}   {{/* bound once at the root template */}}
...
{{define "decl_Struct"}}...{{$G.GoType .TypeExpr .Annotations}}...{{end}}
```

`$G` is then reachable from every template below without being threaded through `dot`,
exactly as [go_types.tmpl](../../ohmjs/ohm-go/ohm-cli/ruleast/templates/go_types.tmpl)
does with `{{$G := .}}` / `{{$GMR := .GrammarNode}}`.

Where a params struct carries a genuinely *derived* value rather than plumbing - `IsVoid`,
`HasDefault`, `ContainsTypeToken` - make it a method on the ADL type instead of a struct
field. `gogen.TypeParam` ([gogen_type_params.go:65-158](goadlc/internal/cli/gogen/gogen_type_params.go#L65-L158))
is already this pattern done right (`LSide`, `RSide`, `TexprArgs`, `Last`), and `ruleast`
uses it throughout (`IsNode`, `GoType`, `Descr`, `RuleName`). Extend it rather than
inventing a parallel struct:

- `fieldParams.IsVoid` -> `func (f Field) IsVoid() bool`
- `fieldParams.HasDefault` -> `func (f Field) HasDefault() bool`
- `structParams.ContainsTypeToken` -> `func (s Struct) ContainsTypeToken() bool`
  ([containsTypeToken](goadlc/internal/cli/gotypes/gotypes_fn.go#L315-L322) already has
  the body)

These go in a `gogen` helpers file, not in generated `adlast` code.

## 3. One template set, not three

`gogen`, `gotypes` and `goapi` each declare their own `templates` var with their own
`ParseFS`, and each defines an **identical** `public()` helper
([gogen_embed.go:12](goadlc/internal/cli/gogen/gogen_embed.go#L12),
[gotypes_embed.go:12](goadlc/internal/cli/gotypes/gotypes_embed.go#L12),
[goapi_embed.go:12](goadlc/internal/cli/goapi/goapi_embed.go#L12)).

Because the sets are disjoint, a template in one package cannot invoke one in another.
That is why `goCustomType`
([gogen_adl2go_val.go:193-223](goadlc/internal/cli/gogen/gogen_adl2go_val.go#L193-L223))
constructs an entire throwaway `Generator` just to render one snippet and read
`.Buf.String()` back out.

Move to a single `gogen` template set + FuncMap that all three packages parse into. The
throwaway-Generator dance then becomes a plain `{{template "custTypeConstruction" .}}`.

While there: `TemplateRenderer` gives every caller its own `bytes.Buffer` which is then
concatenated by hand ([gogen_writefile.go:53](goadlc/internal/cli/gogen/gogen_writefile.go#L53)).
Execute into one `io.Writer` instead.

## 4. Move the emission `Sprintf`s into templates - but only the emission

[gogen_adl2go_val.go](goadlc/internal/cli/gogen/gogen_adl2go_val.go) is the main target.
`goStruct` (:258-328), `goUnion` (:330-422) and `goValuePrimitive` (:424-490) build Go
source with `fmt.Sprintf` and `strings.Join`, including the layout (`"%sMakeAll_%s%s(\n%s,\n)"`).
That is a template's job, and `tmpl_by_type` on `DeclType` / `TypeRef.Branch` is the
dispatch it needs.

**Be selective.** Not all 2160 lines want to be templates - the split that matters is
*emission* vs *resolution*:

| stays in Go | moves to templates |
|---|---|
| `goType` / `PrimitiveMap` ([gogen_adl2go.go](goadlc/internal/cli/gogen/gogen_adl2go.go)) | `goStruct`, `goUnion`, `goValuePrimitive` |
| `SubstituteTypeBindings`, `CreateDecBoundTypeParams` | `strRep` (:226-245) |
| `goimports` bookkeeping | `genInterface` / `genRegister` bodies |
| the `dfs` / `transKids` graph walk in `goapi` | |

`goType` returns a structured `goTypeExpr` consumed by other Go code, not a string of
output - it belongs where it is. Pushing it into a template would be the same mistake in
the opposite direction.

## 5. Fix the split delimiters in `goapi`

[goapi_fn.go:232](goadlc/internal/cli/goapi/goapi_fn.go#L232) and
[:309](goadlc/internal/cli/goapi/goapi_fn.go#L309) both do:

```go
body.Rr.Buf.WriteString("}\n")
```

...because the `service` and `register` templates emit the *opening* `interface {` / `) {`
and nothing closes them. This is precisely the failure mode the gotmpl README calls out:
matched delimiters split across two places because a value computed on the way down the
tree cannot be seen on the way back up. With dynamic scoping the whole interface becomes
one recursive template that opens, ranges, and closes.

Also collapse the duplicated dispatch. `genInterface` (:172-233) and `genRegister`
(:235-310) run the *same* `switch ref.Name { case "HttpPost" / "HttpGet" /
"CapabilityApi" / default }` over the same fields, differing only in which template name
they pick. With gotmpl's dynamic template names this is one range and a computed name:

```
{{range .Fields}}{{$n := printf "%s%s" $mode (refKind .)}}{{template $n .}}{{end}}
```

## 6. Dead code

Confirmed unreferenced across the whole workspace:

- **`goadlc/internal/diff/`** - 261 lines, zero importers.
- **`goadlc/internal/root/`** - 47 lines, an obsolete pre-ADL twin of
  `internal/cli/root`. Zero importers.
- **`root.Config`** ([root_fn.go:13-54](goadlc/internal/cli/root/root_fn.go#L13-L54)) - no
  callers; its body is `ReadConfig` + `DumpConfig` inlined, both of which *are* called.
- **`templates/capget` and `templates/cappost`** in `goapi` - no `capgetParams` /
  `cappostParams` type exists, so `Render` can never name them. The live `get` / `post`
  templates already handle the cap case inline via `{{if .IsCap}}`.
- **`TypeParam.Has`**, **`TypeParam.TpArgs`**, **`Generator.JsonEncode`**,
  **`Generator.ToTitle`**.

Note `TypeParam.Has` ([gogen_type_params.go:109-111](goadlc/internal/cli/gogen/gogen_type_params.go#L109-L111))
reads `(!tp.Added && len(tp.Params) != 0) || len(tp.Params) != 1`, which is true for zero
params. If you keep it rather than delete it, that second clause wants checking.

### One to confirm, not assume

[templates/getapi](goadlc/internal/cli/goapi/templates/getapi) emits
`{{if .Params}}[{{range .Params}}, {{$G.GoType . $anns}}{{end}}]{{end}}` - a leading comma
inside the brackets, producing `Foo_Service[, T]`, which will not compile. The sibling
`getcapapi` template gets it right because `C` and `S` precede the range. I could not find
a fixture in `tests/` that exercises a non-empty `.Params` on the `getapi` path, so this
is latent rather than an observed breakage - worth a test either way.

## 7. Smaller Go-level cleanups

- `gogen.TypeParamsFromDecl`
  ([gogen_type_params.go:14-56](goadlc/internal/cli/gogen/gogen_type_params.go#L14-L56))
  is a 4-closure `Handle_DeclType` whose branches are identical apart from `.TypeParams`.
  **`adl.TypeParamsFromDecl` already exists** ([adl/utils.go:67](adl/utils.go#L67)) and
  returns `[]string`; `gogen`'s version is that plus a `lo.Map` into `param`. ~30 lines
  -> ~5.
- The `jb := adl.CreateJsonDecodeBinding(adl.Texpr_GoCustomType(), adl.RESOLVER)` +
  `adl.GetAnnotation(...)` + `panic(err)` triple appears four times
  ([gotypes_fn.go:76](goadlc/internal/cli/gotypes/gotypes_fn.go#L76),
  [gotypes_fn.go:238](goadlc/internal/cli/gotypes/gotypes_fn.go#L238),
  [gogen_adl2go.go:155](goadlc/internal/cli/gogen/gogen_adl2go.go#L155),
  [gogen_adl2go_val.go:178](goadlc/internal/cli/gogen/gogen_adl2go_val.go#L178)).
  One `func goCustomTypeAnn(anns) *GoCustomType`.
- The `ImportSpec{Path: gct.Gotype.Import_path, Name: gct.Gotype.Pkg, Aliased: pkg != ...}`
  block appears three times verbatim. One helper.
- The `defer func() { r := recover(); ... panic(r) }()` stack-dump preamble appears four
  times. One `defer dumpOnPanic("GoType")()`.
- **House style.** `CLAUDE.md` specifies hoisting locals into a `var (...)` block and
  assigning inside `if` with an inlined error check. `ruleast` follows it
  (`if gmr, err = ohm.NewGrammar(ctx, ...); err != nil`); `goadlc` mostly does not
  (`fd, err := os.Open(...)` then a separate check). Worth normalising in the files you
  touch anyway, not as a standalone pass.
- Minor: `ReadConfig` and `Config` both `defer fd.Close()` *before* checking the `os.Open`
  error. Harmless today (`(*os.File)(nil).Close()` returns `ErrInvalid`), but inverted.

---

## Verification loop

`goadlc` is **self-hosting** - `goadlc/cfgs/goadlc.v2.cfg.json` regenerates goadlc's own
`internal/cli/*/\*.go` and `*_ast.go` from `goadlc/adl/`, and `adlast.v2.cfg.json`
regenerates the `adlast` types goadlc itself consumes. So every step has an exact check:

```sh
task gen                          # gen_texpr, gen_adlast, gen_goadlc, gen_stdlib, gen_exer
git diff --exit-code              # generated output must be byte-identical
cd tests && go test ./...
```

Steps 1-3, 6 and 7 must leave `git diff` empty. Step 4 is the only one where output
*layout* can legitimately shift (templates lay out whitespace differently from
`Sprintf`) - and since everything is run through `go/format` in
[WriteFile](goadlc/internal/cli/gogen/gogen_writefile.go#L57), even that should mostly
wash out. If a diff appears there, `gofmt` both sides before judging it.

The self-hosting property also means **a mistake is load-bearing**: a broken `goadlc`
regenerates a broken `adlast`, which `goadlc` then compiles against. Commit a known-good
binary or work on a branch you can reset.

## Suggested order

**6 -> 7 -> 3 -> 1 -> 2 -> 5 -> 4**

Dead code and mechanical dedup first (they shrink the surface everything else touches),
then merge the template sets, then switch dispatch to `tmpl_by_type`, then drop the params
structs once dispatch no longer needs them, then `goapi`, and only then the `goValue`
rewrite - which is the one genuinely risky step and wants the `tests/` fixtures green
before it starts.

## What this is not

Not an argument that everything should become a template. `ruleast` is the right reference
for *dispatch and emission*; it is not a reference for type resolution, because it has
none - Ohm rules map to Go types by naming convention, whereas ADL has generics, type
aliases, newtypes, custom types and binding substitution. That machinery
([gogen_adl2go.go](goadlc/internal/cli/gogen/gogen_adl2go.go),
`adl.SubstituteTypeBindings`) is the part of `goadlc` that has no `ruleast` counterpart,
and it should stay in Go.
