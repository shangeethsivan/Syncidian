package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shangeethsivan/Syncidian/internal/config"
	"github.com/shangeethsivan/Syncidian/internal/store"
)

func newAdminHostServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	srv := New(config.Config{
		Addr:          ":0",
		DataDir:       dir,
		PublicURL:     "https://syncidian.com",
		AdminPath:     "/admin",
		AdminHost:     "admin.syncidian.com",
		AdminListenIP: "100.64.1.20",
	}, st, nil)
	return srv, st
}

func serveHost(srv *Server, method, path, host string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Host = host
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func setupAdminOnHost(t *testing.T, srv *Server, host string) []*http.Cookie {
	t.Helper()
	rec := serveHost(srv, http.MethodPost, "/api/v1/setup", host, map[string]string{
		"username": "ada", "password": "password1",
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup on %s: %d %s", host, rec.Code, rec.Body.Bytes())
	}
	return rec.Result().Cookies()
}

func TestAdminHostHidesOperatorOnPublic(t *testing.T) {
	srv, _ := newAdminHostServer(t)

	rec := serveHost(srv, http.MethodGet, "/admin", "syncidian.com", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public /admin should 404, got %d", rec.Code)
	}

	rec = serveHost(srv, http.MethodPost, "/api/v1/setup", "syncidian.com", map[string]string{
		"username": "ada", "password": "password1",
	}, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public setup should 404, got %d %s", rec.Code, rec.Body.Bytes())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader([]byte(`{"username":"ada","password":"password1"}`)))
	req.Host = "syncidian.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Host", "admin.syncidian.com")
	spoof := httptest.NewRecorder()
	srv.Handler().ServeHTTP(spoof, req)
	if spoof.Code != http.StatusNotFound {
		t.Fatalf("X-Forwarded-Host must not unlock admin, got %d %s", spoof.Code, spoof.Body.Bytes())
	}

	rec = serveHost(srv, http.MethodGet, "/", "syncidian.com", nil, nil)
	if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte(`id="landing"`)) {
		t.Fatalf("public landing: %d", rec.Code)
	}

	rec = serveHost(srv, http.MethodGet, "/robots.txt", "syncidian.com", nil, nil)
	if strings.Contains(rec.Body.String(), "/admin") {
		t.Fatalf("robots must not advertise the operator path when AdminHost is set:\n%s", rec.Body.String())
	}
}

func TestAdminHostServesOperatorAndSetup(t *testing.T) {
	srv, _ := newAdminHostServer(t)

	rec := serveHost(srv, http.MethodGet, "/", "admin.syncidian.com", nil, nil)
	if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte(`isAdminPath`)) {
		t.Fatalf("admin host / should serve SPA: %d", rec.Code)
	}

	rec = serveHost(srv, http.MethodGet, "/api/v1/setup", "admin.syncidian.com", nil, nil)
	if rec.Code != 200 {
		t.Fatalf("setup status %d %s", rec.Code, rec.Body.Bytes())
	}
	if !strings.Contains(rec.Body.String(), `"admin_host":"admin.syncidian.com"`) {
		t.Fatalf("setup on admin host should include admin_host: %s", rec.Body.Bytes())
	}
	if !strings.Contains(rec.Body.String(), `"callback":"https://syncidian.com/api/v1/auth/github/callback"`) {
		t.Fatalf("GitHub URLs must stay on the public origin: %s", rec.Body.Bytes())
	}

	rec = serveHost(srv, http.MethodGet, "/api/v1/setup", "syncidian.com", nil, nil)
	if rec.Code != 200 {
		t.Fatalf("public setup status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"admin_host"`) {
		t.Fatalf("public setup must not advertise admin_host: %s", rec.Body.Bytes())
	}

	cookies := setupAdminOnHost(t, srv, "admin.syncidian.com")

	rec = serveHost(srv, http.MethodPost, "/api/v1/auth/login", "syncidian.com", map[string]string{
		"username": "ada", "password": "password1",
	}, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("admin login on public host should 404, got %d %s", rec.Code, rec.Body.Bytes())
	}

	rec = serveHost(srv, http.MethodPost, "/api/v1/auth/login", "admin.syncidian.com", map[string]string{
		"username": "ada", "password": "password1",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin login on admin host: %d %s", rec.Code, rec.Body.Bytes())
	}

	rec = serveHost(srv, http.MethodGet, "/api/v1/users", "syncidian.com", nil, cookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("admin user list on public host should 404, got %d %s", rec.Code, rec.Body.Bytes())
	}

	rec = serveHost(srv, http.MethodGet, "/api/v1/users", "admin.syncidian.com", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin user list on admin host: %d %s", rec.Code, rec.Body.Bytes())
	}
}

func TestAdminHostWaitlistList(t *testing.T) {
	srv, st := newAdminHostServer(t)
	if _, err := st.AddWaitlistEmail("ada@example.com"); err != nil {
		t.Fatal(err)
	}
	cookies := setupAdminOnHost(t, srv, "admin.syncidian.com")

	rec := serveHost(srv, http.MethodGet, "/api/v1/waitlist", "syncidian.com", nil, cookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("waitlist list on public host should 404, got %d %s", rec.Code, rec.Body.Bytes())
	}

	rec = serveHost(srv, http.MethodGet, "/api/v1/waitlist", "admin.syncidian.com", nil, cookies)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ada@example.com") {
		t.Fatalf("waitlist list on admin host: %d %s", rec.Code, rec.Body.Bytes())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", bytes.NewReader([]byte(`{"email":"bob@example.com"}`)))
	req.Host = "syncidian.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://syncidian.com")
	join := httptest.NewRecorder()
	srv.Handler().ServeHTTP(join, req)
	if join.Code != http.StatusOK {
		t.Fatalf("public waitlist join should still work: %d %s", join.Code, join.Body.Bytes())
	}
}
