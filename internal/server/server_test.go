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
		`id="landing"`,
		`Start a journey with Syncidian`,
		`Connect to your GitHub repository`,
		`Sign up using GitHub`,
		`id="auth-btn" type="button">Sign in</button`,
		`href="/admin"`,
		`/api/v1/auth/github/callback`,
		`/api/v1/github/app/setup`,
		`/api/v1/github/app/webhook`,
		`id="help-fab"`,
		`id="help-overlay"`,
		`id="help-panel"`,
		`Setup walkthrough`,
		`HELP_STEPS`,
		`scripts/install-plugin.sh`,
		`Settings → Syncidian`,
		`Connect your GitHub repository`,
		`one GitHub repository`,
		`Connect with GitHub`,
		`id="gh-connect-app"`,
		`only support one single branch`,
		`Connect to GitHub`,
	} {
		if !bytes.Contains(b, []byte(needle)) {
			t.Fatalf("dashboard missing required markup %q", needle)
		}
	}
	for _, forbidden := range []string{
		`id="gh-setup-modal"`,
		`Configure GitHub first`,
		`id="auth-btn" type="button" disabled`,
		`syncidian_pending_github`,
		`required before you can create an admin or sign in`,
		`id="gh-branch"`,
		`id="gh-token"`,
		`Personal access token (repo scope)`,
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

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/start", map[string]any{}, adminCookies, "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("admin POST github app start: want 403, got %d %v", res.StatusCode, m)
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
	if m["branch"] != "main" {
		t.Fatalf("unconfigured github should lock branch to main, got %v", m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github", map[string]any{
		"repo": "not-a-repo", "token": "ghp_test",
	}, bob, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected PAT to be rejected, got %d %v", res.StatusCode, m)
	}
	if errMsg, _ := m["error"].(string); !strings.Contains(errMsg, "Personal access tokens are not supported") {
		t.Fatalf("expected PAT rejection message, got %v", m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github", map[string]any{
		"repo": "bob/vault",
	}, bob, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected missing GitHub App to fail, got %d %v", res.StatusCode, m)
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

func TestGitHubAppStartManifest(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/start", map[string]any{}, nil, "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated app start: want 401, got %d %v", res.StatusCode, m)
	}

	adminCookies := setupAdmin(t, hs)
	bob := createAndLoginUser(t, hs, adminCookies, "bob")
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/start", map[string]any{}, bob, "")
	if res.StatusCode != 200 {
		t.Fatalf("app start: %d %v", res.StatusCode, m)
	}
	if m["branch"] != "main" {
		t.Fatalf("app start should lock to main, got %v", m)
	}
	githubURL, _ := m["github_url"].(string)
	if !strings.Contains(githubURL, "https://github.com/settings/apps/new?state=") {
		t.Fatalf("github_url: %v", m)
	}
	manifest, _ := m["manifest"].(map[string]any)
	if manifest == nil {
		t.Fatalf("missing manifest: %v", m)
	}
	perms, _ := manifest["default_permissions"].(map[string]any)
	if perms["contents"] != "write" || perms["metadata"] != "read" {
		t.Fatalf("permissions: %v", perms)
	}
	if redirect, _ := manifest["redirect_url"].(string); !strings.Contains(redirect, "/api/v1/github/app/callback") {
		t.Fatalf("redirect_url: %v", manifest)
	}
	if setup, _ := manifest["setup_url"].(string); !strings.Contains(setup, "/api/v1/github/app/setup") {
		t.Fatalf("setup_url: %v", manifest)
	}
	gotCookie := false
	for _, c := range res.Cookies() {
		if c.Name == "syncidian_github_state" && c.Value != "" {
			gotCookie = true
		}
	}
	if !gotCookie {
		t.Fatal("expected GitHub state cookie")
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/webhook", map[string]any{"zen": "ok"}, nil, "")
	if res.StatusCode != 200 {
		t.Fatalf("webhook should be public: %d %v", res.StatusCode, m)
	}

	getRes, err := http.Get(hs.URL + "/api/v1/github/app/webhook")
	if err != nil {
		t.Fatal(err)
	}
	getRes.Body.Close()
	if getRes.StatusCode != 200 {
		t.Fatalf("GET webhook should be public: %d", getRes.StatusCode)
	}
}

func noFollowClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestPublicLandingAdminAndGitHubAppURLs(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	res, err := http.Get(hs.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.Contains(body, []byte(`id="landing"`)) {
		t.Fatalf("public landing: %d", res.StatusCode)
	}

	res, err = http.Get(hs.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	adminBody, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.Contains(adminBody, []byte(`isAdminPath`)) {
		t.Fatalf("admin page should serve the same SPA: %d", res.StatusCode)
	}

	res, m := doJSON(t, http.MethodGet, hs.URL+"/api/v1/github/app/urls", nil, nil, "")
	if res.StatusCode != 200 {
		t.Fatalf("urls: %d %v", res.StatusCode, m)
	}
	if m["configured"] != false {
		t.Fatalf("expected unconfigured app: %v", m)
	}
	urls, _ := m["urls"].(map[string]any)
	if urls == nil {
		t.Fatalf("missing urls: %v", m)
	}
	for key, suffix := range map[string]string{
		"callback":          "/api/v1/auth/github/callback",
		"setup":             "/api/v1/github/app/setup",
		"webhook":           "/api/v1/github/app/webhook",
		"manifest_callback": "/api/v1/github/app/callback",
	} {
		got, _ := urls[key].(string)
		if !strings.HasSuffix(got, suffix) {
			t.Fatalf("%s: %q want suffix %s", key, got, suffix)
		}
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/setup", nil, nil, "")
	if res.StatusCode != 200 || m["needs_setup"] != true || m["github_login"] != false {
		t.Fatalf("setup status: %v", m)
	}
	ghApp, _ := m["github_app"].(map[string]any)
	if ghApp == nil {
		t.Fatalf("setup missing github_app: %v", m)
	}
	setupURLs, _ := ghApp["urls"].(map[string]any)
	if setupURLs["callback"] != urls["callback"] {
		t.Fatalf("setup urls should match public urls: %v vs %v", setupURLs, urls)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/signup", map[string]any{
		"username": "cara", "password": "password1", "email": "cara@example.com",
	}, nil, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("signup before admin: want 400, got %d %v", res.StatusCode, m)
	}

	start, err := http.NewRequest(http.MethodGet, hs.URL+"/api/v1/auth/github/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	redir, err := noFollowClient().Do(start)
	if err != nil {
		t.Fatal(err)
	}
	redir.Body.Close()
	if redir.StatusCode != http.StatusFound {
		t.Fatalf("github start before setup: %d", redir.StatusCode)
	}
	loc := redir.Header.Get("Location")
	if !strings.HasSuffix(loc, "/admin") {
		t.Fatalf("github start before setup should send people to /admin, got %q", loc)
	}

	adminCookies := setupAdmin(t, hs)

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/signup", map[string]any{
		"username": "cara", "password": "password1", "email": "cara@example.com",
	}, nil, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("email signup: %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/signup", map[string]any{
		"username": "other", "password": "password1", "email": "cara@example.com",
	}, nil, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate email: want 400, got %d %v", res.StatusCode, m)
	}

	start, err = http.NewRequest(http.MethodGet, hs.URL+"/api/v1/auth/github/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	redir, err = noFollowClient().Do(start)
	if err != nil {
		t.Fatal(err)
	}
	redir.Body.Close()
	if redir.StatusCode != http.StatusFound {
		t.Fatalf("github start without app: %d", redir.StatusCode)
	}
	loc = redir.Header.Get("Location")
	if !strings.Contains(loc, "github=error") {
		t.Fatalf("github start without instance app should error, got %q", loc)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/register", map[string]any{
		"app_id": 99, "slug": "syncidian-test", "pem": "-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----",
		"client_id": "Iv1.abc", "client_secret": "secret",
	}, adminCookies, "")
	if res.StatusCode != 200 {
		t.Fatalf("register app: %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github/app/urls", nil, nil, "")
	if m["configured"] != true || m["slug"] != "syncidian-test" {
		t.Fatalf("urls after register: %v", m)
	}

	bob := createAndLoginUser(t, hs, adminCookies, "bob")
	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, bob, "")
	if res.StatusCode != 200 || m["instance_app"] != true {
		t.Fatalf("bob github status should see instance app: %d %v", res.StatusCode, m)
	}
	installURL, _ := m["install_url"].(string)
	if !strings.Contains(installURL, "github.com/apps/syncidian-test/installations/new") {
		t.Fatalf("install_url: %v", m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/start", map[string]any{}, bob, "")
	if res.StatusCode != 200 {
		t.Fatalf("app start with instance app: %d %v", res.StatusCode, m)
	}
	if m["existing"] != true {
		t.Fatalf("expected existing instance app install URL: %v", m)
	}
	githubURL, _ := m["github_url"].(string)
	if !strings.Contains(githubURL, "github.com/apps/syncidian-test/installations/new") {
		t.Fatalf("github_url after instance app: %v", m)
	}

	start, err = http.NewRequest(http.MethodGet, hs.URL+"/api/v1/auth/github/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	redir, err = noFollowClient().Do(start)
	if err != nil {
		t.Fatal(err)
	}
	redir.Body.Close()
	if redir.StatusCode != http.StatusFound {
		t.Fatalf("github start with app: %d", redir.StatusCode)
	}
	loc = redir.Header.Get("Location")
	if !strings.Contains(loc, "https://github.com/login/oauth/authorize") || !strings.Contains(loc, "client_id=Iv1.abc") {
		t.Fatalf("expected GitHub OAuth redirect, got %q", loc)
	}
}
