package goimports

import (
	"fmt"
	"path/filepath"
	"strings"
)

func MidPath(outputdir, rootDir string) (string, error) {
	oabs, err1 := filepath.Abs(outputdir)
	rabs, err2 := filepath.Abs(rootDir)
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("error get abs dir %w %w", err1, err2)
	}
	if !strings.HasPrefix(oabs, rabs) {
		return "", fmt.Errorf("output dir must be inside root of go.mod out: %s root: %s", oabs, rabs)
	}
	// if in.Root.Debug {
	// 	fmt.Fprintf(os.Stderr, "out: '%s' root: '%s'\n", oabs, rabs)
	// }
	var midPath string
	if oabs != rabs {
		midPath = oabs[len(rabs)+1:]
	}
	return midPath, nil
}
