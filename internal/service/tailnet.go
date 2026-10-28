package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"tailforward/internal/tailnet"
)

func (s *Service) Loop() {
restartOuter:
	for {
		s.mu.Lock()
		ephemeral := s.ephemeral
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.mu.Unlock()
		srv := tailnet.New(s.paths.StateDir, ephemeral, s.userLogf)
		if err := srv.Start(); err != nil {
			s.tracker.SetStatus("tailnet error: " + err.Error())
			log.Printf("tailnet start failed: %v", err)
			select {
			case <-s.restart:
				continue restartOuter
			case <-time.After(10 * time.Second):
				continue restartOuter
			}
		}
		s.mu.Lock()
		s.server = srv
		s.mu.Unlock()
		s.tracker.SetStatus("connecting to tailnet...")
		go s.watchEvents(ctx, srv)
		go s.pollDevices(ctx, srv)

		for attempt := 0; ; attempt++ {
			select {
			case <-s.restart:
				closeTimeout(srv, 10*time.Second)
				continue restartOuter
			default:
			}
			st, err := srv.Up(ctx)
			if err == nil {
				s.tracker.SetStatus("connected")
				s.tracker.SetAuthURL("")
				log.Print("tailnet connected")
				s.fwd.Sync(s.Rules())
				go s.refreshDevices(srv)
				break
			}
			if ctx.Err() != nil {
				log.Print("tailnet Up superseded by restart")
			} else {
				s.tracker.SetStatus("tailnet error: " + err.Error())
			}
			if st != nil && st.AuthURL != "" && ctx.Err() == nil {
				if s.tracker.SetAuthURL(st.AuthURL) {
					log.Printf("login URL: %s", st.AuthURL)
				}
			}
			log.Printf("tailnet Up failed (attempt %d): %v", attempt+1, err)
			wait := time.Duration(min(attempt+1, 5)) * 10 * time.Second
			select {
			case <-s.restart:
				closeTimeout(srv, 10*time.Second)
				continue restartOuter
			case <-time.After(wait):
			}
			s.tracker.SetStatus("connecting to tailnet...")
		}

		<-s.restart
		closeTimeout(srv, 10*time.Second)
	}
}

func (s *Service) userLogf(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	if url, ok := tailnet.LoginURL(text); ok {
		if s.tracker.SetAuthURL(url) {
			log.Printf("login URL: %s", url)
		}
		s.tracker.SetStatus("login needed")
		return
	}
	if tailnet.ShouldSkip(text) {
		return
	}
	log.Print(text)
}

func (s *Service) watchEvents(ctx context.Context, srv *tailnet.Server) {
	onEvent := func(e tailnet.Event) {
		if s.current() != srv {
			return
		}
		s.applyEvent(e)
	}
	if err := srv.Watch(ctx, onEvent); err != nil {
		if s.current() == srv && ctx.Err() == nil {
			log.Printf("tailnet bus watch failed: %v", err)
		}
	}
}

func (s *Service) applyEvent(e tailnet.Event) {
	if e.Connected {
		hadAuth := s.tracker.AuthURL() != ""
		s.tracker.SetAuthURL("")
		s.tracker.SetStatus("connected")
		if hadAuth {
			log.Print("tailnet login approved")
		}
	}
	if e.LoginNeeded {
		s.tracker.SetStatus("login needed")
	}
	if e.ApprovalNeeded {
		s.tracker.SetStatus("approval needed")
	}
	if e.Starting {
		s.tracker.SetStatus("connecting to tailnet...")
	}
	if e.AuthURL != "" {
		if s.tracker.SetAuthURL(e.AuthURL) {
			log.Printf("login URL: %s", e.AuthURL)
		}
		s.tracker.SetStatus("login needed")
	}
}

func (s *Service) refreshDevices(srv *tailnet.Server) {
	if s.current() != srv {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	devices, err := srv.Devices(ctx)
	if err != nil {
		return
	}
	if s.current() != srv {
		return
	}
	s.tracker.SetDevices(devices)
}

func (s *Service) pollDevices(ctx context.Context, srv *tailnet.Server) {
	s.refreshDevices(srv)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.current() != srv {
				return
			}
			s.refreshDevices(srv)
		}
	}
}
