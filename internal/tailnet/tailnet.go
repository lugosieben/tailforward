package tailnet

import (
	"context"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"

	"tailforward/internal/domain"
)

var loginPattern = regexp.MustCompile(`https://login\.tailscale\.com/a/[0-9a-f]+`)

type Event struct {
	Connected      bool
	LoginNeeded    bool
	ApprovalNeeded bool
	Starting       bool
	AuthURL        string
}

type Server struct {
	inner *tsnet.Server
}

func New(dir string, ephemeral bool, userLogf func(string, ...any), store ipn.StateStore) *Server {
	srv := &tsnet.Server{
		Hostname:  "tailforward",
		Dir:       dir,
		Ephemeral: ephemeral,
		AuthKey:   strings.TrimSpace(os.Getenv("TS_AUTHKEY")),
		Logf:      func(string, ...any) {},
		UserLogf:  userLogf,
	}
	if store != nil {
		srv.Store = store
	}
	return &Server{inner: srv}
}

func LoginURL(text string) (string, bool) {
	m := loginPattern.FindString(text)
	return m, m != ""
}

func Devices(st *ipnstate.Status) []domain.Device {
	if st == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []domain.Device
	add := func(host, dns string, online bool) {
		n := host
		if n == "" {
			n = strings.TrimSuffix(dns, ".")
		}
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, domain.Device{Name: n, Online: online})
		}
	}
	if st.Self != nil {
		add(st.Self.HostName, st.Self.DNSName, st.Self.Online)
	}
	for _, p := range st.Peer {
		if p != nil {
			add(p.HostName, p.DNSName, p.Online)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Server) Start() error {
	_, err := s.inner.LocalClient()
	return err
}

func (s *Server) Up(ctx context.Context) (*ipnstate.Status, error) {
	return s.inner.Up(ctx)
}

func (s *Server) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return s.inner.Dial(ctx, network, address)
}

func (s *Server) Close() error { return s.inner.Close() }

func (s *Server) Logout(ctx context.Context) error {
	client, err := s.inner.LocalClient()
	if err != nil {
		return err
	}
	return client.Logout(ctx)
}

func (s *Server) Devices(ctx context.Context) ([]domain.Device, error) {
	client, err := s.inner.LocalClient()
	if err != nil {
		return nil, err
	}
	st, err := client.Status(ctx)
	if err != nil {
		return nil, err
	}
	return Devices(st), nil
}

func (s *Server) Watch(ctx context.Context, onEvent func(Event)) error {
	client, err := s.inner.LocalClient()
	if err != nil {
		return err
	}
	watcher, err := client.WatchIPNBus(ctx, ipn.NotifyInitialState)
	if err != nil {
		return err
	}
	defer func() { _ = watcher.Close() }()
	for {
		n, err := watcher.Next()
		if err != nil {
			return err
		}
		var e Event
		empty := true
		if n.State != nil {
			switch *n.State {
			case ipn.Running:
				e.Connected = true
				empty = false
			case ipn.NeedsLogin:
				e.LoginNeeded = true
				empty = false
			case ipn.NeedsMachineAuth:
				e.ApprovalNeeded = true
				empty = false
			case ipn.Starting:
				e.Starting = true
				empty = false
			}
		}
		if n.BrowseToURL != nil && *n.BrowseToURL != "" {
			e.AuthURL = *n.BrowseToURL
			empty = false
		}
		if !empty {
			onEvent(e)
		}
	}
}

func ShouldSkip(text string) bool {
	return strings.HasPrefix(text, "To start this tsnet server") ||
		strings.HasPrefix(text, "AuthLoop: state is")
}
