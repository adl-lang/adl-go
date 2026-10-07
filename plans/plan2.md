# plan2: one template call per generated file

Goal: **each output file is produced by a single `ExecuteTemplate` call.** The template
owns the file's structure; Go supplies the data and writes the result. Two pieces of
scaffolding go away in the process:

- `{{define "render"}}` - the `tmpl_by_type`-by-params-type trampoline
- `gogen.RenderString` - render-a-fragment-to-a-string, called from inside Go

This supersedes the parts of [plan.md](plan.md) that are already done (M0-M3). It is a
response to the M3 outcome: templates earned their keep for *file and decl structure*,
and cost more than they returned for *small expression fragments*.

## Where the indirection is today

Nothing renders a file. Go renders ~20 fragments and concatenates them:

```
gotypes_fn.go   loops decls -> Rr.Render(declParams)      -> "render" -> tmpl_by_type -> "declParams"
                              Rr.Render(aTexprParams)     -> "render" -> ...
                              Rr.Render(scopedDeclParams) -> "render" -> ...
gogen_writefile.go  Rr.Render(headerParams) + Rr.Render(importsParams)
                    then Buf.Write(body.Bytes())          <- string concatenation
goapi_fn.go     Rr.Render(serviceParams) ... then Buf.WriteString("}\n")   <- brace in Go
gogen_adl2go_val.go  RenderString(ctorParams) x3, RenderString(texprParams),
                     RenderString(annMapParams), RenderString(annEntryParams),
                     RenderString(custTypeConstructionParams)
```

Every arrow is a hop that exists only because no template is in charge. `render` exists
solely so `Render(any)` can pick a template from the value's type - a trampoline that is
unnecessary once each call site names the template it wants.

---

## The one real obstacle: imports

