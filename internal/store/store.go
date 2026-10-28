package store

import (
	"encoding/json"
	"log"
	"os"

	"tailforward/internal/domain"
)

func Load(path string) []domain.Rule {
	data, err := os.ReadFile(path)
	if err != nil {
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

func Save(path string, rules []domain.Rule) {
	data, _ := json.MarshalIndent(rules, "", "  ")
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("cannot save %s: %v", path, err)
	}
}
