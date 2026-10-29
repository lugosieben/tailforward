package tailnet

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"tailscale.com/ipn"

	"tailforward/internal/crypt"
)

func StateFile(dir string) string { return filepath.Join(dir, "tailscaled.state") }

type EncryptedStore struct {
	path  string
	key   string
	mu    sync.Mutex
	cache map[ipn.StateKey][]byte
}

func newEncryptedStore(path, key string, raw []byte) (*EncryptedStore, error) {
	s := &EncryptedStore{path: path, key: key, cache: map[ipn.StateKey][]byte{}}
	if len(raw) > 0 {
		plain := raw
		if crypt.IsEncrypted(raw) {
			var err error
			plain, err = crypt.Open(raw, key)
			if err != nil {
				return nil, err
			}
		} else if key != "" {
			if err := crypt.WriteFile(path, plain, key); err != nil {
				return nil, err
			}
		}
		if err := json.Unmarshal(plain, &s.cache); err != nil {
			return nil, err
		}
	} else if key != "" {
		if err := crypt.WriteFile(path, []byte("{}"), key); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *EncryptedStore) ReadState(id ipn.StateKey) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bs, ok := s.cache[id]
	if !ok {
		return nil, ipn.ErrStateNotExist
	}
	return bs, nil
}

func (s *EncryptedStore) WriteState(id ipn.StateKey, bs []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bytes.Equal(s.cache[id], bs) {
		return nil
	}
	if bs == nil {
		delete(s.cache, id)
	} else {
		s.cache[id] = bytes.Clone(bs)
	}
	plain, err := json.MarshalIndent(s.cache, "", "  ")
	if err != nil {
		return err
	}
	return crypt.WriteFile(s.path, plain, s.key)
}

func OpenState(dir, key string) (ipn.StateStore, error) {
	path := StateFile(dir)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if key == "" {
			return nil, nil
		}
		return newEncryptedStore(path, key, nil)
	}
	if err != nil {
		return nil, err
	}
	if key == "" {
		if crypt.IsEncrypted(raw) {
			return nil, crypt.ErrEncrypted
		}
		return nil, nil
	}
	return newEncryptedStore(path, key, raw)
}
