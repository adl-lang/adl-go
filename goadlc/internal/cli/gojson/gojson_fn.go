package gojson

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adl-lang/adl-go/adl/sys/adlast"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/gogen"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/loader"

	"golang.org/x/sync/errgroup"
)

// Run writes one <pkg>_json.go per module in the generated set, beside the
// <pkg>.go that gotypes wrote: per struct, union and newtype an Encode_,
// a Decode_ and, with Equal, an Equal_ func. A type alias gets none; a
// reference to one is read through it. A module with no such decl gets no
// file.
//
// The funcs read and write the JSON of package adl's reflective codec:
// every field of a struct, in decl order; a union as {"branch":value}, an
// enum as "branch"; a StringMap with its keys in order. A decoder takes a
// union as a string or an object, sets a missing field to its default,
// and refuses a missing field with none and a field the struct lacks.
func (in *GoJson) Run() error {
	var (
		eg      = &errgroup.Group{}
		midPath string
		mods    []loader.NamedModule
		err     error
	)
	if midPath, err = goimports.MidPath(in.Outputdir, in.GoMod.RootDir); err != nil {
		return err
	}
	if mods, err = in.moduleSet(); err != nil {
		return err
	}
	for _, m := range mods {
		eg.Go(in.thunkGenModule(m, midPath))
	}
	if err = eg.Wait(); err != nil {
		return fmt.Errorf("error generating json module : %w", err)
	}
	return nil
}

// moduleSet is the modules named in Modules, or every loaded module.
func (in *GoJson) moduleSet() ([]loader.NamedModule, error) {
	var (
		byName = map[string]loader.NamedModule{}
		mods   []loader.NamedModule
	)
	if len(in.Modules) == 0 {
		return in.Loader.Modules, nil
	}
	for _, m := range in.Loader.Modules {
		byName[m.Name] = m
	}
	for _, name := range in.Modules {
		var m, ok = byName[name]
		if !ok {
			return nil, fmt.Errorf("GoJson.Modules names '%s', which is not a loaded module (bundle modules such as sys.* are never generated)", name)
		}
		mods = append(mods, m)
	}
	return mods, nil
}

func (in *GoJson) thunkGenModule(m loader.NamedModule, midPath string) func() error {
	return func() error {
		var (
			modDir = strings.Split(m.Name, ".")
			pkg    = modDir[len(modDir)-1]
			dir    = filepath.Join(in.Outputdir, filepath.Join(modDir...))
			g      = &generator{
				in: in,
				G: &gogen.Generator{
					BaseGen: gogen.NewBaseGen(in.GoMod.ModulePath, midPath, m.Name, in, *in.Loader),
				},
				module: m.Name,
			}
			names []string
			wrote bool
		)
		for k := range m.Module_.Decls {
			names = append(names, k)
		}
		slices.Sort(names)
		for _, k := range names {
			var (
				decl = m.Module_.Decls[k]
				ok   bool
				err  error
			)
			if ok, err = g.decl(decl); err != nil {
				return fmt.Errorf("%s.%s: %w", m.Name, decl.Name, err)
			}
			wrote = wrote || ok
		}
		if !wrote {
			return nil
		}
		return gogen.WriteFile(in.Root, filepath.Join(dir, pkg+"_json.go"), in.NoGoFmt,
			&gogen.FileParams{
				Pkg:      pkg,
				G:        g.G,
				BodyTmpl: "text_body",
				BodyData: gogen.TextBody{
					Text: g.out.String(),
				},
			})
	}
}

// generator writes one module's file.
type generator struct {
	in     *GoJson
	G      *gogen.Generator
	module string
	out    strings.Builder
}

func (g *generator) p(format string, args ...any) {
	fmt.Fprintf(&g.out, format, args...)
	g.out.WriteByte('\n')
}

// rt is the runtime package's qualifier, registering its import.
func (g *generator) rt() string {
	var q, err = g.G.GoImport("gojson")
	if err != nil {
		panic(err)
	}
	return q
}

// goType is te's Go type, registering the imports it names.
func (g *generator) goType(te adlast.TypeExpr) string {
	return g.G.GoType(te, nil).String()
}

