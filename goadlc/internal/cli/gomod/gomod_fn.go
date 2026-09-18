package gomod

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adl-lang/adl-go/adl"
	"golang.org/x/mod/modfile"
)

func (in *GoModule) Modpath(debug bool) (*GoModResult, error) {
	return HandleWithErr_GoModule[*GoModResult](
		*in,
		func(goModFile string) (*GoModResult, error) {
			return fromGoModFile(goModFile, debug)
		},
		func(dir string) (*GoModResult, error) {
			goModFile := filepath.Join(dir, "go.mod")
			for {
				if debug {
					fmt.Fprintf(os.Stderr, "searching for module-path in go.mod file. go.mod:%s\n", goModFile)
				}
				if _, err := os.Stat(goModFile); dir == "" || err == nil {
					break
				}
				dir, _ = filepath.Split(dir)
				goModFile = filepath.Join(dir, "go.mod")
			}
			if debug {
				fmt.Fprintf(os.Stderr, "looking for module-path in go.mod file. go.mod:%s\n", goModFile)
			}
			res, err := fromGoModFile(goModFile, debug)
			if err != nil {
				return nil, err
			}
			return res, nil
		},
		func() (*GoModResult, error) {
			panic("shouldn't get here")
		},
	)
}

func fromGoModFile(goModFile string, debug bool) (*GoModResult, error) {
	if gms, err := os.Stat(goModFile); err != nil {
		return nil, fmt.Errorf("no go.mod file found : %v", goModFile)
	} else if gms.IsDir() {
		return nil, fmt.Errorf("go.mod specified is a dir, file expected : %v", goModFile)
	} else {
		if modbufm, err := os.ReadFile(goModFile); err == nil {
			modulePath := modfile.ModulePath(modbufm)
			if debug {
				fmt.Fprintf(os.Stderr, "using module-path found in go.mod file. module-path:%s\n", modulePath)
			}
			rootDir := filepath.Dir(goModFile)
			return adl.Addr(Make_GoModResult(modulePath, rootDir)), nil
		} else {
			return nil, fmt.Errorf("module-path needed. Not specified in --module-path and couldn't be found in a go.mod file")
		}
	}
}
