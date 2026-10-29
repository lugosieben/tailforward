package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"

	"tailforward/internal/crypt"
)

type settings struct {
	Ephemeral *bool `json:"ephemeral"`
}

func LoadEphemeral(path, key string) bool {
	data, err := crypt.ReadFile(path, key)
	if err != nil {
		if !errors.Is(err, crypt.ErrEncrypted) && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("ignoring unreadable %s: %v", path, err)
		}
		return true
	}
	var s settings
	if err := json.Unmarshal(data, &s); err != nil || s.Ephemeral == nil {
		if err != nil {
			log.Printf("ignoring corrupt %s: %v", path, err)
		}
		return true
	}
	return *s.Ephemeral
}

func SaveEphemeral(path string, ephemeral bool, key string) {
	data, _ := json.MarshalIndent(settings{Ephemeral: &ephemeral}, "", "  ")
	if err := crypt.WriteFile(path, data, key); err != nil {
		log.Printf("cannot save %s: %v", path, err)
	}
}
