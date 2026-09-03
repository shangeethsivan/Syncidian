package server

import (
	"crypto/rand"
	"crypto/subtle"
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

func githubErr(r *http.Request) string {
	if msg := r.URL.Query().Get("error_description"); msg != "" {
		return msg
	}
	return r.URL.Query().Get("error")
}

func (s *Server) adminAuthed(fn func(http.ResponseWriter, *http.Request, *store.User)) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if s.requireAdminHost(w, r) {
			return
		}
		if !u.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}
		fn(w, r, u)
	})
}

func (s *Server) handleGitHubAppRegisterStart(w http.ResponseWriter, r *http.Request, u *store.User) {
	state, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start GitHub App registration")
		return
	}
	s.setGitHubStateCookie(w, r, state)
	base := s.githubBase(r)
	manifest := githubapp.NewManifest(base, "Syncidian")
	writeJSON(w, http.StatusOK, map[string]any{
		"github_url": "https://github.com/settings/apps/new?state=" + url.QueryEscape(state),
		"manifest":   manifest,
		"urls":       githubapp.AppURLs(base),
		"branch":     GitHubBranch,
	})
}

func (s *Server) handleGitHubAppRegisterSave(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		AppID        int64  `json:"app_id"`
		Slug         string `json:"slug"`
		PEM          string `json:"pem"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Slug = githubapp.NormalizeAppSlug(req.Slug)
	req.PEM = strings.TrimSpace(req.PEM)
	req.ClientID = strings.TrimSpace(req.ClientID)
	req.ClientSecret = strings.TrimSpace(req.ClientSecret)
	if req.AppID == 0 || req.PEM == "" || req.ClientID == "" || req.ClientSecret == "" {
		writeError(w, http.StatusBadRequest, "app_id, pem, client_id, and client_secret are required")
		return
	}
	if req.Slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required (e.g. syncidian from github.com/apps/syncidian)")
		return
	}
	if err := s.Store.SetInstanceGitHubApp(store.GitHubApp{
		AppID: req.AppID, Slug: req.Slug, PEM: req.PEM, ClientID: req.ClientID, ClientSecret: req.ClientSecret,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slug": req.Slug})
}

func (s *Server) handleGitHubAppStart(w http.ResponseWriter, r *http.Request, u *store.User) {
	state, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start GitHub App setup")
		return
	}
	s.setGitHubStateCookie(w, r, state)
	// Mark this browser as waiting for an install return. With
	// "Request user authorization during installation", GitHub redirects to the
	// OAuth callback URL (not the setup URL) with installation_id.
	s.setPendingInstallIntent(w, r)

	if inst := s.instanceGitHubApp(); inst.Configured() && inst.Slug != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"github_url": githubapp.InstallURL(inst.Slug, state),
			"existing":   true,
			"branch":     GitHubBranch,
			"urls":       githubapp.AppURLs(s.githubBase(r)),
		})
		return
	}
	if cfg, _ := s.Store.GetGitHub(u.ID); cfg.HasApp() && cfg.AppSlug != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"github_url": githubapp.InstallURL(cfg.AppSlug, state),
			"existing":   true,
			"branch":     GitHubBranch,
		})
		return
	}
	base := s.githubBase(r)
	name := "Syncidian-" + strings.ReplaceAll(u.ID, "-", "")
	if len(name) > 34 {
		name = name[:34]
	}
	manifest := githubapp.NewManifest(base, name)
	writeJSON(w, http.StatusOK, map[string]any{
		"github_url": "https://github.com/settings/apps/new?state=" + url.QueryEscape(state),
		"manifest":   manifest,
		"branch":     GitHubBranch,
		"urls":       githubapp.AppURLs(base),
	})
}

func (s *Server) handleGitHubAppCallback(w http.ResponseWriter, r *http.Request) {
	user, _ := s.authenticate(r)
	if user == nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {"Sign in first."}})
		return
	}
	if msg := githubErr(r); msg != "" {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {msg}})
		return
	}
	if !s.validGitHubState(r, r.URL.Query().Get("state")) {
		s.dashboardRedirect(w, r, url.Values{
			"github":  {"error"},
			"message": {"GitHub App setup expired. Try again."},
		})
		return
	}
	creds, err := githubapp.ConvertManifest(r.URL.Query().Get("code"))
	if err != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
		return
	}
	if user.IsAdmin {
		if err := s.Store.SetInstanceGitHubApp(store.GitHubApp{
			AppID: creds.ID, Slug: creds.Slug, PEM: creds.PEM, ClientID: creds.ClientID, ClientSecret: creds.ClientSecret,
		}); err != nil {
			s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
			return
		}
		http.Redirect(w, r, s.adminURL(r)+"?github=app-ready", http.StatusFound)
		return
	}
	cfg := &store.GitHubConfig{
		UserID: user.ID, Branch: GitHubBranch, AppID: creds.ID, AppSlug: creds.Slug,
		AppPEM: creds.PEM, ClientID: creds.ClientID, ClientSecret: creds.ClientSecret,
	}
	if err := s.Store.SetGitHub(*cfg); err != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
		return
	}
	installState, err := randomHex(16)
	if err != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {"Could not continue to install."}})
		return
	}
	s.setGitHubStateCookie(w, r, installState)
	s.setPendingInstallIntent(w, r)
	http.Redirect(w, r, githubapp.InstallURL(creds.Slug, installState), http.StatusFound)
}

func (s *Server) handleGitHubAppSetup(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("setup_action") == "request" {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {"The GitHub App installation was not approved."}})
		return
	}
	installationID, err := strconv.ParseInt(r.URL.Query().Get("installation_id"), 10, 64)
	if err != nil || installationID == 0 {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {"GitHub did not return an installation."}})
		return
	}
	// GitHub documents that setup URLs can be hit with a spoofed installation_id.
	// Require the nonce we put on the install URL (and in the state cookie).
	if !s.validGitHubState(r, r.URL.Query().Get("state")) {
		s.dashboardRedirect(w, r, url.Values{
			"github":  {"error"},
			"message": {"GitHub App setup expired. Try again."},
		})
		return
	}
	user, _ := s.authenticate(r)
	if user == nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "syncidian_pending_install",
			Value:    strconv.FormatInt(installationID, 10),
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
			MaxAge:   600,
		})
		http.Redirect(w, r, s.requestBase(r)+"/api/v1/auth/github/start?next=setup", http.StatusFound)
		return
	}
	if user.IsAdmin {
		http.Redirect(w, r, s.adminURL(r), http.StatusFound)
		return
	}
	s.finishInstallation(w, r, user, installationID)
}

func (s *Server) finishInstallation(w http.ResponseWriter, r *http.Request, u *store.User, installationID int64) {
	s.clearPendingInstallCookies(w, r)
	cfg, _ := s.Store.GetGitHub(u.ID)
	if cfg == nil {
		cfg = &store.GitHubConfig{UserID: u.ID, Branch: GitHubBranch}
	}
	if inst := s.instanceGitHubApp(); inst.Configured() {
		cfg.AppID = inst.AppID
		cfg.AppSlug = inst.Slug
		cfg.AppPEM = inst.PEM
		cfg.ClientID = inst.ClientID
		cfg.ClientSecret = inst.ClientSecret
	}
	if !cfg.HasApp() {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {"GitHub App credentials are missing."}})
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
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {"Install the GitHub App on at least one repository, and allow write access."}})
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

func (s *Server) handleGitHubAppURLs(w http.ResponseWriter, r *http.Request) {
	app := s.instanceGitHubApp()
	writeJSON(w, http.StatusOK, map[string]any{
		"urls":       githubapp.AppURLs(s.githubBase(r)),
		"configured": app.Configured(),
		"slug":       app.Slug,
		"branch":     GitHubBranch,
		"note":       "GitHub requires a webhook URL so it can ping the app. Syncidian accepts that ping even if you do not subscribe to extra events.",
	})
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

// githubBase is the origin GitHub can reach (callback, setup, webhook, OAuth).
// Prefer SYNCIDIAN_PUBLIC_URL so sign-in always uses the URL registered on the
// GitHub App (custom domain), not a Tailscale or Railway hostname.
func (s *Server) githubBase(r *http.Request) string {
	if u := strings.TrimRight(s.Cfg.PublicURL, "/"); u != "" {
		return u
	}
	return s.requestBase(r)
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

const pendingInstallIntentCookie = "syncidian_install_intent"

func (s *Server) setPendingInstallIntent(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     pendingInstallIntentCookie,
		Value:    "1",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   600,
	})
}

func (s *Server) hasPendingInstallIntent(r *http.Request) bool {
	c, err := r.Cookie(pendingInstallIntentCookie)
	return err == nil && c.Value != ""
}

func (s *Server) clearPendingInstallCookies(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	expired := func(name string) *http.Cookie {
		return &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   secure,
			MaxAge:   -1,
		}
	}
	http.SetCookie(w, expired("syncidian_pending_install"))
	http.SetCookie(w, expired(pendingInstallIntentCookie))
}

func (s *Server) validGitHubState(r *http.Request, state string) bool {
	c, err := r.Cookie(githubStateCookie)
	if err != nil || c.Value == "" || state == "" {
		return false
	}
	if len(c.Value) != len(state) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) == 1
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
