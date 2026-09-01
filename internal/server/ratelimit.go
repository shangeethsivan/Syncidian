package server

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type authLimits struct {
	PasswordMax    int
	PasswordWindow time.Duration
	SignupMax      int
	SignupWindow   time.Duration
	SetupMax       int
	SetupWindow    time.Duration
	MaxKeys        int
}

var defaultAuthLimits = authLimits{
	PasswordMax:    10,
	PasswordWindow: 15 * time.Minute,
	SignupMax:      5,
	SignupWindow:   15 * time.Minute,
	SetupMax:       8,
	SetupWindow:    15 * time.Minute,
	MaxKeys:        4096,
}

type authLimiter struct {
	mu      sync.Mutex
	windows map[string][]time.Time
	ops     int
	limits  authLimits
}

func newAuthLimiter(l authLimits) *authLimiter {
	if l.MaxKeys <= 0 {
		l.MaxKeys = 4096
	}
	if l.PasswordMax <= 0 {
		l.PasswordMax = defaultAuthLimits.PasswordMax
	}
	if l.PasswordWindow <= 0 {
		l.PasswordWindow = defaultAuthLimits.PasswordWindow
	}
	if l.SignupMax <= 0 {
		l.SignupMax = defaultAuthLimits.SignupMax
	}
	if l.SignupWindow <= 0 {
		l.SignupWindow = defaultAuthLimits.SignupWindow
	}
	if l.SetupMax <= 0 {
		l.SetupMax = defaultAuthLimits.SetupMax
	}
	if l.SetupWindow <= 0 {
		l.SetupWindow = defaultAuthLimits.SetupWindow
	}
	return &authLimiter{windows: make(map[string][]time.Time), limits: l}
}

func (l *authLimiter) blocked(key string, max int, window time.Duration) (retry time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.pruneLocked(key, now, window)
	hits := l.windows[key]
	if len(hits) < max {
		return 0, false
	}
	return hits[0].Add(window).Sub(now), true
}

func (l *authLimiter) add(key string, window time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.pruneLocked(key, now, window)
	l.windows[key] = append(l.windows[key], now)
	l.ops++
	if l.ops%64 == 0 {
		l.sweepLocked(now)
	}
	l.evictLocked()
}

func (l *authLimiter) clear(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.windows, key)
}

func (l *authLimiter) pruneLocked(key string, now time.Time, window time.Duration) {
	hits := l.windows[key]
	i := 0
	for i < len(hits) && now.Sub(hits[i]) >= window {
		i++
	}
	if i == len(hits) {
		delete(l.windows, key)
		return
	}
	if i > 0 {
		l.windows[key] = append([]time.Time(nil), hits[i:]...)
	}
}

func (l *authLimiter) sweepLocked(now time.Time) {
	longest := l.limits.PasswordWindow
	if l.limits.SignupWindow > longest {
		longest = l.limits.SignupWindow
	}
	if l.limits.SetupWindow > longest {
		longest = l.limits.SetupWindow
	}
	for key, hits := range l.windows {
		i := 0
		for i < len(hits) && now.Sub(hits[i]) >= longest {
			i++
		}
		if i == len(hits) {
			delete(l.windows, key)
			continue
		}
		if i > 0 {
			l.windows[key] = append([]time.Time(nil), hits[i:]...)
		}
	}
}

func (l *authLimiter) evictLocked() {
	for len(l.windows) > l.limits.MaxKeys {
		for key := range l.windows {
			delete(l.windows, key)
			break
		}
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) writeRateLimited(w http.ResponseWriter, retry time.Duration) {
	secs := int(retry.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
}

func (s *Server) denyPassword(w http.ResponseWriter, r *http.Request, username string) bool {
	if s.authLimit == nil {
		return false
	}
	ipKey := "password:ip:" + clientIP(r)
	if retry, ok := s.authLimit.blocked(ipKey, s.authLimit.limits.PasswordMax, s.authLimit.limits.PasswordWindow); ok {
		s.writeRateLimited(w, retry)
		return true
	}
	if username == "" {
		return false
	}
	userKey := "password:user:" + strings.ToLower(username)
	if retry, ok := s.authLimit.blocked(userKey, s.authLimit.limits.PasswordMax, s.authLimit.limits.PasswordWindow); ok {
		s.writeRateLimited(w, retry)
		return true
	}
	return false
}

func (s *Server) notePasswordFail(r *http.Request, username string) {
	if s.authLimit == nil {
		return
	}
	s.authLimit.add("password:ip:"+clientIP(r), s.authLimit.limits.PasswordWindow)
	if username != "" {
		s.authLimit.add("password:user:"+strings.ToLower(username), s.authLimit.limits.PasswordWindow)
	}
}

func (s *Server) notePasswordOK(username string) {
	if s.authLimit == nil || username == "" {
		return
	}
	s.authLimit.clear("password:user:" + strings.ToLower(username))
}

func (s *Server) denySignup(w http.ResponseWriter, r *http.Request) bool {
	if s.authLimit == nil {
		return false
	}
	key := "signup:ip:" + clientIP(r)
	if retry, ok := s.authLimit.blocked(key, s.authLimit.limits.SignupMax, s.authLimit.limits.SignupWindow); ok {
		s.writeRateLimited(w, retry)
		return true
	}
	return false
}

func (s *Server) noteSignup(r *http.Request) {
	if s.authLimit == nil {
		return
	}
	s.authLimit.add("signup:ip:"+clientIP(r), s.authLimit.limits.SignupWindow)
}

func (s *Server) denySetup(w http.ResponseWriter, r *http.Request) bool {
	if s.authLimit == nil {
		return false
	}
	key := "setup:ip:" + clientIP(r)
	if retry, ok := s.authLimit.blocked(key, s.authLimit.limits.SetupMax, s.authLimit.limits.SetupWindow); ok {
		s.writeRateLimited(w, retry)
		return true
	}
	return false
}

func (s *Server) noteSetup(r *http.Request) {
	if s.authLimit == nil {
		return
	}
	s.authLimit.add("setup:ip:"+clientIP(r), s.authLimit.limits.SetupWindow)
}
