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

const (
	dialTimeout    = 15 * time.Second
	retryInterval  = 30 * time.Second
	dialLogEvery   = 10 * time.Second
	dialFailWindow = time.Minute
	udpIdleTimeout = 5 * time.Minute
	udpBufferSize  = 65536
)

type portSpec struct {
	local  int
	remote string
}

type dialKey struct {
	p     int
	proto string
}

type Manager struct {
	mu           sync.Mutex
	syncMu       sync.Mutex
	desired      map[int]portSpec
	tcp          map[int]net.Listener
	udp          map[int]*net.UDPConn
	listenFail   map[int]string
	dialFail     map[dialKey]string
	dialFailTime map[dialKey]time.Time
	dial         DialFunc
}

func New(dial DialFunc) *Manager {
	m := &Manager{
		desired:      map[int]portSpec{},
		tcp:          map[int]net.Listener{},
		udp:          map[int]*net.UDPConn{},
		listenFail:   map[int]string{},
		dialFail:     map[dialKey]string{},
		dialFailTime: map[dialKey]time.Time{},
		dial:         dial,
	}
	go m.retryLoop()
	return m
}

func (m *Manager) Sync(rules []domain.Rule) {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	next := make(map[int]portSpec, len(rules))
	for _, r := range rules {
		for off := 0; off <= r.RemoteEnd-r.RemoteStart; off++ {
			next[r.LocalStart+off] = portSpec{
				local:  r.LocalStart + off,
				remote: fmt.Sprintf("%s:%d", r.Host, r.RemoteStart+off),
			}
		}
	}

	m.mu.Lock()
	var stop, start []portSpec
	for p, s := range m.desired {
		if ns, ok := next[p]; !ok || ns.remote != s.remote {
			stop = append(stop, s)
		}
	}
	for p, s := range next {
		if os, ok := m.desired[p]; !ok || os.remote != s.remote {
			start = append(start, s)
		}
	}
	m.desired = next
	m.mu.Unlock()

	for _, s := range stop {
		m.stopPort(s.local)
	}
	for _, s := range start {
		m.ensurePort(s.local, s.remote)
	}
}

func (m *Manager) stopPort(p int) {
	m.mu.Lock()
	ln := m.tcp[p]
	up := m.udp[p]
	delete(m.tcp, p)
	delete(m.udp, p)
	delete(m.listenFail, p)
	for _, proto := range []string{"tcp", "udp"} {
		delete(m.dialFail, dialKey{p, proto})
		delete(m.dialFailTime, dialKey{p, proto})
	}
	m.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	if up != nil {
		_ = up.Close()
	}
}

func (m *Manager) ensurePort(p int, remote string) {
	m.mu.Lock()
	needTCP := m.tcp[p] == nil
	needUDP := m.udp[p] == nil
	m.mu.Unlock()
	if needTCP {
		m.bindTCP(p, remote)
	}
	if needUDP {
		m.bindUDP(p, remote)
	}
}

func (m *Manager) bindTCP(p int, remote string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tcp[p] != nil {
		return
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
	if err != nil {
		m.listenFail[p] = "listen tcp: " + err.Error()
		return
	}
	m.tcp[p] = ln
	if m.udp[p] != nil {
		delete(m.listenFail, p)
	}
	go m.acceptLoop(ln, p, remote)
}

func (m *Manager) bindUDP(p int, remote string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.udp[p] != nil {
		return
	}
	pc, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", p))
	if err != nil {
		m.listenFail[p] = "listen udp: " + err.Error()
		return
	}
	uc, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		m.listenFail[p] = "listen udp: unexpected conn type"
		return
	}
	m.udp[p] = uc
	if m.tcp[p] != nil {
		delete(m.listenFail, p)
	}
	go m.serveUDP(uc, p, remote)
}

func (m *Manager) retryLoop() {
	t := time.NewTicker(retryInterval)
	for range t.C {
		m.mu.Lock()
		specs := make([]portSpec, 0, len(m.desired))
		for _, s := range m.desired {
			specs = append(specs, s)
		}
		m.mu.Unlock()
		m.syncMu.Lock()
		for _, s := range specs {
			m.ensurePort(s.local, s.remote)
		}
		m.syncMu.Unlock()
	}
}

func (m *Manager) Health(r domain.Rule) (active, total int, problem string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	total = r.RemoteEnd - r.RemoteStart + 1
	var parts []string
	for p := r.LocalStart; p <= r.LocalEnd(); p++ {
		if m.tcp[p] != nil || m.udp[p] != nil {
			active++
		}
		recentDial := func() (string, bool) {
			for _, proto := range []string{"tcp", "udp"} {
				k := dialKey{p, proto}
				if d, ok := m.dialFail[k]; ok && time.Since(m.dialFailTime[k]) < dialFailWindow {
					return d, true
				}
			}
			return "", false
		}
		var msg string
		if m.tcp[p] == nil || m.udp[p] == nil {
			if l, ok := m.listenFail[p]; ok {
				msg = l
			} else if d, ok := recentDial(); ok {
				msg = d
			}
		} else if d, ok := recentDial(); ok {
			msg = d
		}
		if msg != "" {
			parts = append(parts, fmt.Sprintf(":%d %s", p, msg))
		}
	}
	return active, total, strings.Join(parts, "; ")
}

