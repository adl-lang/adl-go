package gogen

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"

	"github.com/adl-lang/adl-go/goadlc/internal/cli/root"
	"github.com/adl-lang/adl-go/goadlc/internal/cli/templates"
)

// WriteFile renders one generated file from the single "file" template and
// writes it to path, gofmt'd unless noGoFmt.
func WriteFile(
	rt *root.Root,
	path string,
	noGoFmt bool,
	file *FileParams,
) error {
	var err error
	dir, _ := filepath.Split(path)

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

	var buf bytes.Buffer
	if err = templates.Gen.ExecuteTemplate(&buf, "file", file); err != nil {
		renderPanic(file.BodyData, err)
	}
	unformatted := buf.Bytes()

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
