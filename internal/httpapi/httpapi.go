package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"tailforward/internal/domain"
	"tailforward/internal/service"
	"tailforward/web"
)

type ruleRequest struct {
	Action        string `json:"action"`
	Host          string `json:"host"`
	RemoteStart   int    `json:"remoteStart"`
	RemoteEnd     int    `json:"remoteEnd"`
	LocalStart    int    `json:"localStart"`
	OldLocalStart int    `json:"oldLocalStart"`
}

type ruleEntry struct {
	Label       string `json:"label"`
	Host        string `json:"host"`
	RemoteStart int    `json:"remoteStart"`
	RemoteEnd   int    `json:"remoteEnd"`
	LocalStart  int    `json:"localStart"`
	Active      int    `json:"active"`
	Total       int    `json:"total"`
	Error       string `json:"error,omitempty"`
}

type stateResponse struct {
	Status    string          `json:"status"`
	AuthURL   string          `json:"authURL"`
	Rules     []ruleEntry     `json:"rules"`
	Devices   []domain.Device `json:"devices"`
	Ephemeral bool            `json:"ephemeral"`
	Encrypted bool            `json:"encrypted"`
	Locked    bool            `json:"locked"`
}

type keyRequest struct {
	Action string `json:"action"`
	Key    string `json:"key"`
	OldKey string `json:"oldKey"`
}

type modeRequest struct {
	Ephemeral *bool `json:"ephemeral"`
}

type Server struct {
	service *service.Service
}

func New(svc *service.Service) *Server { return &Server{service: svc} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/qrcode.min.js", s.handleScript)
	mux.HandleFunc("/favicon.svg", s.handleFavicon)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/rules", s.handleRules)
	mux.HandleFunc("/api/reauth", s.handleReauth)
	mux.HandleFunc("/api/mode", s.handleMode)
	mux.HandleFunc("/api/key", s.handleKey)
	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, web.UI)
}

func (s *Server) handleScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, web.QR)
}

func (s *Server) handleFavicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, web.Favicon)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	status, authURL, devices := s.service.Status()
	entries := make([]ruleEntry, 0)
	for _, rule := range s.service.Rules() {
		active, total, failure := s.service.Health(rule)
		entries = append(entries, ruleEntry{
			Label:       rule.Label(),
			Host:        rule.Host,
			RemoteStart: rule.RemoteStart,
			RemoteEnd:   rule.RemoteEnd,
			LocalStart:  rule.LocalStart,
			Active:      active,
			Total:       total,
			Error:       failure,
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stateResponse{
		Status: status, AuthURL: authURL, Rules: entries, Devices: devices,
		Ephemeral: s.service.Ephemeral(), Encrypted: s.service.Encrypted(), Locked: s.service.Locked(),
	})
}

func (s *Server) guardLocked(w http.ResponseWriter) bool {
	if s.service.Locked() {
		http.Error(w, "locked", http.StatusLocked)
		return false
	}
	return true
}

func (s *Server) keyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrLocked):
		http.Error(w, err.Error(), http.StatusLocked)
	case errors.Is(err, service.ErrWrongKey):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, service.ErrReauthInProgress):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, service.ErrEncrypted):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}

func (s *Server) handleKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req keyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	var err error
	switch req.Action {
	case "unlock":
		err = s.service.Unlock(req.Key)
	case "set":
		err = s.service.SetKey(req.Key)
	case "change":
		err = s.service.ChangeKey(req.OldKey, req.Key)
	case "remove":
		err = s.service.RemoveKey(req.Key)
	case "lock":
		err = s.service.Lock()
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	if err != nil {
		s.keyError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"ok":true}`)
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if !s.guardLocked(w) {
		return
	}
	var req ruleRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	switch req.Action {
	case "add":
		if _, err := s.service.Add(req.Host, req.RemoteStart, req.RemoteEnd, req.LocalStart); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	case "update":
		if _, err := s.service.Update(req.OldLocalStart, req.Host, req.RemoteStart, req.RemoteEnd, req.LocalStart); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	case "delete":
		s.service.Delete(req.LocalStart)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"ok":true}`)
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if !s.guardLocked(w) {
		return
	}
	var req modeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Ephemeral == nil {
		http.Error(w, "missing ephemeral", http.StatusBadRequest)
		return
	}
	changed, err := s.service.SetEphemeral(*req.Ephemeral)
	if err != nil {
		if errors.Is(err, service.ErrReauthInProgress) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ephemeral": s.service.Ephemeral(), "changed": changed})
}

func (s *Server) handleReauth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if !s.guardLocked(w) {
		return
	}
	if err := s.service.Reauth(); err != nil {
		if errors.Is(err, service.ErrReauthInProgress) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"ok":true}`)
}
