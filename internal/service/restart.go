package service

import (
	"context"
	"log"
	"os"
	"time"

	"tailforward/internal/tailnet"
)

func (s *Service) cancelCurrent() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func closeTimeout(srv *tailnet.Server, d time.Duration) {
	done := make(chan struct{})
	go func() {
		_ = srv.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		log.Print("tailnet close timed out, continuing anyway")
	}
}

func (s *Service) restartTailnet(old *tailnet.Server, reason string) {
	log.Print(reason)
	s.cancelCurrent()
	if old != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = old.Logout(ctx)
		cancel()
		closeTimeout(old, 10*time.Second)
	}
	_ = os.RemoveAll(s.paths.StateDir)
	s.tracker.SetStatus("connecting to tailnet...")
	s.tracker.SetAuthURL("")
	select {
	case s.restart <- struct{}{}:
	default:
	}
}
