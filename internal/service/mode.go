package service

import (
	"log"

	"tailforward/internal/store"
)

func (s *Service) SetEphemeral(ephemeral bool) (bool, error) {
	s.mu.Lock()
	if s.locked {
		s.mu.Unlock()
		return false, ErrLocked
	}
	if s.reauthing {
		s.mu.Unlock()
		return false, ErrReauthInProgress
	}
	if s.ephemeral == ephemeral {
		s.mu.Unlock()
		return false, nil
	}
	s.reauthing = true
	old := s.server
	key := s.key
	s.ephemeral = ephemeral
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.reauthing = false
		s.mu.Unlock()
	}()

	store.SaveEphemeral(s.paths.SettingsFile, ephemeral, key)
	if ephemeral {
		s.restartTailnet(old, "mode: switching to ephemeral, logging out")
		log.Print("mode: now ephemeral")
	} else {
		s.restartTailnet(old, "mode: switching to persistent, logging out")
		log.Print("mode: now persistent")
	}
	return true, nil
}
