package goimports

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/adl-lang/adl-go/goadlc/internal/cli/loader"
)

type Imports struct {
	// importMap map[string]importSpec
	bundleMap []loader.BundleMap
	Specs     []ImportSpec
	Used      map[string]bool // keyed on import path
}

type ImportSpec struct {
	Path    string
	Aliased bool
	Name    string
}

func NewImports(
	reserved []ImportSpec,
	bundleMap []loader.BundleMap,
	// importMap map[string]importSpec,
) Imports {
	im := Imports{
		Used:      make(map[string]bool),
		bundleMap: bundleMap,
	}
	for i := range reserved {
		spec := reserved[i]
		if !spec.Aliased {
			spec.Name = pkgFromImport(spec.Path)
		}
		im.Specs = append(im.Specs, spec)
	}
	return im
}

func (spec ImportSpec) String() string {
	if !spec.Aliased {
		return strconv.Quote(spec.Path)
	}
	return spec.Name + " " + strconv.Quote(spec.Path)
}

func (i *Imports) ByPath(path string) (spec ImportSpec, ok bool) {
	for _, spec = range i.Specs {
		if spec.Path == path {
			return spec, true
		}
	}
	return ImportSpec{}, false
}

func (i *Imports) ByName(name string) (spec ImportSpec, ok bool) {
	for _, spec = range i.Specs {
		if spec.Name == name {
			return spec, true
		}
	}
	return ImportSpec{}, false
}

func (i *Imports) AddSpec(spec ImportSpec) (name string) {
	spec0 := i.reserveSpec(spec)
	i.Used[spec0.Path] = true
	return spec0.Name
}

func (i *Imports) AddModule(module string, modulePath, midPath string) (name string) {
	for _, bun := range i.bundleMap {
		if strings.HasPrefix(module, bun.AdlModuleNamePrefix) {
			parts := strings.Split(module, ".")
			name := parts[len(parts)-1]
			spec := ImportSpec{
				Path:    filepath.Join(bun.GoModPath, strings.ReplaceAll(module, ".", "/")),
				Name:    name,
				Aliased: false,
			}
			if i.Used[spec.Path] {
				spec, _ = i.ByPath(spec.Path)
				return spec.Name
			}
			spec0 := i.reserveSpec(spec)
			i.Used[spec0.Path] = true
			return spec0.Name
		}
	}
	// if spec, ok := i.importMap[module]; ok {
	// 	if i.used[spec.Path] {
	// 		return spec.Name
	// 	}
	// 	spec0 := i.reserveSpec(spec)
	// 	i.used[spec0.Path] = true
	// 	return spec0.Name
	// }
	if midPath != "" {
		pkg := modulePath + "/" + midPath + "/" + strings.ReplaceAll(module, ".", "/")
		return i.AddPath(pkg)
	}
	pkg := modulePath + "/" + strings.ReplaceAll(module, ".", "/")
	return i.AddPath(pkg)
}

func (i *Imports) AddPath(path string) (name string) {
	spec := i.reserve(path)
	i.Used[spec.Path] = true
	return spec.Name
}

// reserve adds an import spec without marking it as used.
func (i *Imports) reserve(path string) ImportSpec {
	if ispec, ok := i.ByPath(path); ok {
		return ispec
	}
	spec := ImportSpec{
		Path:    path,
		Name:    pkgFromImport(path),
		Aliased: false,
	}
	return i.reserveSpec(spec)
}

func (i *Imports) reserveSpec(spec ImportSpec) ImportSpec {
	if ispec, ok := i.ByPath(spec.Path); ok {
		return ispec
	}
	if _, found := i.ByName(spec.Name); found {
		base := spec.Name
		spec.Aliased = true
		n := uint64(1)
		for {
			n++
			spec.Name = base + strconv.FormatUint(n, 10)
			if _, found = i.ByName(spec.Name); !found {
				break
			}
		}
	}
	i.Specs = append(i.Specs, spec)
	return spec
}

func pkgFromImport(path string) string {
	if i := strings.LastIndex(path, "/"); i != -1 {
		path = path[i+1:]
	}
	p := []rune(path)
	n := 0
	for _, r := range p {
		if isIdent(r) {
			p[n] = r
			n++
		}
	}
	if n == 0 || !isLower(p[0]) {
		return "pkg" + string(p[:n])
	}
	return string(p[:n])
}

func isLower(r rune) bool {
	return 'a' <= r && r <= 'z' || r == '_'
}

func isIdent(r rune) bool {
	return isLower(r) || 'A' <= r && r <= 'Z' || r >= 0x80 && unicode.IsLetter(r)
}
