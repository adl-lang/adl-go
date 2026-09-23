package gogen

import (
	"github.com/adl-lang/adl-go/goadlc/internal/cli/goimports"
)

type headerParams struct {
	Pkg string
}

type importsParams struct {
	// Rt      string
	Imports []goimports.ImportSpec
}
