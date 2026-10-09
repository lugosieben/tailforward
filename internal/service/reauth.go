package service

import (
	"context"
	"errors"
	"log"
	"os"
	"time"
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
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.reauthing = false
		s.mu.Unlock()
	}()

	log.Print("reauth: requested")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	_ = old.Logout(ctx)
	cancel()
	_ = old.Close()
	_ = os.RemoveAll(s.paths.StateDir)
	s.tracker.SetStatus("connecting to tailnet...")
	s.tracker.SetAuthURL("")
	select {
	case s.restart <- struct{}{}:
	default:
	}
	return nil
}
