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
	mu             sync.Mutex
	paths          config.Paths
	tracker        *state.Tracker
	fwd            *forward.Manager
	rules          []domain.Rule
	ephemeral      bool
	key            string
	locked         bool
	server         *tailnet.Server
	cancel         context.CancelFunc
	reauthing      bool
	restart        chan struct{}
	restartPending bool
}

func New(paths config.Paths, tracker *state.Tracker) *Service {
	s := &Service{paths: paths, tracker: tracker, ephemeral: true, restart: make(chan struct{}, 1)}
	s.fwd = forward.New(s.dial)
	return s
}

func (s *Service) Rules() []domain.Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Rule{}, s.rules...)
}

func (s *Service) Status() (string, string, []domain.Device) { return s.tracker.Snapshot() }

func (s *Service) Ephemeral() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ephemeral
}

func (s *Service) Encrypted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.key != "" || s.locked
}

func (s *Service) Locked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.locked
}

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
