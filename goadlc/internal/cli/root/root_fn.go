package root

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/adl-lang/adl-go/adl"
	"github.com/adl-lang/adl-go/adl/sys/adlast"
)

func DumpConfig[A any](
	rt Root,
	te adlast.ATypeExpr[A],
	in A,
) error {
	enc := adl.CreateJsonEncodeBinding(te, adl.RESOLVER)
	buf := bytes.Buffer{}
	err := enc.Encode(&buf, in)
	if err != nil {
		return fmt.Errorf("json encoding error %v", err)
	}
	buf0 := bytes.Buffer{}
	err = json.Indent(&buf0, buf.Bytes(), "", "  ")
	if err != nil {
		return fmt.Errorf("json indent error %v", err)
	}
	fmt.Printf("%s\n", buf0.String())
	os.Exit(0)
	return nil
}

func ReadConfig[A any](
	rt Root,
	te adlast.ATypeExpr[A],
	in *A,
) error {
	fd, err := os.Open(rt.Cfg)
	defer func() {
		fd.Close()
	}()
	if err != nil {
		cwd, _ := os.Getwd()
		return fmt.Errorf("error opening file cwd:%s cfg:%s err:%v", cwd, rt.Cfg, err)
	}
	dec := adl.CreateJsonDecodeBinding(te, adl.RESOLVER)
	err = dec.Decode(fd, in)
	if err != nil {
		return err
	}
	return nil
}
