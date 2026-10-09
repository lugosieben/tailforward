package service

import (
	"log"

	"tailforward/internal/domain"
	"tailforward/internal/store"
)

func (s *Service) Restore() {
	loaded := store.Load(s.paths.ConfigFile)
	s.mu.Lock()
	s.rules = append([]domain.Rule{}, loaded...)
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

	if replaced != nil {
		s.fwd.Stop(*replaced)
		log.Printf("rule replaced: %s (was %s)", r.Label(), replaced.Label())
	} else {
		log.Printf("rule added: %s", r.Label())
	}
	store.Save(s.paths.ConfigFile, snap)
	s.fwd.Start(r)
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

	if removed != nil {
		s.fwd.Stop(*removed)
		log.Printf("rule deleted: %s", removed.Label())
	}
	store.Save(s.paths.ConfigFile, snap)
}
