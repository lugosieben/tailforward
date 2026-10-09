package domain

import (
	"errors"
	"fmt"
	"strings"
)

const (
	DashboardPort = 8971
	MaxRange      = 256
	MaxPort       = 65535
	MinUserPort   = 1024
)

type Rule struct {
	Host        string `json:"host"`
	RemoteStart int    `json:"remoteStart"`
	RemoteEnd   int    `json:"remoteEnd"`
	LocalStart  int    `json:"localStart"`
}

func (r Rule) LocalEnd() int { return r.LocalStart + r.RemoteEnd - r.RemoteStart }

func (r Rule) Label() string {
	if r.RemoteStart == r.RemoteEnd {
		return fmt.Sprintf("%s:%d → %d", r.Host, r.RemoteStart, r.LocalStart)
	}
	return fmt.Sprintf("%s:%d-%d → %d-%d", r.Host, r.RemoteStart, r.RemoteEnd, r.LocalStart, r.LocalEnd())
}

func Overlaps(a, b Rule) bool { return a.LocalStart <= b.LocalEnd() && b.LocalStart <= a.LocalEnd() }

func Validate(host string, rs, re, ls int, existing []Rule) (Rule, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return Rule{}, errors.New("tailnet host required")
	}
	if rs < 1 || rs > MaxPort || re < 1 || re > MaxPort || re < rs {
		return Rule{}, errors.New("remote range must be start-end within 1-65535")
	}
	if re-rs+1 > MaxRange {
		return Rule{}, fmt.Errorf("max %d ports per rule", MaxRange)
	}
	r := Rule{Host: host, RemoteStart: rs, RemoteEnd: re, LocalStart: ls}
	if ls < MinUserPort || ls > MaxPort || r.LocalEnd() > MaxPort {
		return Rule{}, errors.New("local ports must be start-end within 1024-65535")
	}
	for p := ls; p <= r.LocalEnd(); p++ {
		if p == DashboardPort {
			return Rule{}, fmt.Errorf("local port %d is the dashboard port", DashboardPort)
		}
	}
	for _, o := range existing {
		if o.LocalStart == ls {
			continue // replacing this one
		}
		if Overlaps(r, o) {
			return Rule{}, fmt.Errorf("local ports overlap %s", o.Label())
		}
	}
	return r, nil
}
