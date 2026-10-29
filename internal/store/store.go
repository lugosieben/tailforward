package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"

	"tailforward/internal/crypt"
	"tailforward/internal/domain"
)

func Load(path, key string) []domain.Rule {
	data, err := crypt.ReadFile(path, key)
	if err != nil {
		if !errors.Is(err, crypt.ErrEncrypted) && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("ignoring unreadable %s: %v", path, err)
		}
		return nil
	}
	var raw []domain.Rule
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("ignoring corrupt %s: %v", path, err)
		return nil
	}
	var kept []domain.Rule
	seen := map[int]bool{}
	for _, c := range raw {
		if seen[c.LocalStart] {
			log.Printf("dropping duplicate saved rule %s", c.Label())
			continue
		}
		r, err := domain.Validate(c.Host, c.RemoteStart, c.RemoteEnd, c.LocalStart, kept)
		if err != nil {
			log.Printf("dropping invalid saved rule %s: %s", c.Label(), err)
			continue
		}
		seen[r.LocalStart] = true
		kept = append(kept, r)
	}
	log.Printf("loaded %d rule(s) from %s", len(kept), path)
	return kept
}

func Save(path string, rules []domain.Rule, key string) {
	data, _ := json.MarshalIndent(rules, "", "  ")
	if err := crypt.WriteFile(path, data, key); err != nil {
		log.Printf("cannot save %s: %v", path, err)
	}
}
