package gogen

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"

	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/root"
)

func (in *Generator) WriteFile(
	rt *root.Root,
	modCodeGenPkg string,
	path string,
	noGoFmt bool,
	specialImports []goimports.ImportSpec,
) error {
	var err error
	dir, file := filepath.Split(path)
	_ = file

	if d, err := os.Stat(dir); err != nil {
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
	} else {
		if !d.IsDir() {
			return fmt.Errorf("directory expected %v", dir)
		}
	}

	header := &Generator{
		BaseGen: in.BaseGen,
		Rr:      TemplateRenderer{Tmpl: templates},
	}
	header.Rr.Render(headerParams{
		Pkg: modCodeGenPkg,
	})
	useImports := []goimports.ImportSpec{}
	for _, spec := range in.Imports.Specs {
		if in.Imports.Used[spec.Path] {
			useImports = append(useImports, spec)
		}
	}
	useImports = append(useImports, specialImports...)

	header.Rr.Render(importsParams{
		Imports: useImports,
	})
	header.Rr.Buf.Write(in.Rr.Bytes())
	unformatted := header.Rr.Bytes()

	var formatted []byte
	if !noGoFmt {
		formatted, err = format.Source(unformatted)
		if err != nil {
			formatted = unformatted
			fmt.Fprintf(os.Stderr, "error go fmt src file: %s, err: %v\n", path, err)
		}
	} else {
		formatted = unformatted
	}
	var fd *os.File = nil
	fd, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return err
	}
	err = fd.Truncate(0)
	if err != nil {
		return err
	}
	_, err = fd.Seek(0, 0)
	if err != nil {
		return err
	}
	defer func() {
		fd.Sync()
		fd.Close()
	}()
	_, err = fd.Write(formatted)
	if rt.Debug {
		fmt.Fprintf(os.Stderr, "wrote file %s\n", path)
	}
	return err
}