func public(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// quoted is s as a JSON string, for a key the generated code writes raw.
func quoted(s string) string {
	var b, _ = json.Marshal(s)
	return string(b)
}

// backquoted is a Go raw string literal of s, which holds no backquote.
func backquoted(s string) string {
	if strings.Contains(s, "`") {
		var b, _ = json.Marshal(s)
		return string(b)
	}
	return "`" + s + "`"
}

// declHead is what a decl's three funcs share: the Go type, the type
// params in brackets, and the codec params of each type param.
type declHead struct {
	goName    string
	typeArgs  string
	typeLSide string
	params    []string
}

func (g *generator) head(decl adlast.Decl, goName string) declHead {
	var (
		tp = gogen.TypeParamsFromDecl(decl)
		h  = declHead{
			goName:    goName,
			typeArgs:  tp.RSide(),
			typeLSide: tp.LSide(),
		}
	)
	for _, p := range tp.Params {
		h.params = append(h.params, p.Name)
	}
	return h
}

// sig is one func's signature: kind is Encode, Decode or Equal.
func (g *generator) sig(kind string, h declHead) string {
	var (
		rt    = g.rt()
		self  = h.goName + h.typeArgs
		extra strings.Builder
	)
	for _, p := range h.params {
		switch kind {
		case "Encode":
			fmt.Fprintf(&extra, ", enc%s %sEncoder[%s]", p, rt, p)
		case "Decode":
			fmt.Fprintf(&extra, ", dec%s %sDecoder[%s]", p, rt, p)
		case "Equal":
			fmt.Fprintf(&extra, ", eq%s %sEqual[%s]", p, rt, p)
		}
	}
	switch kind {
	case "Encode":
		return fmt.Sprintf("func Encode_%s%s(w *%sWriter, v *%s%s) error", h.goName, h.typeLSide, rt, self, extra.String())
	case "Decode":
		return fmt.Sprintf("func Decode_%s%s(r *%sReader, v *%s%s) error", h.goName, h.typeLSide, rt, self, extra.String())
	}
	return fmt.Sprintf("func Equal_%s%s(a, b *%s%s) bool", h.goName, h.typeLSide, self, extra.String())
}

// decl writes the funcs of one decl, and reports whether it wrote any.
func (g *generator) decl(decl adlast.Decl) (wrote bool, err error) {
	var (
		st adlast.Struct
		un adlast.Union
		nt adlast.NewType
		ok bool
	)
	if gogen.GoCustomTypeAnn(decl.Annotations) != nil {
		return false, nil
	}
	defer func() {
		if r := recover(); r != nil {
			if e, isErr := r.(unsupported); isErr {
				err = e
				return
			}
			panic(r)
		}
	}()
	if st, ok = decl.Type_.Cast_struct_(); ok {
		g.structFuncs(decl, st)
		return true, nil
	}
	if un, ok = decl.Type_.Cast_union_(); ok {
		g.unionFuncs(decl, un)
		return true, nil
	}
	if nt, ok = decl.Type_.Cast_newtype_(); ok {
		g.newtypeFuncs(decl, nt)
		return true, nil
	}
	return false, nil
}

// unsupported is a type the backend has no codec for; it panics up to
// decl, which answers it as the error.
type unsupported struct {
	what string
}

func (u unsupported) Error() string {
	return "the GoJson backend has no codec for " + u.what
}

func (g *generator) structFuncs(decl adlast.Decl, st adlast.Struct) {
	var (
		name   = public(decl.Name)
		h      = g.head(decl, name)
		rt     = g.rt()
		fields []adlast.Field
		adl    = g.module + "." + decl.Name
	)
	for _, f := range st.Fields {
		if f.SerializedName != "-" {
			fields = append(fields, f)
		}
	}
	// Encode
	g.p("%s {", g.sig("Encode", h))
	if len(fields) == 0 {
		g.p("\tw.Raw(`{}`)")
		g.p("\treturn nil")
		g.p("}\n")
	} else {
		for i, f := range fields {
			var sep = ","
			if i == 0 {
				sep = "{"
			}
			g.p("\tw.Raw(%s)", backquoted(sep+quoted(f.SerializedName)+":"))
			g.p("\tif err := %s(w, &v.%s); err != nil {", g.codec("Encode", f.TypeExpr), public(f.Name))
			g.p("\t\treturn err")
			g.p("\t}")
		}
		g.p("\tw.Byte('}')")
		g.p("\treturn nil")
		g.p("}\n")
	}
	// Decode
	g.p("%s {", g.sig("Decode", h))
	if len(fields) > 0 {
		g.p("\tvar seen [%d]bool", len(fields))
	}
	g.p("\tif err := r.Object(func(key string) error {")
	if len(fields) > 0 {
		g.p("\t\tswitch key {")
		for i, f := range fields {
			g.p("\t\tcase %s:", quoted(f.SerializedName))
			g.p("\t\t\tseen[%d] = true", i)
			g.p("\t\t\treturn %s(r, &v.%s)", g.codec("Decode", f.TypeExpr), public(f.Name))
		}
		g.p("\t\t}")
	}
	g.p("\t\treturn %sUnexpected(%q, key)", rt, adl)
	g.p("\t}); err != nil {")
	g.p("\t\treturn err")
	g.p("\t}")
	for i, f := range fields {
		g.p("\tif !seen[%d] {", i)
		if _, has := f.Default.Cast_just(); has {
			g.p("\t\tv.%s = ((*%s%s)(nil)).Default_%s()", public(f.Name), name, h.typeArgs, g.G.GoEscape(f.Name))
		} else {
			g.p("\t\treturn %sMissing(%q, %q)", rt, adl, f.SerializedName)
		}
		g.p("\t}")
	}
	for _, f := range st.Fields {
		if f.SerializedName != "-" {
			continue
		}
		if _, has := f.Default.Cast_just(); has {
			g.p("\tv.%s = ((*%s%s)(nil)).Default_%s()", public(f.Name), name, h.typeArgs, g.G.GoEscape(f.Name))
		}
	}
	g.p("\treturn nil")
	g.p("}\n")
	// Equal
	if !g.in.Equal {
		return
	}
	g.p("%s {", g.sig("Equal", h))
	if len(fields) == 0 {
		g.p("\treturn true")
	} else {
		var terms []string
		for _, f := range fields {
			terms = append(terms, fmt.Sprintf("%s(&a.%s, &b.%s)", g.codec("Equal", f.TypeExpr), public(f.Name), public(f.Name)))
		}
		g.p("\treturn %s", strings.Join(terms, " &&\n\t\t"))
	}
	g.p("}\n")
}

// isVoid is whether te is Void, through aliases.
func (g *generator) isVoid(te adlast.TypeExpr) bool {
	var prim, ok = gogen.ExpandTypeAliases(g.in.Loader, te).TypeRef.Cast_primitive()
	return ok && prim == "Void"
}

// isNullable is whether te is Nullable, through aliases: a branch of
// that type written as a string is null, as package adl reads it.
func (g *generator) isNullable(te adlast.TypeExpr) bool {
	var prim, ok = gogen.ExpandTypeAliases(g.in.Loader, te).TypeRef.Cast_primitive()
	return ok && prim == "Nullable"
}

func (g *generator) unionFuncs(decl adlast.Decl, un adlast.Union) {
	var (
		name     = public(g.G.GoEscape(decl.Name))
		h        = g.head(decl, name)
		rt       = g.rt()
		adl      = g.module + "." + decl.Name
		enum     = true
		branches []adlast.Field
	)
	for _, f := range un.Fields {
		if f.SerializedName == "-" {
			continue
		}
		branches = append(branches, f)
		if !g.isVoid(f.TypeExpr) {
			enum = false
		}
	}
	// branchType is the Go type of a branch, as gotypes names it.
	var branchType = func(f adlast.Field) string {
		return "_" + name + "_" + public(f.Name) + g.G.GoType(f.TypeExpr, f.Annotations).UnionTypeParams.RSide()
	}
	// Encode
	g.p("%s {", g.sig("Encode", h))
	if enum {
		g.p("\tswitch v.Branch.(type) {")
	} else {
		g.p("\tswitch b := v.Branch.(type) {")
	}
	for _, f := range branches {
		g.p("\tcase %s:", branchType(f))
		switch {
		case enum:
			g.p("\t\tw.Raw(%s)", backquoted(quoted(f.SerializedName)))
		case g.isVoid(f.TypeExpr):
			g.p("\t\tw.Raw(%s)", backquoted("{"+quoted(f.SerializedName)+":null}"))
		default:
			g.p("\t\tw.Raw(%s)", backquoted("{"+quoted(f.SerializedName)+":"))
			g.p("\t\tif err := %s(w, &b.V); err != nil {", g.codec("Encode", f.TypeExpr))
			g.p("\t\t\treturn err")
			g.p("\t\t}")
			g.p("\t\tw.Byte('}')")
		}
	}
	g.p("\tdefault:")
	g.p("\t\treturn %sIncomplete(%q)", rt, adl)
	g.p("\t}")
	g.p("\treturn nil")
	g.p("}\n")
	// Decode
	g.p("%s {", g.sig("Decode", h))
	g.p("\tvar branch, inObject, err = r.Union()")
	g.p("\tif err != nil {")
	g.p("\t\treturn err")
	g.p("\t}")
	g.p("\tswitch branch {")
	for _, f := range branches {
		g.p("\tcase %s:", quoted(f.SerializedName))
		if g.isVoid(f.TypeExpr) {
			g.p("\t\tif inObject {")
			g.p("\t\t\tif err = r.Skip(); err != nil {")
			g.p("\t\t\t\treturn err")
			g.p("\t\t\t}")
			g.p("\t\t}")
			g.p("\t\tv.Branch = %s{}", branchType(f))
			continue
		}
		g.p("\t\tvar x %s", g.goType(f.TypeExpr))
		g.p("\t\tif inObject {")
		g.p("\t\t\tif err = %s(r, &x); err != nil {", g.codec("Decode", f.TypeExpr))
		g.p("\t\t\t\treturn err")
		g.p("\t\t\t}")
		if !g.isNullable(f.TypeExpr) {
			g.p("\t\t} else {")
			g.p("\t\t\treturn %sNeedsValue(%q, branch)", rt, adl)
		}
		g.p("\t\t}")
		g.p("\t\tv.Branch = %s{x}", branchType(f))
	}
	g.p("\tdefault:")
	g.p("\t\treturn %sUnknownBranch(%q, branch)", rt, adl)
	g.p("\t}")
	g.p("\treturn r.UnionEnd(inObject)")
	g.p("}\n")
	// Equal
	if !g.in.Equal {
		return
	}
	g.p("%s {", g.sig("Equal", h))
	if enum {
		g.p("\tswitch a.Branch.(type) {")
	} else {
		g.p("\tswitch x := a.Branch.(type) {")
	}
	for _, f := range branches {
		g.p("\tcase %s:", branchType(f))
		if g.isVoid(f.TypeExpr) {
			g.p("\t\t_, ok := b.Branch.(%s)", branchType(f))
			g.p("\t\treturn ok")
			continue
		}
		g.p("\t\ty, ok := b.Branch.(%s)", branchType(f))
		g.p("\t\treturn ok && %s(&x.V, &y.V)", g.codec("Equal", f.TypeExpr))
	}
	g.p("\tcase nil:")
	g.p("\t\treturn b.Branch == nil")
	g.p("\t}")
	g.p("\treturn false")
	g.p("}\n")
}

// newtypeFuncs reads a newtype as its underlying type: Go converts the
// pointer, since the two have one underlying type.
func (g *generator) newtypeFuncs(decl adlast.Decl, nt adlast.NewType) {
	var (
		name  = public(decl.Name)
		h     = g.head(decl, name)
		inner = g.goType(nt.TypeExpr)
	)
	g.p("%s {", g.sig("Encode", h))
	g.p("\treturn %s(w, (*%s)(v))", g.codec("Encode", nt.TypeExpr), inner)
	g.p("}\n")
	g.p("%s {", g.sig("Decode", h))
	g.p("\treturn %s(r, (*%s)(v))", g.codec("Decode", nt.TypeExpr), inner)
	g.p("}\n")
	if !g.in.Equal {
		return
	}
	g.p("%s {", g.sig("Equal", h))
	g.p("\treturn %s((*%s)(a), (*%s)(b))", g.codec("Equal", nt.TypeExpr), inner, inner)
	g.p("}\n")
}

// codec is a Go expression of te's codec of the kind: a func that takes
// the Writer, the Reader or the second value, and a pointer to te's Go
// type. A type param's codec is the param the decl's func was given.
func (g *generator) codec(kind string, te adlast.TypeExpr) string {
	var (
		rt   = g.rt()
		prim string
		tp   string
		ref  adlast.ScopedName
		ok   bool
	)
	te = gogen.ExpandTypeAliases(g.in.Loader, te)
	if tp, ok = te.TypeRef.Cast_typeParam(); ok {
		switch kind {
		case "Encode":
			return "enc" + tp
		case "Decode":
			return "dec" + tp
		}
		return "eq" + tp
	}
	if prim, ok = te.TypeRef.Cast_primitive(); ok {
		return g.primitive(kind, prim, te)
	}
	ref, _ = te.TypeRef.Cast_reference()
	var decl, found = g.in.Loader.Resolver(ref)
	if !found {
		panic(unsupported{what: "the unresolved " + ref.ModuleName + "." + ref.Name})
	}
	if gogen.GoCustomTypeAnn(decl.Annotations) != nil {
		panic(unsupported{what: "the custom type " + ref.ModuleName + "." + ref.Name})
	}
	if ref.ModuleName == "sys.types" {
		switch ref.Name {
		case "Maybe", "Pair", "Either":
			return g.closure(kind, te, rt+kind+ref.Name, te.Parameters)
		}
		panic(unsupported{what: "sys.types." + ref.Name})
	}
	if g.isBundle(ref.ModuleName) {
		panic(unsupported{what: "the bundle decl " + ref.ModuleName + "." + ref.Name})
	}
	var (
		goName = public(decl.Name)
		pkg    = ""
	)
	if _, isUnion := decl.Type_.Cast_union_(); isUnion {
		goName = public(g.G.GoEscape(decl.Name))
	}
	if ref.ModuleName != g.module {
		pkg = g.G.Imports.AddModule(ref.ModuleName, g.G.ModulePath, g.G.MidPath) + "."
	}
	if len(te.Parameters) == 0 {
		return pkg + kind + "_" + goName
	}
	return g.closure(kind, te, pkg+kind+"_"+goName, te.Parameters)
}

// isBundle is whether a module is a bundle's, sys.* and the like, which
// the backend never generates.
func (g *generator) isBundle(module string) bool {
	for _, b := range g.in.Loader.BundleMaps {
		if strings.HasPrefix(module, b.AdlModuleNamePrefix) {
			return true
		}
	}
	return false
}

// closure is a func literal of te's codec that calls fn with the codecs
// of params after the value.
func (g *generator) closure(kind string, te adlast.TypeExpr, fn string, params []adlast.TypeExpr) string {
	var (
		rt   = g.rt()
		self = g.goType(te)
		args []string
	)
	for _, p := range params {
		args = append(args, g.codec(kind, p))
	}
	var tail = ""
	if len(args) > 0 {
		tail = ", " + strings.Join(args, ", ")
	}
	switch kind {
	case "Encode":
		return fmt.Sprintf("func(w *%sWriter, v *%s) error { return %s(w, v%s) }", rt, self, fn, tail)
	case "Decode":
		return fmt.Sprintf("func(r *%sReader, v *%s) error { return %s(r, v%s) }", rt, self, fn, tail)
	}
	return fmt.Sprintf("func(a, b *%s) bool { return %s(a, b%s) }", self, fn, tail)
}

func (g *generator) primitive(kind, prim string, te adlast.TypeExpr) string {
	var rt = g.rt()
	switch prim {
	case "Vector", "StringMap", "Nullable":
		return g.closure(kind, te, rt+kind+prim, te.Parameters)
	case "Json":
		panic(unsupported{what: "Json, which is any in Go"})
	case "TypeToken":
		panic(unsupported{what: "TypeToken"})
	}
	var names = map[string]string{
		"String":     "String",
		"Bool":       "Bool",
		"Int8":       "Int8",
		"Int16":      "Int16",
		"Int32":      "Int32",
		"Int64":      "Int64",
		"Word8":      "Word8",
		"Word16":     "Word16",
		"Word32":     "Word32",
		"Word64":     "Word64",
		"Float":      "Float",
		"Double":     "Float",
		"Void":       "Void",
		"ByteVector": "Bytes",
	}
	var name, ok = names[prim]
	if !ok {
		panic(unsupported{what: "the primitive " + prim})
	}
	if kind != "Equal" {
		return rt + kind + name
	}
	switch name {
	case "Void", "Bytes":
		return rt + "Equal" + name
	}
	return rt + "EqualPrim[" + g.goType(te) + "]"
}

func (in *GoJson) ReservedImports() []goimports.ImportSpec {
	return []goimports.ImportSpec{
		{
			Path:    in.GoAdlPath + "/gojson",
			Aliased: false,
			Name:    "gojson",
		},
	}
}

func (in *GoJson) GoImport(pkg string, currModuleName string, imports *goimports.Imports) (string, error) {
	var (
		spec goimports.ImportSpec
		ok   bool
	)
	if spec, ok = imports.ByName(pkg); !ok {
		return "", fmt.Errorf("unknown import %s", pkg)
	}
	imports.AddPath(spec.Path)
	return spec.Name + ".", nil
}

func (in *GoJson) IsStdLibGen() bool {
	return false
}

func (in *GoJson) GoAdlImportPath() string {
	return in.GoAdlPath
}
