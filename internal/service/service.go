package service

import (
	"context"
	"errors"
	"net"
	"sync"

	"tailforward/internal/config"
	"tailforward/internal/domain"
	"tailforward/internal/forward"
	"tailforward/internal/state"
	"tailforward/internal/tailnet"
)

type Service struct {
	mu        sync.Mutex
	paths     config.Paths
	tracker   *state.Tracker
	fwd       *forward.Manager
	rules     []domain.Rule
	server    *tailnet.Server
	reauthing bool
	restart   chan struct{}
}

func New(paths config.Paths, tracker *state.Tracker) *Service {
	s := &Service{paths: paths, tracker: tracker, restart: make(chan struct{}, 1)}
	s.fwd = forward.New(s.dial)
	return s
}

func (s *Service) Rules() []domain.Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Rule{}, s.rules...)
}

func (s *Service) Status() (string, string, []string) { return s.tracker.Snapshot() }

func (s *Service) Health(r domain.Rule) (int, int, string) { return s.fwd.Health(r) }

func (s *Service) current() *tailnet.Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.server
}

func (s *Service) dial(ctx context.Context, network, address string) (net.Conn, error) {
	srv := s.current()
	if srv == nil {
		return nil, errors.New("tailnet not started yet")
	}
	return srv.Dial(ctx, network, address)
}
