package forward

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"tailforward/internal/domain"
)

type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

type Manager struct {
	mu        sync.Mutex
	listeners map[int]net.Listener
	failures  map[int]string
	dial      DialFunc
}

func New(dial DialFunc) *Manager {
	return &Manager{listeners: map[int]net.Listener{}, failures: map[int]string{}, dial: dial}
}

func (m *Manager) Start(r domain.Rule) {
	for off := 0; off <= r.RemoteEnd-r.RemoteStart; off++ {
		m.StartPort(r.LocalStart+off, fmt.Sprintf("%s:%d", r.Host, r.RemoteStart+off))
	}
}

func (m *Manager) StartPort(local int, remote string) {
	m.mu.Lock()
	if _, ok := m.listeners[local]; ok {
		m.mu.Unlock()
		return
	}
	delete(m.failures, local)
	m.mu.Unlock()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", local))
	if err != nil {
		m.mu.Lock()
		m.failures[local] = "listen: " + err.Error()
		m.mu.Unlock()
		log.Printf("local :%d (%s) failed to listen: %v", local, remote, err)
		return
	}
	m.mu.Lock()
	m.listeners[local] = ln
	m.mu.Unlock()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go m.serve(c, remote)
		}
	}()
}

func (m *Manager) Stop(r domain.Rule) {
	for p := r.LocalStart; p <= r.LocalEnd(); p++ {
		m.mu.Lock()
		ln, ok := m.listeners[p]
		if ok {
			delete(m.listeners, p)
		}
		delete(m.failures, p)
		m.mu.Unlock()
		if ok {
			_ = ln.Close()
		}
	}
}

func (m *Manager) Health(r domain.Rule) (active, total int, problem string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	total = r.RemoteEnd - r.RemoteStart + 1
	var parts []string
	for p := r.LocalStart; p <= r.LocalEnd(); p++ {
		if _, ok := m.listeners[p]; ok {
			active++
		} else if msg, ok := m.failures[p]; ok {
			parts = append(parts, fmt.Sprintf(":%d %s", p, msg))
		}
	}
	return active, total, strings.Join(parts, "; ")
}

func (m *Manager) serve(local net.Conn, remote string) {
	defer func() { _ = local.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	up, err := m.dial(ctx, "tcp", remote)
	if err != nil {
		return
	}
	defer func() { _ = up.Close() }()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(up, local)
		closeWrite(up)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(local, up)
		closeWrite(local)
	}()
	wg.Wait()
}

func closeWrite(c net.Conn) {
	if hw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = hw.CloseWrite()
	}
}
