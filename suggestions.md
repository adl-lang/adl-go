# suggestions: goadlc

Cleanups for [goadlc](goadlc/), ranked within each section by payoff over effort. Read
[plan3.md](plan3.md) first - everything here is deliberately outside its scope, and
nothing below moves code between Go and a template. Items marked **(blocks plan3)** are
worth doing before the next milestone because plan3 touches the same lines.

Line numbers are as of `fa961f5`.

---

## 1. Bugs

### 1.1 `dfs` swallows its own errors

[goadlc/internal/cli/goapi/goapi_fn.go:122](goadlc/internal/cli/goapi/goapi_fn.go#L122)
and [:140](goadlc/internal/cli/goapi/goapi_fn.go#L140):

```go
err := in.dfs(nil, inst0, visited, result)
if err != nil {
    return nil          // ← should be: return err
}
```

Both recursive calls. Every error the walk can raise - the `[C,S,V]` name clashes, an
unresolvable ref, a capability api that is not a struct - is discarded for any api below
the root struct, and generation continues with a half-built `result`. The three-line
shape reads as if the error were handled, which is why it has survived. `return err`,
twice.

### 1.2 The value walk assigns to its caller's parameter

[goadlc/internal/cli/gogen/gogen_adl2go_val.go:295](goadlc/internal/cli/gogen/gogen_adl2go_val.go#L295),
inside `goStruct`'s field closure:

```go
val = reflect.ValueOf(just).Interface()   // val is goStruct's parameter
fgv = bg.goValue(fld.Annotations, monoTe, val)
```

`reflect.ValueOf(x).Interface()` is the identity on `any`, so the right-hand side is just
`just`; and the assignment mutates the enclosing `goStruct(…, val any)` from inside a
loop over fields. It is harmless *today* only because `mval` was captured before the loop
and nothing reads `val` afterwards. Use a local:

```go
fgv = bg.goValue(fld.Annotations, monoTe, just)
```

**(blocks plan3)** - M3 moves this function, and this line will not survive the move
intact.

### 1.3 `log.Fatalf` inside the loader

[goadlc/internal/cli/loader/loader_fn.go:110](goadlc/internal/cli/loader/loader_fn.go#L110).
`loadAdl` returns `error` everywhere else; a failed `os.Mkdir` of the working dir kills
the process with no config context and no stack. Return the error. Same file:
`filepath.WalkDir`'s return value at
[:127](goadlc/internal/cli/loader/loader_fn.go#L127) is discarded, so a permission error
reading the individual-AST dir silently yields zero modules and a successful run that
generates nothing.

### 1.4 Generated files are written 0777

[goadlc/internal/cli/gogen/gogen_writefile.go:53](goadlc/internal/cli/gogen/gogen_writefile.go#L53)
passes `os.ModePerm` to `os.OpenFile`, and [:26](goadlc/internal/cli/gogen/gogen_writefile.go#L26)
passes it to `MkdirAll`. Every generated `.go` file is world-writable and executable
(umask permitting). `0o644` and `0o755`.

### 1.5 `ReadConfig` defers `Close` before checking the error

[goadlc/internal/cli/root/root_fn.go:39-43](goadlc/internal/cli/root/root_fn.go#L39-L43).
Survives only because `(*os.File).Close` nil-checks its receiver. Move the `defer` below
the `if err != nil`.

### 1.6 `DumpConfig` calls `os.Exit` from a library

[goadlc/internal/cli/root/root_fn.go:30](goadlc/internal/cli/root/root_fn.go#L30). The
function prints, exits, and then returns `nil` for a caller that can never see it -
while `main` already has an `os.Exit` path for this flag. Drop the `os.Exit` and the
unused `rt Root` parameter; let `main` decide to stop.

---

## 2. Dead code

All verified as having no reader anywhere in `internal/`, `main.go` or the `.tmpl` files:

| what | where | note |
|---|---|---|
| `headerParams`, `importsParams` | [gogen_params.go](goadlc/internal/cli/gogen/gogen_params.go) | the whole file; left behind when plan2 collapsed the file into one `"file"` template |
| `param.Concrete` | [gogen_type_params.go:62](goadlc/internal/cli/gogen/gogen_type_params.go#L62) | written at 8 construction sites, read at none - deleting it simplifies every `param{…}` literal in `gogen_adl2go.go` |
| `TypeParam.Has`, `TypeParam.TpArgs` | [gogen_type_params.go:109,153](goadlc/internal/cli/gogen/gogen_type_params.go#L109) | `Has()`'s condition is also incomprehensible on its own terms |
| `goTypeExpr.Complete` | [gogen_adl2go.go:34](goadlc/internal/cli/gogen/gogen_adl2go.go#L34) | identical to `String()` minus the `IsTypeParam` branch |
| `SubTask.GoAdlImportPath` | [gogen_types.go:20](goadlc/internal/cli/gogen/gogen_types.go#L20) | never called; costs two implementations and two test stubs |
| `dump_exmaple` | [main.go:106](goadlc/main.go#L106) | 35 lines, misspelled, half commented out. If it is wanted, it is `-dump-config` on a fixture, not a function |

Roughly 120 lines, no behaviour change, and `git diff --exit-code` after a regen proves it.

---

## 3. Simplifications

### 3.1 Separate the config from the wiring **(largest win)**

Six ADL structs carry fields that are not configuration:

```adl
@SerializedName "-"  Nullable<Root> root = null;
@SerializedName "-"  Nullable<LoadResult> loader = null;
@SerializedName "-"  Nullable<GoModResult> goMod = null;
```

They exist so that `main` can pre-build an object graph, and `ReadConfig` can decode the
file *onto* it. That works because of two decoder properties that nothing states and no
test pins: a non-nil `Nullable` decodes in place, and a `"-"` field falls through to its
`= null` default, which is a no-op for a pointer. Give any of those fields a non-null
default, reassign a pointer between construction and `ReadConfig`, or change the decoder,
and the wiring goes nil - at which point `in.Root.Debug` panics somewhere down in the
loader.

The wiring is also split across two places today: `main` does `ld.Root`/`gt.Root`/
`gg.Loader`, and [gengo_fn.go](goadlc/internal/cli/gengo/gengo_fn.go) does
`GoTypes.Loader`/`GoTypes.GoMod`/`api.Root`/`api.GoMod`/`api.Loader` - because the second
group only exists after the loader has run. Nothing marks which group a given field is in.

Suggested shape: the ADL types describe **only** what is in the JSON, and the results flow
as arguments.

```go
type deps struct {           // hand-written, not ADL
    debug bool
    load  *loader.LoadResult
    mod   *gomod.GoModResult
}

func (in *GoTypes) Run(d deps) error
func (in *GoApi)   Run(d deps) error
```

That deletes ~10 nullable fields and their `Default_` funcs from the generated code, both
wiring blocks, the commented-out wiring in `main` (lines 59-62) and `gengo_fn.go`, and the
dependence on decode-in-place. It is a schema change, so it regenerates - do it between
plan3 milestones, not inside one.

### 3.2 Replace `Root.Debug` back-pointers with a logger

Falls out of 3.1 and is worth doing even without it. `*root.Root` is threaded through the
loader, gengo, gotypes and goapi to read one bool, next to ~15 hand-written
`if x.Debug { fmt.Fprintf(os.Stderr, …) }` sites. An `*slog.Logger` (or a two-method
interface) on the generator collapses the condition into the call, makes the diagnostics
testable, and removes the last reason for `GoApi` and `GoTypes` to hold a `Root` at all.

### 3.3 One `GoImport`, not two

[gotypes_fn.go:143-153](goadlc/internal/cli/gotypes/gotypes_fn.go#L143-L153) and
[goapi_fn.go:350-368](goadlc/internal/cli/goapi/goapi_fn.go#L350-L368) end in the same
six lines (`ByName` ➜ `AddPath` ➜ `Name + "."`). They differ only in a prefix rule:
gotypes suppresses the `adl` qualifier during stdlib generation, goapi expands a dotted
name like `common.http` against `GoAdlCommonImport`. Make that difference the interface -
`resolveImport(pkg string) (ImportSpec, bool)` or similar - and put the shared tail in
`goimports`.

While there: gotypes' `specialTexpr()`
([:135](goadlc/internal/cli/gotypes/gotypes_fn.go#L135)) allocates a three-entry map on
every call, and it is called once per `GoImport` - i.e. per qualified identifier emitted.
Package-level `var`.

### 3.4 `gotypes` re-implements `goimports.MidPath`

[gotypes_fn.go:42-54](goadlc/internal/cli/gotypes/gotypes_fn.go#L42-L54) is
[goimports/utils.go](goadlc/internal/cli/goimports/utils.go) inlined, down to the error
message - and it runs inside the per-module goroutine, so it recomputes the same two
`filepath.Abs` calls for every module. `goapi` already calls the helper. Call it once,
before the `errgroup`.

### 3.5 `kvBy` is a `sort.Interface` for a two-field struct

[gogen_adl2go_val.go:525-548](goadlc/internal/cli/gogen/gogen_adl2go_val.go#L525-L548) -
`Len`/`Swap`/`Less` plus a `String()` that joins. `slices.SortFunc(vs, func(a, b kv) int
{ return cmp.Compare(a.k, b.k) })` and a `strings.Join` over a mapped slice is four lines
and drops the type. **(blocks plan3)** - M3 rewrites the caller.

### 3.6 `samber/lo` earns less than it costs

`lo.Map`, `lo.FlatMap`, `lo.Contains`, `lo.ContainsBy` and `lo.Find` are the whole usage.
`slices.Contains`/`slices.IndexFunc` cover three of them; the two `Map`s over
`[]adlast.Field` read no worse as a `for` loop with an explicit `append`, and better where
the closure already spans eight lines
([goapi_fn.go:261](goadlc/internal/cli/goapi/goapi_fn.go#L261)). Optional - but it is a
dependency in the bootstrap path of a self-hosting compiler.

---

## 4. Performance

The generator is fast enough that none of this is urgent; both items are also
*simplifications*, which is why they are here.

### 4.1 JSON bindings are rebuilt per call

`adl.CreateJsonDecodeBinding` walks a type expression and constructs a closure tree. It is
called fresh on every invocation of:

- `get_type_constraints` ([gogen_adl2go.go:136](goadlc/internal/cli/gogen/gogen_adl2go.go#L136))
  - once per `goType` recursion, i.e. per type expression node,
- `TypeParamsFromDecl` ([gogen_type_params.go:47](goadlc/internal/cli/gogen/gogen_type_params.go#L47)) - per decl,
- `GoCustomTypeAnn` ([gogen_customtype.go:21](goadlc/internal/cli/gogen/gogen_customtype.go#L21)) - per decl *and* per resolved reference,
- `GoDeclValue` / `GoTexprValue` (the encode side) - per AST decl.

Each binding is a pure function of a fixed `Texpr_*`, so all of them are package-level
`var`s built once. This is a two-line change per site, and it removes the only reason
these functions look expensive.

### 4.2 Don't rewrite unchanged files

`WriteFile` always truncates and writes. Since the check the whole workflow rests on is
`git diff --exit-code` after a regen, most files are byte-identical most of the time;
comparing against the existing content and skipping the write keeps mtimes stable and
stops `task gen` from invalidating the Go build cache for the whole tree.

---

## 5. Testing

The real safety net is the regen-and-diff loop, and it needs `adlc` on `PATH` - a Haskell
binary, pinned by the flake. That is a good check and a bad CI dependency, and it is why
`go test ./...` in `goadlc/` currently exercises one package.

**Golden test over a checked-in AST.** [tests/combined.json](tests/combined.json) is
already in the repo: the decoded output of `adlc ast --combined-output`. A test that
decodes it, runs `gotypes` over one module and compares against a golden file covers the
loader-to-output half of the generator with no external binary. Regenerate goldens with a
`-update` flag, so the diff review is the same motion as the regen review.

**Pin the import names the templates use.** `{{$G.GoImport "adlast"}}` fails at execution
time, only on the branch that runs it, and only with `unknown import adlast`. A test that
greps the `.tmpl` files for `GoImport "…"` literals and asserts each name resolves in the
corresponding sub-task's `ReservedImports()` is ~20 lines and turns a latent runtime
failure into a compile-time-ish one. Worth more once plan3 moves more emission into
templates, where `WithDynamicScopedVars` has already given up the parser's scope checking.

**Pin decode-in-place** if 3.1 is *not* done: one test that decodes a config over a
pre-wired `GenGo` and asserts `gg.Loader.Root != nil`. It is the property the whole
startup path depends on and nothing currently states it.

---

## 6. Smaller things

| | where | |
|---|---|---|
| Empty `import ()` in generated files | [gengo.tmpl:18](goadlc/internal/cli/templates/gengo.tmpl#L18) | `{{if .Imports}}` around the block; gofmt keeps the empty parens otherwise |
| `goapi` ignores `NoGoFmt` | [goapi_fn.go:63](goadlc/internal/cli/goapi/goapi_fn.go#L63) | hardcoded `false`; either honour the flag or move it to `Root` |
| `Loader.Load` mutates its receiver | [loader_fn.go:39-52](goadlc/internal/cli/loader/loader_fn.go#L39-L52) | appends bundle dirs to `in.Searchdir`, so a second call double-adds. Build the list locally |
| `path[strings.LastIndex(path, "/")+1:]` | [loader_fn.go:133](goadlc/internal/cli/loader/loader_fn.go#L133) | `filepath.Base` + `strings.TrimSuffix`; the hand-rolled form also breaks on Windows separators |
| Nil `Module_` is not guarded | [gotypes_fn.go:65](goadlc/internal/cli/gotypes/gotypes_fn.go#L65) | a module present in the individual output but absent from `combined.json` panics on `m.Module_.Decls`. Cheap named error |
| Four copies of `defer recover; Fprintf; panic` | `gogen_adl2go.go:64`, `gogen_types.go:37`, `gogen_adl2go_val.go:22,100` | one `defer diag("GoType")` helper; they differ only in the label |
| Shadowed `err`, unused `d` | [gogen_writefile.go:25-33](goadlc/internal/cli/gogen/gogen_writefile.go#L25-L33) | correct as written, but it takes a second read to confirm that |
| `errgroup` without `SetLimit` | [gotypes_fn.go:21](goadlc/internal/cli/gotypes/gotypes_fn.go#L21) | one goroutine per ADL module; bound it if a large bundle ever shows up |
| `templates.Gen` is `template.Must` at init | [templates.go:24](goadlc/internal/cli/templates/templates.go#L24) | plan3/M3 needs this in an `init()` anyway, for the `render_each` cycle |

---

## 7. Deliberately not suggested

So these don't get re-litigated:

- **Panics in the resolution and value walks.** They carry a path and a stack, the process
  has nothing to recover to, and returning errors through a `text/template` call site buys
  a worse message. Keep.
- **Shelling out to `adlc` twice.** The individual run is what distinguishes "modules
  asked for" from "modules reachable"; there is no single invocation that answers both.
- **The `_X` embedded-struct shape of generated types.** It is what makes the `Make_`
  constructors the only way to get defaults applied.
- **Moving `GoType` / `GoImport` / `GoEscape` into templates.** [plan3.md](plan3.md)
  settles this: they return structure or perform a side effect, and a template can do
  neither.
- **The two sorts in the value walk.** Also plan3 - they sort different things and must
  stay different.
