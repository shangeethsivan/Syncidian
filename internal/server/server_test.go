package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shangeethsivan/Syncidian/internal/config"
	"github.com/shangeethsivan/Syncidian/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "http://localhost"}, st, nil)
	hs := httptest.NewServer(srv.Handler())
	return hs, func() {
		hs.Close()
		_ = st.Close()
	}
}

func doJSON(t *testing.T, method, url string, body any, cookies []*http.Cookie, token string) (*http.Response, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	var m map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("decode %s: %s", err, raw)
		}
	}
	return res, m
}

func TestHealthAndSetupFlow(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	res, m := doJSON(t, http.MethodGet, hs.URL+"/health", nil, nil, "")
	if res.StatusCode != 200 || m["status"] != "ok" {
		t.Fatalf("health: %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/setup", nil, nil, "")
	if m["needs_setup"] != true {
		t.Fatalf("expected needs_setup: %v", m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/setup", map[string]string{
		"username": "ada", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("setup status %d %v", res.StatusCode, m)
	}
	cookies := res.Cookies()

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/me", nil, cookies, "")
	user, _ := m["user"].(map[string]any)
	if user["username"] != "ada" {
		t.Fatalf("me: %v", m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/tokens", map[string]string{"name": "plugin"}, cookies, "")
	token, _ := m["token"].(string)
	if !strings.HasPrefix(token, "sk_sync_") {
		t.Fatalf("token %v", m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/devices/register", map[string]string{
		"name": "MacBook Pro", "platform": "macOS", "plugin_version": "0.1.0",
	}, nil, token)
	deviceID, _ := m["id"].(string)
	if deviceID == "" {
		t.Fatalf("device: %v", m)
	}

	note := "# Hello\n"
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/sync/push", map[string]any{
		"device_id": deviceID,
		"files": []map[string]any{{
			"path":    "Inbox/Hello.md",
			"hash":    fileSHA256([]byte(note)),
			"mtime":   1,
			"content": base64.StdEncoding.EncodeToString([]byte(note)),
		}},
	}, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("push %d %v", res.StatusCode, m)
	}

	res, man := doJSON(t, http.MethodGet, hs.URL+"/api/v1/sync/manifest", nil, nil, token)
	files, _ := man["files"].([]any)
	if res.StatusCode != 200 || len(files) != 1 {
		t.Fatalf("manifest: %d %v", res.StatusCode, man)
	}

	other := "# Other\n"
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/sync/push", map[string]any{
		"device_id": deviceID,
		"files": []map[string]any{{
			"path":      "Inbox/Hello.md",
			"hash":      fileSHA256([]byte(other)),
			"mtime":     2,
			"base_hash": "deadbeef",
			"content":   base64.StdEncoding.EncodeToString([]byte(other)),
		}},
	}, nil, token)
	conflicts, _ := m["conflicts"].([]any)
	if len(conflicts) != 1 {
		t.Fatalf("expected conflict, got %v", m)
	}
}

func TestDashboardServed(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	res, err := http.Get(hs.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !bytes.Contains(b, []byte("Syncidian")) {
		t.Fatalf("dashboard status %d", res.StatusCode)
	}
	if !bytes.Contains(b, []byte("function asList")) {
		t.Fatal("dashboard must guard null list responses")
	}
	if bytes.Contains(b, []byte("conflicts.length")) {
		t.Fatal("unguarded conflicts.length would throw when API returns null")
	}
	for _, needle := range []string{
		`id="auth-btn" type="button">Sign in</button>`,
		`Authenticate to continue`,
		`id="help-fab"`,
		`id="help-overlay"`,
		`The landing page always asks you to authenticate`,
		`GitHub backup is <b>per user</b>`,
		`Admins only see usernames and roles`,
	} {
		if !bytes.Contains(b, []byte(needle)) {
			t.Fatalf("dashboard missing auth-first / help markup %q", needle)
		}
	}
	for _, forbidden := range []string{
		`id="gh-setup-modal"`,
		`Configure GitHub first`,
		`id="auth-btn" type="button" disabled`,
		`syncidian_pending_github`,
		`required before you can create an admin or sign in`,
	} {
		if bytes.Contains(b, []byte(forbidden)) {
			t.Fatalf("dashboard still gates login behind GitHub setup: %q", forbidden)
		}
	}
}

func TestGitHubRequiresAuth(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	res, m := doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, nil, "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /github should be 401, got %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github", map[string]any{
		"repo": "ada/vault", "token": "ghp_test",
	}, nil, "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST /github should be 401, got %d %v", res.StatusCode, m)
	}
}

