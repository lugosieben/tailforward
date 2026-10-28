package service

import (
	"errors"
	"log"

	"tailforward/internal/domain"
	"tailforward/internal/store"
)

func (s *Service) Restore() {
	loaded := store.Load(s.paths.ConfigFile)
	s.mu.Lock()
	s.rules = append([]domain.Rule{}, loaded...)
	s.ephemeral = store.LoadEphemeral(s.paths.SettingsFile)
	s.mu.Unlock()
}

func (s *Service) Add(host string, rs, re, ls int) (domain.Rule, error) {
	s.mu.Lock()
	r, err := domain.Validate(host, rs, re, ls, s.rules)
	if err != nil {
		s.mu.Unlock()
		return domain.Rule{}, err
	}
	var replaced *domain.Rule
	for i, o := range s.rules {
		if o.LocalStart == r.LocalStart {
			old := o
			replaced = &old
			s.rules[i] = r
			break
		}
	}
	if replaced == nil {
		s.rules = append(s.rules, r)
	}
	snap := append([]domain.Rule{}, s.rules...)
	s.mu.Unlock()

	s.fwd.Sync(snap)
	store.Save(s.paths.ConfigFile, snap)
	if replaced != nil {
		log.Printf("rule replaced: %s (was %s)", r.Label(), replaced.Label())
	} else {
		log.Printf("rule added: %s", r.Label())
	}
	return r, nil
}

func (s *Service) Update(oldLS int, host string, rs, re, ls int) (domain.Rule, error) {
	s.mu.Lock()
	idx := -1
	for i, o := range s.rules {
		if o.LocalStart == oldLS {
			idx = i
			break
		}
	}
	if idx == -1 {
		s.mu.Unlock()
		return domain.Rule{}, errors.New("no rule with that local start port")
	}
	old := s.rules[idx]
	others := make([]domain.Rule, 0, len(s.rules)-1)
	for i, o := range s.rules {
		if i != idx {
			others = append(others, o)
		}
	}
	r, err := domain.Validate(host, rs, re, ls, others)
	if err != nil {
		s.mu.Unlock()
		return domain.Rule{}, err
	}
	s.rules[idx] = r
	snap := append([]domain.Rule{}, s.rules...)
	s.mu.Unlock()

	s.fwd.Sync(snap)
	store.Save(s.paths.ConfigFile, snap)
	log.Printf("rule updated: %s (was %s)", r.Label(), old.Label())
	return r, nil
}

func (s *Service) Delete(ls int) {
	s.mu.Lock()
	var removed *domain.Rule
	for i, o := range s.rules {
		if o.LocalStart == ls {
			old := o
			removed = &old
			s.rules = append(s.rules[:i], s.rules[i+1:]...)
			break
		}
	}
	snap := append([]domain.Rule{}, s.rules...)
	s.mu.Unlock()

	s.fwd.Sync(snap)
	store.Save(s.paths.ConfigFile, snap)
	if removed != nil {
		log.Printf("rule deleted: %s", removed.Label())
	}
}
