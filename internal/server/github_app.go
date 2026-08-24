package server

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/shangeethsivan/Syncidian/internal/githubapp"
	"github.com/shangeethsivan/Syncidian/internal/store"
)

const githubStateCookie = "syncidian_github_state"

func (s *Server) githubBrowserAuthed(fn func(http.ResponseWriter, *http.Request, *store.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.authenticate(r)
		if err != nil || user == nil {
			s.dashboardRedirect(w, r, url.Values{
				"github":  {"error"},
				"message": {"Sign in first, then connect GitHub."},
			})
			return
		}
		if user.IsAdmin {
			s.dashboardRedirect(w, r, url.Values{
				"github":  {"error"},
				"message": {"Admins manage users and cannot connect GitHub."},
			})
			return
		}
		fn(w, r, user)
	}
}

func (s *Server) handleGitHubAppStart(w http.ResponseWriter, r *http.Request, u *store.User) {
	if cfg, _ := s.Store.GetGitHub(u.ID); cfg.HasApp() && cfg.AppSlug != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"github_url": "https://github.com/apps/" + cfg.AppSlug + "/installations/new",
			"existing":   true,
			"branch":     GitHubBranch,
		})
		return
	}
	state, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start GitHub App setup")
		return
	}
	s.setGitHubStateCookie(w, r, state)
	base := s.requestBase(r)
	name := "Syncidian-" + strings.ReplaceAll(u.ID, "-", "")
	if len(name) > 34 {
		name = name[:34]
	}
	manifest := githubapp.NewManifest(base, name)
	writeJSON(w, http.StatusOK, map[string]any{
		"github_url": "https://github.com/settings/apps/new?state=" + url.QueryEscape(state),
		"manifest":   manifest,
		"branch":     GitHubBranch,
	})
}

func (s *Server) handleGitHubAppCallback(w http.ResponseWriter, r *http.Request, u *store.User) {
	if msg := r.URL.Query().Get("error_description"); msg == "" {
		msg = r.URL.Query().Get("error")
		if msg != "" {
			s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {msg}})
			return
		}
	} else {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {msg}})
		return
	}
	if !s.validGitHubState(r, r.URL.Query().Get("state")) {
		s.dashboardRedirect(w, r, url.Values{
			"github":  {"error"},
			"message": {"GitHub App setup expired. Click Connect with GitHub again."},
		})
		return
	}
	creds, err := githubapp.ConvertManifest(r.URL.Query().Get("code"))
	if err != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
		return
	}
	cfg := &store.GitHubConfig{
		UserID:       u.ID,
		Branch:       GitHubBranch,
		AppID:        creds.ID,
		AppSlug:      creds.Slug,
		AppPEM:       creds.PEM,
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
	}
	if err := s.Store.SetGitHub(*cfg); err != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
		return
	}
	_ = s.Store.AddActivity(store.Activity{UserID: u.ID, Action: "github.app", Detail: creds.Slug})
	http.Redirect(w, r, "https://github.com/apps/"+url.PathEscape(creds.Slug)+"/installations/new", http.StatusFound)
}

func (s *Server) handleGitHubAppSetup(w http.ResponseWriter, r *http.Request, u *store.User) {
	if r.URL.Query().Get("setup_action") == "request" {
		s.dashboardRedirect(w, r, url.Values{
			"github":  {"error"},
			"message": {"The GitHub App installation was not approved."},
		})
		return
	}
	installationID, err := strconv.ParseInt(r.URL.Query().Get("installation_id"), 10, 64)
	if err != nil || installationID == 0 {
		s.dashboardRedirect(w, r, url.Values{
			"github":  {"error"},
			"message": {"GitHub did not return an installation. Click Install on one repository."},
		})
		return
	}
	cfg, err := s.Store.GetGitHub(u.ID)
	if err != nil || !cfg.HasApp() {
		s.dashboardRedirect(w, r, url.Values{
			"github":  {"error"},
			"message": {"Start from Connect with GitHub so Syncidian can create the app first."},
		})
		return
	}
	cfg.InstallationID = installationID
	if err := s.Store.SetGitHub(*cfg); err != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
		return
	}
	repos, listErr := s.listInstallRepos(cfg)
	if listErr != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {listErr.Error()}})
		return
	}
	if len(repos) == 0 {
		s.dashboardRedirect(w, r, url.Values{
			"github":  {"error"},
			"message": {"Install the GitHub App on at least one repository, and allow write access."},
		})
		return
	}
	if len(repos) == 1 {
		if err := s.bindGitHubRepo(u, cfg, repos[0].FullName); err != nil {
			s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
			return
		}
		_ = s.Store.AddActivity(store.Activity{UserID: u.ID, Action: "github.configure", Detail: cfg.Repo})
		_, _ = s.commitAndMaybePush(u.ID, "Syncidian: connect GitHub")
		s.dashboardRedirect(w, r, url.Values{"github": {"connected"}})
		return
	}
	s.dashboardRedirect(w, r, url.Values{"github": {"select"}})
}

func (s *Server) handleGitHubAppWebhook(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) requestBase(r *http.Request) string {
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	proto := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		proto = "https"
	}
	return proto + "://" + host
}

func (s *Server) dashboardRedirect(w http.ResponseWriter, r *http.Request, vals url.Values) {
	loc := s.requestBase(r) + "/"
	if encoded := vals.Encode(); encoded != "" {
		loc += "?" + encoded
	}
	http.Redirect(w, r, loc, http.StatusFound)
}

func (s *Server) setGitHubStateCookie(w http.ResponseWriter, r *http.Request, state string) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     githubStateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   600,
	})
}

func (s *Server) validGitHubState(r *http.Request, state string) bool {
	c, err := r.Cookie(githubStateCookie)
	if err != nil || c.Value == "" || state == "" {
		return false
	}
	return c.Value == state
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