func TestAdminCannotSeeOtherUserPrivateData(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "http://localhost"}, st, nil)
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/setup", map[string]string{
		"username": "admin", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("setup %d %v", res.StatusCode, m)
	}
	adminCookies := res.Cookies()

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/users", map[string]any{
		"username": "bob", "password": "password1", "admin": false,
	}, adminCookies, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create user %d %v", res.StatusCode, m)
	}
	bobID, _ := m["id"].(string)
	if bobID == "" {
		t.Fatalf("expected user id: %v", m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/login", map[string]string{
		"username": "bob", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("bob login %d %v", res.StatusCode, m)
	}
	bobCookies := res.Cookies()

	if err := st.SetGitHub(store.GitHubConfig{
		UserID: bobID,
		Token:  "ghp_bob_secret",
		Repo:   "bob/private-vault",
		Branch: "main",
	}); err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateToken(bobID, "obsidian", "sk_sync_bobsecret", "sk_sync_bobs…")
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AddActivity(store.Activity{UserID: bobID, Action: "note.edit", Path: "Secret.md", Detail: "private note"})

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, adminCookies, "")
	if res.StatusCode != 200 {
		t.Fatalf("admin github %d %v", res.StatusCode, m)
	}
	if m["configured"] != false {
		t.Fatalf("admin must not see bob's GitHub config: %v", m)
	}
	raw, _ := json.Marshal(m)
	if strings.Contains(string(raw), "bob/private-vault") || strings.Contains(string(raw), "ghp_bob_secret") {
		t.Fatalf("admin github response leaked user repo or token: %s", raw)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, bobCookies, "")
	if m["configured"] != true || m["repo"] != "bob/private-vault" {
		t.Fatalf("bob should see his own repo: %v", m)
	}
	if _, ok := m["token"]; ok {
		t.Fatalf("github GET must never return the raw token: %v", m)
	}

	req, err := http.NewRequest(http.MethodGet, hs.URL+"/api/v1/tokens", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range adminCookies {
		req.AddCookie(c)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	tokenRaw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(tokenRaw), "sk_sync_bobsecret") || strings.Contains(string(tokenRaw), "obsidian") {
		t.Fatalf("admin tokens leaked bob's token: %s", tokenRaw)
	}

	req, err = http.NewRequest(http.MethodGet, hs.URL+"/api/v1/activity", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range adminCookies {
		req.AddCookie(c)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	actRaw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(actRaw), "Secret.md") || strings.Contains(string(actRaw), "note.edit") {
		t.Fatalf("admin activity leaked bob's events: %s", actRaw)
	}

	req, err = http.NewRequest(http.MethodGet, hs.URL+"/api/v1/users", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range adminCookies {
		req.AddCookie(c)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	usersRaw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("users %d %s", res.StatusCode, usersRaw)
	}
	for _, secret := range []string{"ghp_bob_secret", "bob/private-vault", "sk_sync_bobsecret", "Secret.md", "password_hash"} {
		if strings.Contains(string(usersRaw), secret) {
			t.Fatalf("admin user list leaked private field %q: %s", secret, usersRaw)
		}
	}
	var users []map[string]any
	if err := json.Unmarshal(usersRaw, &users); err != nil {
		t.Fatalf("users decode: %s", usersRaw)
	}
	if len(users) != 2 {
		t.Fatalf("expected admin + bob, got %s", usersRaw)
	}
	for _, u := range users {
		if _, ok := u["password_hash"]; ok {
			t.Fatalf("user list must not include password_hash: %v", u)
		}
		if _, ok := u["token"]; ok {
			t.Fatalf("user list must not include tokens: %v", u)
		}
		if _, ok := u["repo"]; ok {
			t.Fatalf("user list must not include github repo: %v", u)
		}
	}
}

func TestGitHubRequiredForNewUser(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/setup", map[string]string{
		"username": "ada", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("setup %d %v", res.StatusCode, m)
	}
	cookies := res.Cookies()

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, cookies, "")
	if res.StatusCode != 200 {
		t.Fatalf("github status %d %v", res.StatusCode, m)
	}
	if m["configured"] != false {
		t.Fatalf("new user should not have GitHub configured: %v", m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github", map[string]any{
		"repo": "not-a-repo", "token": "ghp_test",
	}, cookies, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid repo to fail, got %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github", map[string]any{
		"repo": "ada/vault", "token": "",
	}, cookies, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected missing token to fail, got %d %v", res.StatusCode, m)
	}
}

func TestEmptyListEndpointsReturnArrays(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/setup", map[string]string{
		"username": "ada", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("setup status %d %v", res.StatusCode, m)
	}
	cookies := res.Cookies()

	for _, path := range []string{"/api/v1/conflicts", "/api/v1/activity", "/api/v1/devices", "/api/v1/tokens"} {
		req, err := http.NewRequest(http.MethodGet, hs.URL+path, nil)
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
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("%s status %d %s", path, res.StatusCode, raw)
		}
		var list []any
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatalf("%s: expected JSON array, got %s (%v)", path, raw, err)
		}
		if list == nil {
			t.Fatalf("%s: decoded as null, want empty array", path)
		}
	}
}

func TestVaultIsolation(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	u, err := st.CreateUser("a", "x", true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.VaultDir(u.ID), "n.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, u.ID) {
		t.Fatal(path)
	}
}
