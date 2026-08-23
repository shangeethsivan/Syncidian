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

func setupAdmin(t *testing.T, hs *httptest.Server) []*http.Cookie {
	t.Helper()
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/setup", map[string]string{
		"username": "ada", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("setup status %d %v", res.StatusCode, m)
	}
	return res.Cookies()
}

func createAndLoginUser(t *testing.T, hs *httptest.Server, adminCookies []*http.Cookie, username string) []*http.Cookie {
	t.Helper()
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/users", map[string]any{
		"username": username, "password": "password1", "admin": false,
	}, adminCookies, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create user %s: %d %v", username, res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/login", map[string]string{
		"username": username, "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login %s: %d %v", username, res.StatusCode, m)
	}
	return res.Cookies()
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

	adminCookies := setupAdmin(t, hs)

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/me", nil, adminCookies, "")
	user, _ := m["user"].(map[string]any)
	if user["username"] != "ada" || user["is_admin"] != true {
		t.Fatalf("me: %v", m)
	}

	cookies := createAndLoginUser(t, hs, adminCookies, "bob")

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
		`id="auth-btn" type="button"`,
		`id="help-fab"`,
		`id="help-overlay"`,
		`id="help-panel"`,
		`Setup walkthrough`,
		`HELP_STEPS`,
		`scripts/install-plugin.sh`,
		`Settings → Syncidian`,
		`Connect your GitHub repository`,
		`admins manage users`,
		`one GitHub repository`,
	} {
		if !bytes.Contains(b, []byte(needle)) {
			t.Fatalf("dashboard missing required markup %q", needle)
		}
	}
	if bytes.Contains(b, []byte(`id="auth-btn" type="button" disabled`)) {
		t.Fatal("login must not be disabled behind a server-wide GitHub popup")
	}
	if bytes.Contains(b, []byte("Configure GitHub first")) {
		t.Fatal("GitHub must not be required before admin or user login")
	}
	if bytes.Contains(b, []byte(`id="gh-setup-modal"`)) {
		t.Fatal("server-wide GitHub setup modal should be removed")
	}
}

func TestAdminDoesNotNeedGitHubAndCannotSeePrivateUserData(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	adminCookies := setupAdmin(t, hs)

	for _, path := range []string{
		"/api/v1/github",
		"/api/v1/tokens",
		"/api/v1/devices",
		"/api/v1/activity",
		"/api/v1/conflicts",
		"/api/v1/stats",
		"/api/v1/mcp",
	} {
		res, m := doJSON(t, http.MethodGet, hs.URL+path, nil, adminCookies, "")
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("admin GET %s: want 403, got %d %v", path, res.StatusCode, m)
		}
	}

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/github", map[string]any{
		"repo": "ada/vault", "token": "ghp_test",
	}, adminCookies, "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("admin POST github: want 403, got %d %v", res.StatusCode, m)
	}

	bobCookies := createAndLoginUser(t, hs, adminCookies, "bob")
	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, bobCookies, "")
	if res.StatusCode != 200 || m["configured"] != false {
		t.Fatalf("bob github before setup: %d %v", res.StatusCode, m)
	}

	req, err := http.NewRequest(http.MethodGet, hs.URL+"/api/v1/users", nil)
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
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	var users []map[string]any
	if err := json.Unmarshal(raw, &users); err != nil {
		t.Fatalf("users list: %s %v", raw, err)
	}
	if len(users) != 2 {
		t.Fatalf("want 2 users, got %s", raw)
	}
	for _, u := range users {
		if _, ok := u["id"]; ok {
			t.Fatalf("admin user list must not include ids: %v", u)
		}
		if _, ok := u["password_hash"]; ok {
			t.Fatalf("admin user list must not include password hashes: %v", u)
		}
		if _, ok := u["repo"]; ok {
			t.Fatalf("admin user list must not include github repo: %v", u)
		}
		if _, ok := u["token"]; ok {
			t.Fatalf("admin user list must not include tokens: %v", u)
		}
		if u["username"] == nil || u["is_admin"] == nil {
			t.Fatalf("admin user list missing public fields: %v", u)
		}
	}
}

func TestGitHubIsPerUserNotPerServer(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	adminCookies := setupAdmin(t, hs)
	bob := createAndLoginUser(t, hs, adminCookies, "bob")
	cara := createAndLoginUser(t, hs, adminCookies, "cara")

	res, m := doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, bob, "")
	if res.StatusCode != 200 || m["configured"] != false {
		t.Fatalf("bob should start unconfigured: %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github", map[string]any{
		"repo": "not-a-repo", "token": "ghp_test",
	}, bob, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid repo to fail, got %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github", map[string]any{
		"repo": "bob/vault", "token": "",
	}, bob, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected missing token to fail, got %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, cara, "")
	if res.StatusCode != 200 || m["configured"] != false {
		t.Fatalf("cara must not inherit a server-wide repo: %d %v", res.StatusCode, m)
	}
}

func TestEmptyListEndpointsReturnArrays(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	adminCookies := setupAdmin(t, hs)
	cookies := createAndLoginUser(t, hs, adminCookies, "bob")

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
	u, err := st.CreateUser("a", "x", false)
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
