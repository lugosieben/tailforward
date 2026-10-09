package service

import (
	"errors"
	"log"
)

var ErrReauthInProgress = errors.New("reauth already in progress")

func (s *Service) Reauth() error {
	s.mu.Lock()
	if s.reauthing {
		s.mu.Unlock()
		return ErrReauthInProgress
	}
	if s.server == nil {
		s.mu.Unlock()
		return errors.New("tailnet not started yet")
	}
	s.reauthing = true
	old := s.server
	persistent := !s.ephemeral
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.reauthing = false
		s.mu.Unlock()
	}()

	if persistent {
		log.Print("reauth: persistent device logged out locally")
	}
	s.restartTailnet(old, "reauth: requested")
	return nil
}