func (m *Manager) acceptLoop(ln net.Listener, p int, remote string) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go m.serveTCP(c, p, remote)
	}
}

func (m *Manager) serveTCP(c net.Conn, p int, remote string) {
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	up, err := m.dial(ctx, "tcp", remote)
	cancel()
	if err != nil {
		m.noteDialFailure(p, "tcp", remote, err)
		return
	}
	m.clearDialFailure(p, "tcp")
	defer func() { _ = up.Close() }()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(up, c)
		closeWrite(up)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(c, up)
		closeWrite(c)
	}()
	wg.Wait()
}

func (m *Manager) serveUDP(local *net.UDPConn, p int, remote string) {
	var mu sync.Mutex
	relays := map[string]*udpRelay{}
	var wg sync.WaitGroup
	buf := make([]byte, udpBufferSize)

	reap := func(key string) {
		mu.Lock()
		if r := relays[key]; r != nil {
			delete(relays, key)
			r.kill()
		}
		mu.Unlock()
	}

	defer func() {
		mu.Lock()
		for _, r := range relays {
			r.kill()
		}
		mu.Unlock()
		wg.Wait()
	}()

	for {
		n, client, err := local.ReadFromUDP(buf)
		if err != nil {
			return
		}
		key := client.String()
		mu.Lock()
		r := relays[key]
		isNew := r == nil
		if isNew {
			r = &udpRelay{timer: time.AfterFunc(udpIdleTimeout, func() { reap(key) })}
			relays[key] = r
		}
		mu.Unlock()
		if isNew {
			wg.Add(1)
			go func(addr *net.UDPAddr, r *udpRelay) {
				defer wg.Done()
				m.pumpRemoteUDP(local, p, remote, addr, r, func() { reap(key) })
			}(client, r)
		}
		if r.send(buf[:n]) != nil {
			if r.isDead() {
				reap(key)
			}
		}
	}
}

func (m *Manager) pumpRemoteUDP(local *net.UDPConn, p int, remote string, client *net.UDPAddr, r *udpRelay, onGone func()) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	up, err := m.dial(ctx, "udp", remote)
	cancel()
	if err != nil {
		m.noteDialFailure(p, "udp", remote, err)
		r.kill()
		onGone()
		return
	}
	pc, ok := up.(net.PacketConn)
	if !ok {
		_ = up.Close()
		r.kill()
		onGone()
		return
	}
	if !r.setConn(pc) {
		_ = pc.Close()
		onGone()
		return
	}
	m.clearDialFailure(p, "udp")
	rbuf := make([]byte, udpBufferSize)
	for {
		n, _, err := pc.ReadFrom(rbuf)
		if err != nil {
			r.kill()
			onGone()
			return
		}
		if _, err := local.WriteToUDP(rbuf[:n], client); err != nil {
			r.kill()
			onGone()
			return
		}
	}
}

type udpRelay struct {
	mu    sync.Mutex
	conn  net.PacketConn
	timer *time.Timer
	dead  bool
}

func (r *udpRelay) send(b []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dead || r.conn == nil {
		return fmt.Errorf("relay not ready")
	}
	r.timer.Reset(udpIdleTimeout)
	w, ok := r.conn.(interface{ Write([]byte) (int, error) })
	if !ok {
		r.dead = true
		return fmt.Errorf("upstream conn has no Write")
	}
	_, err := w.Write(b)
	if err != nil {
		r.dead = true
	}
	return err
}

func (r *udpRelay) setConn(pc net.PacketConn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dead {
		return false
	}
	r.conn = pc
	return true
}

func (r *udpRelay) isDead() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dead
}

func (r *udpRelay) kill() {
	r.mu.Lock()
	if r.dead {
		r.mu.Unlock()
		return
	}
	r.dead = true
	r.timer.Stop()
	if r.conn != nil {
		_ = r.conn.Close()
	}
	r.mu.Unlock()
}

func (m *Manager) noteDialFailure(p int, proto, remote string, err error) {
	msg := fmt.Sprintf("dial %s: %v", remote, err)
	k := dialKey{p, proto}
	m.mu.Lock()
	last, ok := m.dialFailTime[k]
	m.dialFail[k] = msg
	m.dialFailTime[k] = time.Now()
	shouldLog := !ok || time.Since(last) >= dialLogEvery
	m.mu.Unlock()
	if shouldLog {
		log.Printf("local :%d: %s", p, msg)
	}
}

func (m *Manager) clearDialFailure(p int, proto string) {
	m.mu.Lock()
	delete(m.dialFail, dialKey{p, proto})
	m.mu.Unlock()
}

func closeWrite(c net.Conn) {
	if hw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = hw.CloseWrite()
	}
}
