package gengo

import "os"

func (in *GenGo) Run() error {
	if in.ChangePWD != "" {
		err := os.Chdir(in.ChangePWD)
		if err != nil {
			return err
		}
	}
	ld, err := in.Loader.Load()
	if err != nil {
		return err
	}
	gm, err := in.Mod.Modpath(in.Root.Debug)
	if err != nil {
		return err
	}

	if in.GoTypes != nil {
		in.GoTypes.Loader = ld
		in.GoTypes.GoMod = gm
		if err := in.GoTypes.Run(); err != nil {
			return err
		}
	}
	if in.GoApis != nil {
		for _, api := range *in.GoApis {
			api.Root = in.Root
			api.GoMod = gm
			api.Loader = ld
			if err := api.Run(); err != nil {
				return err
			}
		}
	}
	return nil
}
