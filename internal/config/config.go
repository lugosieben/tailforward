package config

import (
	"os"
	"path/filepath"
)

type Paths struct {
	ConfigFile   string
	SettingsFile string
	StateDir     string
}

func FromExecutable() (Paths, error) {
	dir := os.Getenv("TAILFORWARD_DIR")
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			dir, _ = os.Getwd()
		} else {
			dir = filepath.Dir(exe)
		}
	}
	return Paths{
		ConfigFile:   filepath.Join(dir, "tailforward.json"),
		SettingsFile: filepath.Join(dir, "settings.json"),
		StateDir:     filepath.Join(dir, "tsnet-state"),
	}, nil
}
