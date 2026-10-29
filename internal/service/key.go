package service

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

	"tailforward/internal/crypt"
	"tailforward/internal/domain"
	"tailforward/internal/store"
	"tailforward/internal/tailnet"
)

var (
	ErrLocked    = errors.New("locked")
	ErrWrongKey  = errors.New("wrong key")
	ErrEncrypted = errors.New("state is already encrypted")
)

func (s *Service) Unlock(key string) error {
	if key == "" {
		return ErrWrongKey
	}
	raw, err := os.ReadFile(tailnet.StateFile(s.paths.StateDir))
	if err != nil || !crypt.IsEncrypted(raw) {
		return ErrWrongKey
	}
	if _, err := tailnet.OpenState(s.paths.StateDir, key); err != nil {
		if errors.Is(err, crypt.ErrBadKey) {
			return ErrWrongKey
		}
		return err
	}
	s.mu.Lock()
	s.key = key
	s.locked = false
	s.mu.Unlock()
	s.restoreFiles(key)
	log.Print("unlocked")
	s.signalRestart("connecting to tailnet...")
	return nil
}

func (s *Service) restoreFiles(key string) {
	loaded := store.Load(s.paths.ConfigFile, key)
	s.mu.Lock()
	s.rules = append([]domain.Rule{}, loaded...)
	s.ephemeral = store.LoadEphemeral(s.paths.SettingsFile, key)
	s.mu.Unlock()
}

func (s *Service) SetKey(key string) error {
	if key == "" {
		return errors.New("empty key")
	}
	s.mu.Lock()
	locked := s.locked
	s.mu.Unlock()
	raw, err := os.ReadFile(tailnet.StateFile(s.paths.StateDir))
	if err == nil && crypt.IsEncrypted(raw) {
		if locked {
			return ErrLocked
		}
		return ErrEncrypted
	}
	return s.applyKey(key, "encryption enabled")
}

func (s *Service) ChangeKey(old, new string) error {
	if old == "" || new == "" {
		return errors.New("empty key")
	}
	s.mu.Lock()
	if s.key != old {
		s.mu.Unlock()
		return ErrWrongKey
	}
	if _, err := tailnet.OpenState(s.paths.StateDir, old); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return s.applyKey(new, "encryption key changed")
}

func (s *Service) RemoveKey(key string) error {
	s.mu.Lock()
	if s.key != key {
		s.mu.Unlock()
		return ErrWrongKey
	}
	s.mu.Unlock()
	return s.applyKey("", "encryption removed")
}

func (s *Service) Lock() error {
	s.mu.Lock()
	if s.reauthing {
		s.mu.Unlock()
		return ErrReauthInProgress
	}
	if s.key == "" {
		s.mu.Unlock()
		return nil
	}
	s.key = ""
	s.locked = true
	s.mu.Unlock()
	log.Print("locked")
	s.signalRestart("locked")
	return nil
}

func (s *Service) applyKey(newKey, reason string) error {
	s.mu.Lock()
	if s.reauthing {
		s.mu.Unlock()
		return ErrReauthInProgress
	}
	oldKey := s.key
	s.reauthing = true
	old := s.server
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.reauthing = false
		s.mu.Unlock()
	}()

	s.cancelCurrent()
	if old != nil {
		closeTimeout(old, 10*time.Second)
	}
	if err := s.rekeyFiles(oldKey, newKey); err != nil {
		log.Printf("rekey failed, keeping old key: %v", err)
		s.signalRestart("connecting to tailnet...")
		return err
	}
	s.mu.Lock()
	s.key = newKey
	s.mu.Unlock()
	s.tracker.SetStatus("connecting to tailnet...")
	s.tracker.SetAuthURL("")
	log.Print(reason)
	s.requestRestart()
	return nil
}

func (s *Service) rekeyFiles(oldKey, newKey string) error {
	statePlain, err := rekeyState(s.paths.StateDir, oldKey, newKey)
	if err != nil {
		return err
	}
	s.mu.Lock()
	rules := append([]domain.Rule{}, s.rules...)
	ephemeral := s.ephemeral
	s.mu.Unlock()

	rulesJSON, _ := json.MarshalIndent(rules, "", "  ")
	if err := crypt.WriteFile(s.paths.ConfigFile, rulesJSON, newKey); err != nil {
		s.rollbackState(statePlain, oldKey)
		return err
	}
	ephJSON, _ := json.MarshalIndent(map[string]any{"ephemeral": ephemeral}, "", "  ")
	if err := crypt.WriteFile(s.paths.SettingsFile, ephJSON, newKey); err != nil {
		_ = crypt.WriteFile(s.paths.ConfigFile, rulesJSON, oldKey)
		s.rollbackState(statePlain, oldKey)
		return err
	}
	return nil
}

func (s *Service) rollbackState(statePlain []byte, oldKey string) {
	if statePlain == nil {
		return
	}
	if err := crypt.WriteFile(tailnet.StateFile(s.paths.StateDir), statePlain, oldKey); err != nil {
		log.Printf("rekey rollback: cannot restore state file, the new key may be required to unlock: %v", err)
	}
}

func rekeyState(dir, oldKey, newKey string) ([]byte, error) {
	path := tailnet.StateFile(dir)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if newKey == "" {
			return nil, nil
		}
		return []byte("{}"), crypt.WriteFile(path, []byte("{}"), newKey)
	}
	if err != nil {
		return nil, err
	}
	plain := raw
	if crypt.IsEncrypted(raw) {
		if plain, err = crypt.Open(raw, oldKey); err != nil {
			return nil, err
		}
	}
	return plain, crypt.WriteFile(path, plain, newKey)
}

func (s *Service) signalRestart(status string) {
	s.cancelCurrent()
	s.tracker.SetStatus(status)
	s.tracker.SetAuthURL("")
	s.requestRestart()
}
