package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shangeethsivan/Syncidian/internal/config"
	"github.com/shangeethsivan/Syncidian/internal/store"
)

func TestHostedPublicHost(t *testing.T) {
	for _, host := range []string{"syncidian.com", "www.syncidian.com", "SYNCIDIAN.COM", "syncidian.com:443"} {
		if !hostedPublicHost(host) {
			t.Fatalf("%q should be hosted", host)
		}
	}
	for _, host := range []string{"", "localhost", "127.0.0.1", "syncidian.example", "notsyncidian.com", "syncidian.com.evil"} {
		if hostedPublicHost(host) {
			t.Fatalf("%q should not be hosted", host)
		}
	}
}

func TestWaitlistHiddenOffHostedDomain(t *testing.T) {
	hs, cleanup := newTestServer(t)
	defer cleanup()

	res, m := doJSON(t, http.MethodGet, hs.URL+"/api/v1/setup", nil, nil, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("setup %d %v", res.StatusCode, m)
	}
	if m["waitlist"] != false {
		t.Fatalf("waitlist on localhost should be false: %v", m["waitlist"])
	}
	if m["email_login"] != true {
		t.Fatalf("email_login on localhost should be true: %v", m["email_login"])
	}
	if m["ga_id"] != "" && m["ga_id"] != nil {
		t.Fatalf("ga_id should be empty by default: %v", m["ga_id"])
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/waitlist", map[string]string{"email": "ada@example.com"}, nil, "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("join off-host %d %v", res.StatusCode, m)
	}
}

func TestWaitlistJoinOnSyncidianCom(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "https://syncidian.com"}, st, nil)

	post := func(host, origin, email string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"email": email})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", bytes.NewReader(body))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}

	rec := post("example.com", "", "ada@example.com")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign host %d %s", rec.Code, rec.Body.Bytes())
	}

	rec = post("syncidian.com", "https://evil.example", "ada@example.com")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign origin %d %s", rec.Code, rec.Body.Bytes())
	}

	rec = post("syncidian.com", "https://syncidian.com", "  Ada@Example.com ")
	if rec.Code != http.StatusOK {
		t.Fatalf("join %d %s", rec.Code, rec.Body.Bytes())
	}

	rec = post("www.syncidian.com", "https://www.syncidian.com", "ada@example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate should still be ok %d %s", rec.Code, rec.Body.Bytes())
	}

	n, err := st.WaitlistCount()
	if err != nil || n != 1 {
		t.Fatalf("stored %d %v", n, err)
	}

	rec = post("syncidian.com", "https://syncidian.com", "not-an-email")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid email %d %s", rec.Code, rec.Body.Bytes())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup", nil)
	req.Host = "syncidian.com"
	setupRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(setupRec, req)
	if setupRec.Code != http.StatusOK || !strings.Contains(setupRec.Body.String(), `"waitlist":true`) {
		t.Fatalf("setup waitlist flag: %d %s", setupRec.Code, setupRec.Body.Bytes())
	}
	if !strings.Contains(setupRec.Body.String(), `"email_login":false`) {
		t.Fatalf("hosted email_login should be false: %s", setupRec.Body.Bytes())
	}
}

func TestWaitlistListRequiresAdminOnHostedDomain(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.AddWaitlistEmail("ada@example.com"); err != nil {
		t.Fatal(err)
	}
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "https://syncidian.com"}, st, nil)
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()
	cookies := setupAdmin(t, hs)

	req, err := http.NewRequest(http.MethodGet, hs.URL+"/api/v1/waitlist", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("list via localhost host %d", res.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/waitlist", nil)
	req.Host = "syncidian.com"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin list %d %s", rec.Code, rec.Body.Bytes())
	}
	if !strings.Contains(rec.Body.String(), "ada@example.com") {
		t.Fatalf("admin list missing email: %s", rec.Body.Bytes())
	}
}

func TestSetupExposesGAID(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "http://localhost", GAID: "G-LANDING1"}, st, nil)
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()
	res, m := doJSON(t, http.MethodGet, hs.URL+"/api/v1/setup", nil, nil, "")
	if res.StatusCode != http.StatusOK || m["ga_id"] != "G-LANDING1" {
		t.Fatalf("ga_id: %d %v", res.StatusCode, m)
	}
}

func TestEmailSignupRejectedOnHostedDomain(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "https://syncidian.com"}, st, nil)
	if _, err := st.CreateUser("ada", "x", true); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]string{
		"username": "cara", "password": "password1", "email": "cara@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewReader(body))
	req.Host = "syncidian.com"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("hosted signup: want 404, got %d %s", rec.Code, rec.Body.Bytes())
	}
}

func TestEmailLoginAcceptsEmail(t *testing.T) {
	hs, cleanup := newTestServer(t)
	defer cleanup()
	setupAdmin(t, hs)
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/signup", map[string]string{
		"username": "cara", "password": "password1", "email": "cara@example.com",
	}, nil, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("signup: %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/login", map[string]string{
		"username": "cara@example.com", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("email login: %d %v", res.StatusCode, m)
	}
	user, _ := m["user"].(map[string]any)
	if user["username"] != "cara" {
		t.Fatalf("email login user: %v", m)
	}
}
