package state

import "sync"

type Tracker struct {
	mu      sync.Mutex
	status  string
	authURL string
	devices []string
}

func New(initial string) *Tracker { return &Tracker{status: initial} }

func (t *Tracker) SetStatus(s string) {
	t.mu.Lock()
	t.status = s
	t.mu.Unlock()
}

func (t *Tracker) SetAuthURL(u string) bool {
	t.mu.Lock()
	changed := t.authURL != u
	t.authURL = u
	t.mu.Unlock()
	return changed
}

func (t *Tracker) AuthURL() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.authURL
}

func (t *Tracker) SetDevices(d []string) {
	t.mu.Lock()
	t.devices = append([]string{}, d...)
	t.mu.Unlock()
}

func (t *Tracker) Snapshot() (string, string, []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status, t.authURL, append([]string{}, t.devices...)
}
