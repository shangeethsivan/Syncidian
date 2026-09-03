package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/shangeethsivan/Syncidian/internal/config"
	"github.com/shangeethsivan/Syncidian/internal/githubapp"
	"github.com/shangeethsivan/Syncidian/internal/store"
)

func TestGitHubSignInAllowlist(t *testing.T) {
	s := &Server{Cfg: config.Config{GitHubAllowedEmails: []string{"shangeeth95@gmail.com"}}}
	if s.githubSignInAllowed(&githubapp.User{Email: "other@example.com"}) {
		t.Fatal("other email must be denied")
	}
	if !s.githubSignInAllowed(&githubapp.User{Email: "Shangeeth95@gmail.com"}) {
		t.Fatal("allowlist email must match case-insensitively")
	}
	if !s.githubSignInAllowed(&githubapp.User{Emails: []string{"shangeeth95@gmail.com"}}) {
		t.Fatal("verified secondary email must match")
	}
	if s.githubSignInAllowed(nil) {
		t.Fatal("nil GitHub user must be denied when restricted")
	}
	open := &Server{}
	if !open.githubSignInAllowed(&githubapp.User{Email: "anyone@example.com"}) {
		t.Fatal("empty allowlist allows all")
	}
}

func TestSetupHidesGitHubSignInWhenAllowlistSet(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{
		Addr: ":0", DataDir: dir, PublicURL: "http://localhost",
		GitHubAllowedEmails: []string{"shangeeth95@gmail.com"},
	}, st, nil)
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()

	res, m := doJSON(t, http.MethodGet, hs.URL+"/api/v1/setup", nil, nil, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("setup %d %v", res.StatusCode, m)
	}
	if m["github_signin_hidden"] != true {
		t.Fatalf("allowlist should hide GitHub sign-in: %v", m["github_signin_hidden"])
	}
}

func TestSetupHidesGitHubSignInOnHostedDomain(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "https://syncidian.com"}, st, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup", nil)
	req.Host = "syncidian.com"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup %d %s", rec.Code, rec.Body.Bytes())
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["github_signin_hidden"] != true {
		t.Fatalf("hosted domain should hide GitHub sign-in: %v", m)
	}
}

func TestGitHubOAuthRedirectUsesPublicURL(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{
		Addr: ":0", DataDir: dir, PublicURL: "https://syncidian.com",
	}, st, nil)
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()

	adminCookies := setupAdmin(t, hs)
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/register", map[string]any{
		"app_id": 42, "slug": "syncidian", "pem": "-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----",
		"client_id": "Iv1.cb", "client_secret": "secret",
	}, adminCookies, "")
	if res.StatusCode != 200 {
		t.Fatalf("register: %d %v", res.StatusCode, m)
	}

	req, err := http.NewRequest(http.MethodGet, hs.URL+"/api/v1/auth/github/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "something.up.railway.app"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "something.up.railway.app")
	redir, err := noFollowClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	redir.Body.Close()
	if redir.StatusCode != http.StatusFound {
		t.Fatalf("start: %d", redir.StatusCode)
	}
	loc := redir.Header.Get("Location")
	if !strings.Contains(loc, "redirect_uri="+url.QueryEscape("https://syncidian.com/api/v1/auth/github/callback")) {
		t.Fatalf("OAuth must use PublicURL callback, not the Railway host: %q", loc)
	}
	if strings.Contains(loc, "railway.app") {
		t.Fatalf("OAuth must not send a Railway redirect_uri: %q", loc)
	}
}

func TestSetupShowsGitHubSignInOnLocalhost(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	res, m := doJSON(t, http.MethodGet, hs.URL+"/api/v1/setup", nil, nil, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("setup %d %v", res.StatusCode, m)
	}
	if m["github_signin_hidden"] != false {
		t.Fatalf("localhost without allowlist should show GitHub sign-in: %v", m["github_signin_hidden"])
	}
}
