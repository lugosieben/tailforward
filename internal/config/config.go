package config

import (
	"os"
	"path/filepath"
)

type Paths struct {
	ConfigFile string
	StateDir   string
}

func FromExecutable() (Paths, error) {
	exe, err := os.Executable()
	if err != nil {
		dir, _ := os.Getwd()
		return Paths{
			ConfigFile: filepath.Join(dir, "tailforward.json"),
			StateDir:   filepath.Join(dir, "tsnet-state"),
		}, nil
	}
	dir := filepath.Dir(exe)
	return Paths{
		ConfigFile: filepath.Join(dir, "tailforward.json"),
		StateDir:   filepath.Join(dir, "tsnet-state"),
	}, nil
}
