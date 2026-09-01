package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/shangeethsivan/Syncidian/internal/store"
)

func hostedPublicHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(host, ".")
	host = strings.TrimPrefix(host, "www.")
	return host == "syncidian.com"
}

func (s *Server) isHostedPublic(r *http.Request) bool {
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	return hostedPublicHost(host)
}

func (s *Server) denyWaitlist(w http.ResponseWriter, r *http.Request) bool {
	if s.authLimit == nil {
		return false
	}
	key := "waitlist:ip:" + clientIP(r)
	if retry, ok := s.authLimit.blocked(key, s.authLimit.limits.SignupMax, s.authLimit.limits.SignupWindow); ok {
		s.writeRateLimited(w, retry)
		return true
	}
	return false
}

func (s *Server) noteWaitlist(r *http.Request) {
	if s.authLimit == nil {
		return
	}
	s.authLimit.add("waitlist:ip:"+clientIP(r), s.authLimit.limits.SignupWindow)
}

func (s *Server) handleJoinWaitlist(w http.ResponseWriter, r *http.Request) {
	if !s.isHostedPublic(r) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && !hostedPublicHost(rHost(origin)) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if s.denyWaitlist(w, r) {
		return
	}
	s.noteWaitlist(r)
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !store.ValidWaitlistEmail(req.Email) {
		writeError(w, http.StatusBadRequest, "enter a valid email")
		return
	}
	if _, err := s.Store.AddWaitlistEmail(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, "enter a valid email")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleListWaitlist(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if !s.isHostedPublic(r) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	list, err := s.Store.ListWaitlist()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, err := s.Store.WaitlistCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": n, "emails": list})
}
