package store

import (
	"encoding/json"
	"log"
	"os"
)

type settings struct {
	Ephemeral *bool `json:"ephemeral"`
}

func LoadEphemeral(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
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

func SaveEphemeral(path string, ephemeral bool) {
	data, _ := json.MarshalIndent(settings{Ephemeral: &ephemeral}, "", "  ")
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("cannot save %s: %v", path, err)
	}
}
