package server

import (
	"net/http"
	"strings"

	"github.com/shangeethsivan/Syncidian/internal/config"
)

func (s *Server) adminHostConfigured() bool {
	return s.Cfg.AdminHost != ""
}

func (s *Server) isAdminHost(r *http.Request) bool {
	want := strings.TrimSpace(s.Cfg.AdminHost)
	if want == "" || r == nil {
		return false
	}
	return config.Hostname(r.Host) == want
}

func (s *Server) requireAdminHost(w http.ResponseWriter, r *http.Request) bool {
	if !s.adminHostConfigured() {
		return false
	}
	if s.isAdminHost(r) {
		return false
	}
	http.NotFound(w, r)
	return true
}

func (s *Server) operatorPage(path string) bool {
	p := strings.TrimRight(path, "/")
	if p == "" {
		p = "/"
	}
	admin := s.adminPath()
	if p == admin {
		return true
	}
	if admin != "/admin" && p == "/admin" {
		return true
	}
	return false
}

func (s *Server) operatorAPI(r *http.Request) bool {
	path := strings.TrimRight(r.URL.Path, "/")
	switch {
	case r.Method == http.MethodPost && path == "/api/v1/setup":
		return true
	case r.Method == http.MethodGet && path == "/api/v1/waitlist":
		return true
	case r.Method == http.MethodPost && path == "/api/v1/users":
		return true
	case r.Method == http.MethodPost && path == "/api/v1/users/tokens":
		return true
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/api/v1/github/app/register"):
		return true
	default:
		return false
	}
}

// adminHostGate hides the operator UI and admin-only APIs on every Host except
// SYNCIDIAN_ADMIN_HOST. It does not trust X-Forwarded-Host.
func (s *Server) adminHostGate(next http.Handler) http.Handler {
	if !s.adminHostConfigured() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminHost(r) && (s.operatorPage(r.URL.Path) || s.operatorAPI(r)) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostedWaitlistAdmin(r *http.Request) bool {
	publicHost := config.Hostname(strings.TrimPrefix(strings.TrimPrefix(s.Cfg.PublicURL, "https://"), "http://"))
	if i := strings.IndexByte(publicHost, '/'); i >= 0 {
		publicHost = publicHost[:i]
	}
	hosted := hostedPublicHost(publicHost) || s.Cfg.AdminHost == "admin.syncidian.com"
	if !hosted {
		return false
	}
	if s.adminHostConfigured() {
		return s.isAdminHost(r)
	}
	return s.isHostedPublic(r)
}
