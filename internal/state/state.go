package state

import (
	"sync"

	"tailforward/internal/domain"
)

type Tracker struct {
	mu      sync.Mutex
	status  string
	authURL string
	devices []domain.Device
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

func (t *Tracker) SetDevices(d []domain.Device) {
	t.mu.Lock()
	t.devices = append([]domain.Device{}, d...)
	t.mu.Unlock()
}

func (t *Tracker) Snapshot() (string, string, []domain.Device) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status, t.authURL, append([]domain.Device{}, t.devices...)
}
