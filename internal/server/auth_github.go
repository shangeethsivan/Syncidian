package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shangeethsivan/Syncidian/internal/githubapp"
	"github.com/shangeethsivan/Syncidian/internal/store"
)

func (s *Server) instanceGitHubApp() *store.GitHubApp {
	if s.Cfg.GitHubAppID != 0 && s.Cfg.GitHubAppPEM != "" && s.Cfg.GitHubClientID != "" {
		return &store.GitHubApp{
			AppID:        s.Cfg.GitHubAppID,
			Slug:         githubapp.NormalizeAppSlug(s.Cfg.GitHubAppSlug),
			PEM:          s.Cfg.GitHubAppPEM,
			ClientID:     s.Cfg.GitHubClientID,
			ClientSecret: s.Cfg.GitHubClientSecret,
		}
	}
	a, _ := s.Store.GetInstanceGitHubApp()
	if a == nil {
		return &store.GitHubApp{}
	}
	a.Slug = githubapp.NormalizeAppSlug(a.Slug)
	return a
}

func (s *Server) handleGitHubAuthStart(w http.ResponseWriter, r *http.Request) {
	n, _ := s.Store.UserCount()
	if n == 0 {
		s.dashboardRedirect(w, r, url.Values{
			"github":  {"error"},
			"message": {"This instance is not ready yet. An operator must create the first admin."},
		})
		return
	}
	app := s.instanceGitHubApp()
	if !app.Configured() {
		s.dashboardRedirect(w, r, url.Values{
			"github":  {"error"},
			"message": {"This instance has no GitHub App yet. An operator must register it first."},
		})
		return
	}
	state, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start GitHub login")
		return
	}
	s.setGitHubStateCookie(w, r, state)
	next := strings.TrimSpace(r.URL.Query().Get("next"))
	if next == "" {
		next = "app"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "syncidian_github_next",
		Value:    next,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		MaxAge:   600,
	})
	urls := githubapp.AppURLs(s.requestBase(r))
	q := url.Values{
		"client_id":    {app.ClientID},
		"redirect_uri": {urls.Callback},
		"state":        {state},
		"allow_signup": {"true"},
	}
	http.Redirect(w, r, "https://github.com/login/oauth/authorize?"+q.Encode(), http.StatusFound)
}

func (s *Server) handleGitHubAuthCallback(w http.ResponseWriter, r *http.Request) {
	if msg := githubErr(r); msg != "" {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {msg}})
		return
	}

	installationID, _ := strconv.ParseInt(r.URL.Query().Get("installation_id"), 10, 64)
	stateOK := s.validGitHubState(r, r.URL.Query().Get("state"))
	hasIntent := s.hasPendingInstallIntent(r)

	// Install & Authorize (OAuth during installation) returns here with
	// installation_id — not the setup URL. Prefer the existing session so we
	// do not switch accounts. Accept matching state, an install-intent cookie
	// from Connect with GitHub, or a verified App installation (already installed).
	if installationID != 0 {
		if existing, _ := s.authenticate(r); existing != nil && !existing.IsAdmin {
			if stateOK || hasIntent || s.installationOwnedByApp(installationID) {
				s.finishInstallation(w, r, existing, installationID)
				return
			}
		}
	}

	if !stateOK {
		s.dashboardRedirect(w, r, url.Values{
			"github":  {"error"},
			"message": {"GitHub sign-in expired. Open GitHub in the dashboard and click Connect with GitHub again."},
		})
		return
	}

	app := s.instanceGitHubApp()
	if !app.Configured() {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {"GitHub App is not registered."}})
		return
	}
	urls := githubapp.AppURLs(s.requestBase(r))
	token, err := githubapp.ExchangeOAuth(app.ClientID, app.ClientSecret, r.URL.Query().Get("code"), urls.Callback)
	if err != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
		return
	}
	ghUser, err := githubapp.GetUser(token)
	if err != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
		return
	}
	u, err := s.upsertGitHubUser(ghUser)
	if err != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
		return
	}
	if u.IsAdmin {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {"Admins sign in on the operator page."}})
		return
	}
	sess, err := s.Store.CreateSession(u.ID, 30*24*time.Hour)
	if err != nil {
		s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {err.Error()}})
		return
	}
	setSessionCookie(w, sess.ID, r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))

	if installationID == 0 {
		if c, err := r.Cookie("syncidian_pending_install"); err == nil && c.Value != "" {
			if id, e := strconv.ParseInt(c.Value, 10, 64); e == nil && id != 0 {
				installationID = id
			}
		}
	}
	// Already installed earlier but never bound (common before the callback fix).
	if installationID == 0 && app.AppID != 0 {
		if id, err := githubapp.FindAppInstallation(token, app.AppID); err == nil && id != 0 {
			installationID = id
		}
	}
	if installationID != 0 {
		s.finishInstallation(w, r, u, installationID)
		return
	}

	next := "app"
	if c, err := r.Cookie("syncidian_github_next"); err == nil && c.Value != "" {
		next = c.Value
	}
	if next == "install" || next == "setup" || next == "reconcile" {
		if app.Slug != "" {
			state, err := randomHex(16)
			if err != nil {
				s.dashboardRedirect(w, r, url.Values{"github": {"error"}, "message": {"Could not continue to install."}})
				return
			}
			s.setGitHubStateCookie(w, r, state)
			s.setPendingInstallIntent(w, r)
			http.Redirect(w, r, githubapp.InstallURL(app.Slug, state), http.StatusFound)
			return
		}
	}
	s.dashboardRedirect(w, r, url.Values{"github": {"signed_in"}})
}

func (s *Server) installationOwnedByApp(installationID int64) bool {
	app := s.instanceGitHubApp()
	if !app.Configured() || installationID == 0 {
		return false
	}
	inst, err := githubapp.GetInstallation(app.AppID, []byte(app.PEM), installationID)
	return err == nil && inst != nil && inst.ID == installationID
}

func (s *Server) upsertGitHubUser(gh *githubapp.User) (*store.User, error) {
	if existing, _ := s.Store.GetUserByGitHubID(gh.ID); existing != nil {
		_ = s.Store.SetUserGitHub(existing.ID, gh.ID, gh.Email)
		existing.Email = gh.Email
		return existing, nil
	}
	if gh.Email != "" {
		if existing, _ := s.Store.GetUserByEmail(gh.Email); existing != nil && !existing.IsAdmin {
			_ = s.Store.SetUserGitHub(existing.ID, gh.ID, gh.Email)
			existing.GitHubID = gh.ID
			return existing, nil
		}
	}
	n, err := s.Store.UserCount()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errAdminRequired()
	}
	return s.Store.CreateGitHubUser(gh.Login, gh.Email, gh.ID)
}

func errAdminRequired() error {
	return errString("Create the first admin before signing in with GitHub.")
}

type errString string

func (e errString) Error() string { return string(e) }
