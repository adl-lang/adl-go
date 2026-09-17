package root

import (
	"encoding/json"
	"fmt"
	"os"
)

type RootObj struct {
	Debug      bool   `help:"Print extra diagnostic information, especially about files being read/written"`
	Cfg        string `help:"Config file in json format (NOTE file entries take precedence over command-line flags & env)" json:"-"`
	DumpConfig bool   `opts:"short=u" help:"Dump the config to stdout and exits" json:"-"`
}

func (rt RootObj) Config(in interface{}) error {
	if rt.Cfg != "" {
		fd, err := os.Open(rt.Cfg)
		// config is in its own func
		// this defer fire correctly
		//
		// won't fire if dump is used as os.Exit terminates program
		defer func() {
			fd.Close()
		}()
		if err != nil {
			cwd, _ := os.Getwd()
			return fmt.Errorf("error opening file cwd:%s cfg:%s err:%v", cwd, rt.Cfg, err)
		}
		dec := json.NewDecoder(fd)
		dec.DisallowUnknownFields()
		err = dec.Decode(in)
		if err != nil {
			return err
			// log.Fatalf("json error %v", err)
		}
	}
	if rt.DumpConfig {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		err := enc.Encode(in)
		if err != nil {
			return fmt.Errorf("json encoding error %v", err)
		}
		os.Exit(0)
	}
	return nil
}
