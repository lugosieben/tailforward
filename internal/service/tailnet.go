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
		srv := tailnet.New(s.paths.StateDir, s.userLogf)
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
		go s.watchEvents(srv)
		go s.pollDevices(srv)

		for attempt := 0; ; attempt++ {
			select {
			case <-s.restart:
				_ = srv.Close()
				continue restartOuter
			default:
			}
			st, err := srv.Up(context.Background())
			if err == nil {
				s.tracker.SetStatus("connected")
				s.tracker.SetAuthURL("")
				log.Print("tailnet connected")
				for _, r := range s.Rules() {
					s.fwd.Start(r)
				}
				go s.refreshDevices(srv)
				break
			}
			s.tracker.SetStatus("tailnet error: " + err.Error())
			if st != nil && st.AuthURL != "" {
				if s.tracker.SetAuthURL(st.AuthURL) {
					log.Printf("login URL: %s", st.AuthURL)
				}
			}
			log.Printf("tailnet Up failed (attempt %d): %v", attempt+1, err)
			wait := time.Duration(min(attempt, 5)) * 10 * time.Second
			select {
			case <-s.restart:
				_ = srv.Close()
				continue restartOuter
			case <-time.After(wait):
			}
			s.tracker.SetStatus("connecting to tailnet...")
		}

		<-s.restart
		_ = srv.Close()
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

func (s *Service) watchEvents(srv *tailnet.Server) {
	if err := srv.Watch(context.Background(), s.applyEvent); err != nil {
		if s.current() == srv {
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
	names, err := srv.DeviceNames(ctx)
	if err != nil {
		return
	}
	if s.current() != srv {
		return
	}
	s.tracker.SetDevices(names)
}

func (s *Service) pollDevices(srv *tailnet.Server) {
	s.refreshDevices(srv)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		<-ticker.C
		if s.current() != srv {
			return
		}
		s.refreshDevices(srv)
	}
}