**The import block is emitted at the top of the file, but which imports are needed is
only discovered while rendering the body.** Templates call `{{$G.GoImport "adlast"}}` and
`{{$G.GoType ...}}`, and both mutate `Imports` ([goimports.AddPath / AddModule](goadlc/internal/cli/goimports/imports.go#L69-L110)).
[WriteFile](goadlc/internal/cli/gogen/gogen_writefile.go#L42-L53) then filters to the
specs actually marked used.

That ordering is the reason for the two-buffer concatenation, and it is what blocks a
naive single template call. Three ways out:

### Option A - lazy `Body`, forced by `Imports`

The file params expose `Body()` (renders the decls once, caches) and `Imports()` (calls
`Body()` first, then returns the used specs). `{{range .Imports}}` runs before `{{.Body}}`,
so the forcing order works out.

- Smallest change; one `ExecuteTemplate` from Go; behaviour identical.
- But `Body()` is still "render a template to a string" - `RenderString` under a different
  name, scoped to one place instead of eight.

### Option B - render twice, discard the first pass

```go
Gen.ExecuteTemplate(io.Discard, "gotypes_decls", p) // populates Imports
Gen.ExecuteTemplate(w, "gotypes_file", p)           // emits
```

- No buffering helper at all.
- Two calls, not one, unless the first is hidden behind a void func invoked from the file
  template - which is worse than being honest about it.
- Doubles render cost and assumes every side effect is idempotent. `AddPath` and `sort`
  are; `goval_gen.path` only feeds panic messages. Probably safe, but it is an assumption
  the current code does not make.

### Option C - collect imports from the AST, before rendering

Walk the decls resolving type references and custom-type annotations, registering imports,
without emitting anything. Then one template call renders a file whose imports are already
known.

- The architecturally clean answer: emission stops being the thing that discovers imports.
- Most work, and the walk must agree with what the body emits. **It is self-checking
  though** - over-collect and the generated file has an unused import (compile error),
  under-collect and it has an undefined reference (compile error). Both surface on the
  next `task gen` + build.

**Recommendation: C, with A as the fallback** if the collection walk turns out to need
more than ~80 lines. Do not take B; "one call" that is secretly two is the kind of thing
this plan exists to remove.

---

## Milestones

| | Milestone | Exit criteria |
|---|---|---|
| **N1** | Value fragments return to Go | no `RenderString` calls outside tests; `ctorParams`, `texprParams`, `annMapParams`, `annEntryParams` gone |
| **N2** | Imports known before rendering | chosen option landed; `WriteFile` no longer concatenates two buffers |
| **N3** | One template call per file | three file templates; `render`, `Render`, `RenderString` deleted |

Each milestone must leave `git diff` empty after a regen.

## N1 - value fragments return to Go

These are small format strings sitting deep inside a Go recursion; routing them through
the template set buys layout-in-templates and costs a params type, accessor methods and a
call hop each. Put them back:

| revert | to |
|---|---|
| `RenderString(ctorParams{...})` x3 | `fmt.Sprintf("%s(\n%s,\n)", ctor, arg)` |
| `RenderString(texprParams{...})` | the recursive `Handle_TypeRef` in `strRep` |
| `RenderString(annMapParams/annEntryParams{...})` | the two `Sprintf`s in `goStruct` |

`custTypeConstructionParams` is the judgement call: it is 9 lines of layout with a `range`
in the middle, which is the one of these that reads better as a template. Keep it, and give
it the single remaining `RenderString`-shaped helper - or fold it into N3 by having the
decl template invoke it directly.

Keep the **`ctorParams` consolidation idea** even while reverting the template: struct,
union and newtype all emit `ctor(\narg,\n)` or `ctor()`, so they should share one Go
helper rather than three copies of the format string.

**Cost to be aware of:** [gogen_adl2go_val_test.go](goadlc/internal/cli/gogen/gogen_adl2go_val_test.go)
currently pins these four templates, and covers the two paths the regen never reaches
(`strRep` runs only for `go_custom_type` decls; `annEntryParams` only for a non-empty
annotations list). Those tests must be rewritten against the Go functions, which means a
stub `SubTask` so `strRep` can call `GoImport`. Do not drop the coverage - it is the only
check on those two paths.

## N2 - imports known before rendering

Implement the chosen option above. Exit criteria:

- `Imports` is fully populated before any file content is written
- `WriteFile` stops building a second `Generator` for the header and stops doing
  `header.Rr.Buf.Write(in.Rr.Bytes())`
- regen byte-identical

Land this before N3: N3 is mechanical once imports are no longer order-dependent, and
tangled with it if they still are.

## N3 - one template call per file

Three output shapes, so three entry templates:

| file | template | today |
|---|---|---|
| `<mod>.go` | `gotypes_decls_file` | `declBody` accumulated across decls |
| `<mod>_ast.go` | `gotypes_ast_file` | `astBody` accumulated across decls |
| `<mod>_srv.go` | `goapi_srv_file` | `body` accumulated, plus two literal `"}\n"` writes |

```
{{define "gotypes_decls_file"}}// Code generated by goadlc - DO NOT EDIT.
package {{.Pkg}}

import (
{{range .Imports}}	{{if .Aliased}}{{.Name}} {{end}}"{{.Path}}"
{{end}})
{{range .Decls}}{{template "declParams" .}}{{end}}{{end}}
```

Then:

- `headerParams` and `importsParams` stop being separately rendered templates and become
  part of each file template (or one shared `{{define "file_head"}}`).
- `{{define "render"}}` has no callers and is deleted, along with
  `TemplateRenderer.Render` and `RenderString`. `TemplateRenderer` itself probably reduces
  to nothing: `WriteFile` can take an `io.Writer` and the template can execute straight
  into the gofmt buffer.
- The Go loops in [gotypes_fn.go](goadlc/internal/cli/gotypes/gotypes_fn.go#L68-L91) and
  [goapi_fn.go](goadlc/internal/cli/goapi/goapi_fn.go#L172-L315) become data preparation -
  build the decl list, hand it to one call - instead of emission drivers.

### Bonus: the split braces close themselves

[goapi_fn.go:235](goadlc/internal/cli/goapi/goapi_fn.go#L235) and
[:315](goadlc/internal/cli/goapi/goapi_fn.go#L315) each do `Rr.Buf.WriteString("}\n")`
because the `serviceParams` and `registerParams` templates emit an opening `interface {`
/ `) {` that nothing closes. With one template ranging over the methods, the template that
opens the brace also closes it. This was the old plan's dropped "split delimiters"
section; N3 fixes it as a side effect rather than as its own job.

## What stays

- **`tmpl_by_type` for decl-branch dispatch.** `declParams` -> `decl_Struct` /
  `decl_Union` / `decl_TypeDef` / `decl_NewType` is dispatch on real ADL union branches,
  and it removed a four-closure ladder. It is not the indirection this plan is removing.
  Same for the `.Branch` vs `.Branch.V` rule: `.Branch.V` when the branch payload types
  are distinct, `.Branch` when they are not (`TypeRef` has two `string` branches).
- **`WithDynamicScopedVars`.** It is what lets the branch templates reach `$P` and `$G`,
  and N3 needs it more, not less, as templates nest deeper.
- **Type resolution in Go.** `goType`, `PrimitiveMap`, `SubstituteTypeBindings`,
  `goimports` bookkeeping, the `dfs`/`transKids` walk in `goapi`. Unchanged.
- **`goValuePrimitive` in Go.** Already reverted; it stays a plain switch.

## Verification loop

Unchanged, and still exact - goadlc is self-hosting:

```sh
task gen                          # gen_texpr, gen_adlast, gen_goadlc, gen_stdlib, gen_exer
task tests:gen_api
git diff --exit-code              # generated output must be byte-identical
cd tests && go test -count=1 ./...
cd goadlc && go test -count=1 ./...
```

`adlc` must be on `PATH`; the flake pins 1.2.3.

N1 and N2 must leave `git diff` empty. **N3 is the one that may legitimately move
whitespace** - a file rendered by one template lays out blank lines between decls
differently from concatenated fragments. Everything goes through `go/format` in
`WriteFile`, which collapses runs of blank lines between top-level decls, so most of it
should wash out. If a diff survives, read it before accepting it: gofmt normalises
blank lines, but it will not hide a dropped or duplicated section.

Break-testing is worth repeating per milestone - rename a define and confirm the failure
names it. It caught two templates in M3 that the regen never reaches.
