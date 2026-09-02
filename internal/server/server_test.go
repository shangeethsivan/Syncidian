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

func vaultPluginAuth(t *testing.T, hs *httptest.Server, username string) (token, deviceID string) {
	t.Helper()
	adminCookies := setupAdmin(t, hs)
	cookies := createAndLoginUser(t, hs, adminCookies, username)
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/tokens", map[string]string{"name": "plugin"}, cookies, "")
	token, _ = m["token"].(string)
	if !strings.HasPrefix(token, "sk_sync_") {
		t.Fatalf("token %v", m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/devices/register", map[string]string{
		"name": "MacBook", "platform": "macOS", "plugin_version": "0.1.0",
	}, nil, token)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("register %d %v", res.StatusCode, m)
	}
	deviceID, _ = m["id"].(string)
	if deviceID == "" {
		t.Fatalf("device: %v", m)
	}
	return token, deviceID
}

func pushNote(t *testing.T, hs *httptest.Server, token, deviceID, path, body, base string) map[string]any {
	t.Helper()
	file := map[string]any{
		"path":    path,
		"hash":    fileSHA256([]byte(body)),
		"mtime":   1,
		"content": base64.StdEncoding.EncodeToString([]byte(body)),
	}
	if base != "" {
		file["base_hash"] = base
	}
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/sync/push", map[string]any{
		"device_id": deviceID,
		"files":     []map[string]any{file},
	}, nil, token)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("push %s: %d %v", path, res.StatusCode, m)
	}
	return m
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
	if _, ok := m["persistence"]; !ok {
		t.Fatalf("health should include persistence: %v", m)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/setup", nil, nil, "")
	if m["needs_setup"] != true {
		t.Fatalf("expected needs_setup: %v", m)
	}
	pers, _ := m["persistence"].(map[string]any)
	if pers == nil {
		t.Fatalf("setup should report persistence: %v", m)
	}
	if pers["data_dir"] == "" {
		t.Fatalf("persistence.data_dir missing: %v", pers)
	}
	if _, ok := pers["ok"]; !ok {
		t.Fatalf("persistence.ok missing: %v", pers)
	}
	if pers["kind"] == "" {
		t.Fatalf("persistence.kind missing: %v", pers)
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

func TestSetupReportsEphemeralOnRailwayWithoutVolume(t *testing.T) {
	t.Setenv("RAILWAY_VOLUME_MOUNT_PATH", "")
	t.Setenv("RAILWAY_ENVIRONMENT", "production")
	hs, done := newTestServer(t)
	defer done()

	_, m := doJSON(t, http.MethodGet, hs.URL+"/api/v1/setup", nil, nil, "")
	pers, _ := m["persistence"].(map[string]any)
	if pers["ok"] != false || pers["kind"] != "ephemeral" {
		t.Fatalf("expected ephemeral persistence on Railway without a volume, got %v", pers)
	}
	msg, _ := pers["message"].(string)
	if !strings.Contains(msg, "Railway") {
		t.Fatalf("expected Railway warning, got %q", msg)
	}
}

func TestSyncFolderDeleteRemovesChildren(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	token, deviceID := vaultPluginAuth(t, hs, "bob")

	a := "# A\n"
	b := "# B\n"
	hashA := fileSHA256([]byte(a))
	pushNote(t, hs, token, deviceID, "Projects/a.md", a, "")
	pushNote(t, hs, token, deviceID, "Projects/b.md", b, "")

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/sync/push", map[string]any{
		"device_id": deviceID,
		"files": []map[string]any{{
			"path":      "Projects",
			"hash":      "",
			"deleted":   true,
			"base_hash": hashA,
			"mtime":     3,
			"content":   "",
		}},
	}, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("delete folder %d %v", res.StatusCode, m)
	}
	accepted, _ := m["accepted"].([]any)
	if len(accepted) < 2 {
		t.Fatalf("expected child paths accepted, got %v", m)
	}

	res, man := doJSON(t, http.MethodGet, hs.URL+"/api/v1/sync/manifest", nil, nil, token)
	files, _ := man["files"].([]any)
	if res.StatusCode != 200 || len(files) != 0 {
		t.Fatalf("manifest after folder delete: %d %v", res.StatusCode, man)
	}

	res, plan := doJSON(t, http.MethodPost, hs.URL+"/api/v1/sync/plan", map[string]any{
		"device_id": deviceID,
		"files":     []map[string]any{},
	}, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("plan %d %v", res.StatusCode, plan)
	}
	if pulls, _ := plan["Pull"].([]any); len(pulls) != 0 {
		t.Fatalf("deleted folder came back as pull: %v", plan)
	}
}

func TestSyncPlanTombstonesDeleteInsteadOfPull(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	token, deviceID := vaultPluginAuth(t, hs, "bob")

	body := "# keep on server\n"
	hash := fileSHA256([]byte(body))
	pushNote(t, hs, token, deviceID, "Gone/note.md", body, "")

	res, plan := doJSON(t, http.MethodPost, hs.URL+"/api/v1/sync/plan", map[string]any{
		"device_id": deviceID,
		"files": []map[string]any{{
			"path":      "Gone/note.md",
			"hash":      "",
			"deleted":   true,
			"base_hash": hash,
		}},
	}, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("plan %d %v", res.StatusCode, plan)
	}
	if pulls, _ := plan["Pull"].([]any); len(pulls) != 0 {
		t.Fatalf("local delete should not pull: %v", plan)
	}
	deletes, _ := plan["Delete"].([]any)
	if len(deletes) != 1 || deletes[0] != "Gone/note.md" {
		t.Fatalf("want Delete Gone/note.md, got %v", plan)
	}
}

func TestSyncAutoMergesSimpleTypingConflict(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	token, deviceID := vaultPluginAuth(t, hs, "bob")

	base := "Shared intro that both copies still have.\n"
	pushNote(t, hs, token, deviceID, "Inbox/Hello.md", base, "")
	extended := base + "And then I kept typing.\n"
	m := pushNote(t, hs, token, deviceID, "Inbox/Hello.md", extended, "deadbeef")
	if conflicts, _ := m["conflicts"].([]any); len(conflicts) != 0 {
		t.Fatalf("typing continuation should auto-merge, got %v", m)
	}

	res, file := doJSON(t, http.MethodGet, hs.URL+"/api/v1/sync/file?path=Inbox/Hello.md", nil, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("get file %d %v", res.StatusCode, file)
	}
	raw, _ := base64.StdEncoding.DecodeString(file["content"].(string))
	if string(raw) != extended {
		t.Fatalf("merged content %q", raw)
	}
}

func TestSyncMoveFileAndFolder(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	token, deviceID := vaultPluginAuth(t, hs, "bob")

	note := "# moved\n"
	hash := fileSHA256([]byte(note))
	pushNote(t, hs, token, deviceID, "Inbox/Hello.md", note, "")

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/sync/push", map[string]any{
		"device_id": deviceID,
		"files": []map[string]any{
			{
				"path":      "Inbox/Hello.md",
				"hash":      "",
				"deleted":   true,
				"base_hash": hash,
				"mtime":     2,
			},
			{
				"path":         "Notes/Hello.md",
				"hash":         hash,
				"mtime":        2,
				"content":      base64.StdEncoding.EncodeToString([]byte(note)),
				"base_hash":    hash,
				"renamed_from": "Inbox/Hello.md",
			},
		},
	}, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("move file %d %v", res.StatusCode, m)
	}

	res, man := doJSON(t, http.MethodGet, hs.URL+"/api/v1/sync/manifest", nil, nil, token)
	files, _ := man["files"].([]any)
	if res.StatusCode != 200 || len(files) != 1 {
		t.Fatalf("manifest after file move: %d %v", res.StatusCode, man)
	}
	item, _ := files[0].(map[string]any)
	if item["path"] != "Notes/Hello.md" {
		t.Fatalf("moved path %v", item)
	}

	a, b := "# A\n", "# B\n"
	hashA, hashB := fileSHA256([]byte(a)), fileSHA256([]byte(b))
	pushNote(t, hs, token, deviceID, "Projects/a.md", a, "")
	pushNote(t, hs, token, deviceID, "Projects/nested/b.md", b, "")

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/sync/push", map[string]any{
		"device_id": deviceID,
		"files": []map[string]any{
			{
				"path":         "Archive/a.md",
				"hash":         hashA,
				"mtime":        4,
				"content":      base64.StdEncoding.EncodeToString([]byte(a)),
				"base_hash":    hashA,
				"renamed_from": "Projects/a.md",
			},
			{
				"path":         "Archive/nested/b.md",
				"hash":         hashB,
				"mtime":        4,
				"content":      base64.StdEncoding.EncodeToString([]byte(b)),
				"base_hash":    hashB,
				"renamed_from": "Projects/nested/b.md",
			},
		},
	}, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("move folder %d %v", res.StatusCode, m)
	}

	res, man = doJSON(t, http.MethodGet, hs.URL+"/api/v1/sync/manifest", nil, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("manifest %d %v", res.StatusCode, man)
	}
	got := map[string]bool{}
	for _, rawFile := range man["files"].([]any) {
		got[rawFile.(map[string]any)["path"].(string)] = true
	}
	if got["Projects/a.md"] || got["Projects/nested/b.md"] {
		t.Fatalf("old folder paths remained: %v", got)
	}
	if !got["Notes/Hello.md"] || !got["Archive/a.md"] || !got["Archive/nested/b.md"] {
		t.Fatalf("new paths missing: %v", got)
	}

	res, plan := doJSON(t, http.MethodPost, hs.URL+"/api/v1/sync/plan", map[string]any{
		"device_id": deviceID,
		"files": []map[string]any{
			{"path": "Notes/Hello.md", "hash": hash, "base_hash": hash},
			{"path": "Archive/a.md", "hash": hashA, "base_hash": hashA},
			{"path": "Archive/nested/b.md", "hash": hashB, "base_hash": hashB},
			{"path": "Projects/a.md", "hash": "", "deleted": true, "base_hash": hashA},
		},
	}, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("plan %d %v", res.StatusCode, plan)
	}
	if pulls, _ := plan["Pull"].([]any); len(pulls) != 0 {
		t.Fatalf("moved files came back as pull: %v", plan)
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
		`id="landing-brand"`,
		`github-signin`,
		`syncidian-github-signin`,
		`Continue with GitHub`,
		`Your vault.`,
		`Your data<br>stays with you.`,
		`id="hero-canvas"`,
		`snoise`,
		`id="auth-btn" type="button">Sign in</button`,
		`id="cinematic-footer"`,
		`id="nerds-toggle"`,
		`Stats for Nerds`,
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
		`Browse → search`,
		`shangeethsivan/Syncidian`,
		`Connect your GitHub repository`,
		`one GitHub repository`,
		`Connect with GitHub`,
		`id="gh-connect-app"`,
		`only support one single branch`,
		`Connect to GitHub`,
		`Register the GitHub App`,
		`docs/github-app.md`,
		`Obsidian on desktop`,
		`/assets/obsidian.zip`,
		`Android`,
		`iOS`,
		`id="landing-persist"`,
		`id="auth-persist"`,
		`persistWarningHTML`,
		`persist-banner`,
		`Data will reset on the next deploy`,
		`Settings → Volumes`,
		`The plugin never puts the token in a URL`,
		`id="setup-obsidian"`,
		`Restricted mode`,
		`already installed the app earlier`,
		`id="mcp-overview"`,
		`Connected MCP clients`,
		`MCP calls (7d)`,
		`mcpClientTable`,
		`rel="ai-catalog"`,
		`navigator.modelContext`,
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
		`href="/admin">Admin`,
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

func TestInvalidAccessTokenMessage(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/devices/register", map[string]string{
		"name": "macOS", "platform": "macOS",
	}, nil, "sk_sync_"+strings.Repeat("ab", 32))
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d %v", res.StatusCode, m)
	}
	if m["error"] != "invalid or revoked access token" {
		t.Fatalf("error message: %v", m)
	}

	// Valid token still registers.
	adminCookies := setupAdmin(t, hs)
	cookies := createAndLoginUser(t, hs, adminCookies, "carol")
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/tokens", map[string]string{"name": "plugin"}, cookies, "")
	token, _ := m["token"].(string)
	if token == "" {
		t.Fatalf("create token: %v", m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/devices/register", map[string]string{
		"name": "macOS", "platform": "macOS",
	}, nil, token)
	if res.StatusCode != 200 || m["id"] == "" {
		t.Fatalf("register with valid token: %d %v", res.StatusCode, m)
	}
}

func TestAdminCanIssueVaultUserToken(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	adminCookies := setupAdmin(t, hs)

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/users", map[string]any{
		"username": "vault1", "password": "password1", "admin": false, "issue_token": true, "token_name": "Mac",
	}, adminCookies, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create user+token: %d %v", res.StatusCode, m)
	}
	token, _ := m["token"].(string)
	if !strings.HasPrefix(token, "sk_sync_") {
		t.Fatalf("expected token on create: %v", m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/devices/register", map[string]string{
		"name": "macOS", "platform": "macOS",
	}, nil, token)
	if res.StatusCode != 200 || m["id"] == "" {
		t.Fatalf("register with admin-issued token: %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/users/tokens", map[string]any{
		"username": "vault1", "name": "Phone",
	}, adminCookies, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("admin issue token: %d %v", res.StatusCode, m)
	}
	token2, _ := m["token"].(string)
	if !strings.HasPrefix(token2, "sk_sync_") || token2 == token {
		t.Fatalf("expected a new token: %v", m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/devices/register", map[string]string{
		"name": "iPhone", "platform": "iOS",
	}, nil, token2)
	if res.StatusCode != 200 {
		t.Fatalf("register with second admin-issued token: %d %v", res.StatusCode, m)
	}

	// Admins cannot mint tokens for themselves.
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/users/tokens", map[string]any{
		"username": "ada", "name": "Nope",
	}, adminCookies, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("admin self-token: want 400, got %d %v", res.StatusCode, m)
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
	if m["admin_private"] != false {
		t.Fatalf("self-host default must skip Tailscale admin: %v", m["admin_private"])
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
	if !strings.Contains(loc, "github=error") {
		t.Fatalf("github start before setup should stay on the public site, got %q", loc)
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
	if !strings.Contains(githubURL, "state=") {
		t.Fatalf("install URL must include state so OAuth-during-install can be correlated: %q", githubURL)
	}
	gotInstallState := false
	gotInstallIntent := false
	for _, c := range res.Cookies() {
		if c.Name == "syncidian_github_state" && c.Value != "" {
			gotInstallState = true
		}
		if c.Name == "syncidian_install_intent" && c.Value != "" {
			gotInstallIntent = true
		}
	}
	if !gotInstallState || !gotInstallIntent {
		t.Fatalf("expected state + install intent cookies on Connect with GitHub, cookies=%v", res.Cookies())
	}

	// Pasting a full GitHub App URL as the slug must still produce a clean install URL.
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/register", map[string]any{
		"app_id": 100, "slug": "https://github.com/apps/syncidian/installations/new",
		"pem":       "-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----",
		"client_id": "Iv1.xyz", "client_secret": "secret2",
	}, adminCookies, "")
	if res.StatusCode != 200 {
		t.Fatalf("register app with URL slug: %d %v", res.StatusCode, m)
	}
	if m["slug"] != "syncidian" {
		t.Fatalf("register should normalize slug, got %v", m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/start", map[string]any{}, bob, "")
	if res.StatusCode != 200 {
		t.Fatalf("app start after URL slug: %d %v", res.StatusCode, m)
	}
	githubURL, _ = m["github_url"].(string)
	if !strings.HasPrefix(githubURL, "https://github.com/apps/syncidian/installations/new") {
		t.Fatalf("github_url must not double-prefix apps/: %q", githubURL)
	}
	if strings.Contains(githubURL, "apps/https://") {
		t.Fatalf("malformed install URL: %q", githubURL)
	}
	if !strings.Contains(githubURL, "?state=") {
		t.Fatalf("normalized slug install URL must still carry state: %q", githubURL)
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
	if !strings.Contains(loc, "https://github.com/login/oauth/authorize") || !strings.Contains(loc, "client_id=Iv1.xyz") {
		t.Fatalf("expected GitHub OAuth redirect, got %q", loc)
	}
}

func TestCustomAdminPathHidden(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "http://localhost", AdminPath: "/ops-gate"}, st, nil)
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()

	res, err := http.Get(hs.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("default /admin should 404 when AdminPath is custom, got %d", res.StatusCode)
	}
	res, err = http.Get(hs.URL + "/ops-gate")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.Contains(b, []byte(`isAdminPath`)) {
		t.Fatalf("custom operator path: %d", res.StatusCode)
	}
	res, body := getBody(t, hs.URL+"/robots.txt", "")
	if res.StatusCode != 200 || !strings.Contains(string(body), "Disallow: /ops-gate") {
		t.Fatalf("robots should disallow custom operator path:\n%s", body)
	}
}

func TestGitHubAuthCallbackBindsInstallationForSignedInUser(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	adminCookies := setupAdmin(t, hs)
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/register", map[string]any{
		"app_id": 42, "slug": "syncidian-bind", "pem": "-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----",
		"client_id": "Iv1.bind", "client_secret": "secret",
	}, adminCookies, "")
	if res.StatusCode != 200 {
		t.Fatalf("register: %d %v", res.StatusCode, m)
	}
	bob := createAndLoginUser(t, hs, adminCookies, "bob")

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/start", map[string]any{}, bob, "")
	if res.StatusCode != 200 {
		t.Fatalf("start: %d %v", res.StatusCode, m)
	}
	var state string
	cookies := append([]*http.Cookie{}, bob...)
	for _, c := range res.Cookies() {
		cookies = append(cookies, c)
		if c.Name == "syncidian_github_state" {
			state = c.Value
		}
	}
	if state == "" {
		t.Fatal("missing state cookie from Connect with GitHub")
	}

	// With Request user authorization during installation, GitHub returns to the
	// OAuth callback with installation_id (not the setup URL).
	req, err := http.NewRequest(http.MethodGet, hs.URL+"/api/v1/auth/github/callback?code=unused&installation_id=98765&setup_action=install&state="+state, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	redir, err := noFollowClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	redir.Body.Close()
	if redir.StatusCode != http.StatusFound {
		t.Fatalf("callback status: %d", redir.StatusCode)
	}
	loc := redir.Header.Get("Location")
	if strings.Contains(loc, "github=signed_in") {
		t.Fatalf("install return must not look like a plain sign-in: %q", loc)
	}
	if strings.Contains(loc, "sign-in expired") || strings.Contains(loc, "GitHub+sign-in+expired") {
		t.Fatalf("install return must accept the Connect state cookie: %q", loc)
	}
	// Fake PEM cannot mint an installation token, so expect an error redirect —
	// but the installation_id must already be stored for this user.
	if !strings.Contains(loc, "github=") {
		t.Fatalf("expected github query on redirect: %q", loc)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, bob, "")
	if res.StatusCode != 200 {
		t.Fatalf("github status: %d %v", res.StatusCode, m)
	}
	if m["installed"] != true {
		t.Fatalf("expected installation recorded after Install & Authorize callback: %v", m)
	}
}

func TestBearerTokenCannotManageGitHub(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	admin := setupAdmin(t, hs)
	cookies := createAndLoginUser(t, hs, admin, "bob")
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/tokens", map[string]string{"name": "plugin"}, cookies, "")
	token, _ := m["token"].(string)
	if token == "" {
		t.Fatalf("token: %v", m)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, nil, token)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("bearer GET github: want 403, got %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github", map[string]any{"repo": "bob/vault"}, nil, token)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("bearer POST github: want 403, got %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/app/start", map[string]any{}, nil, token)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("bearer github app start: want 403, got %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/github/sync", map[string]any{}, nil, token)
	if res.StatusCode == http.StatusForbidden {
		t.Fatalf("bearer github/sync should import GitHub for the plugin, got 403 %v", m)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bearer github/sync without repo: want 400, got %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github/tree", nil, nil, token)
	if res.StatusCode == http.StatusForbidden {
		t.Fatalf("bearer github/tree should list GitHub files for the plugin, got 403 %v", m)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bearer github/tree without repo: want 400, got %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/tokens", map[string]string{"name": "another"}, nil, token)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("bearer mint token: want 403, got %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/github", nil, cookies, "")
	if res.StatusCode != 200 {
		t.Fatalf("session GET github: %d %v", res.StatusCode, m)
	}
}

func TestBearerCanGetAndResolveConflict(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	token, deviceID := vaultPluginAuth(t, hs, "bob")

	note := "# Hello\n"
	pushNote(t, hs, token, deviceID, "Inbox/Hello.md", note, "")
	other := "# Other\n"
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/sync/push", map[string]any{
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
	if res.StatusCode != 200 || len(conflicts) != 1 {
		t.Fatalf("expected conflict, got %d %v", res.StatusCode, m)
	}
	c, _ := conflicts[0].(map[string]any)
	id, _ := c["id"].(string)
	if id == "" {
		t.Fatalf("conflict id: %v", c)
	}

	res, got := doJSON(t, http.MethodGet, hs.URL+"/api/v1/conflicts/"+id, nil, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("bearer GET conflict: want 200, got %d %v", res.StatusCode, got)
	}
	if got["path"] != "Inbox/Hello.md" {
		t.Fatalf("conflict payload: %v", got)
	}

	res, resolved := doJSON(t, http.MethodPost, hs.URL+"/api/v1/conflicts/"+id+"/resolve", map[string]any{
		"resolution": "local",
		"device_id":  deviceID,
	}, nil, token)
	if res.StatusCode != 200 {
		t.Fatalf("bearer resolve: want 200, got %d %v", res.StatusCode, resolved)
	}
}

func TestAccessTokenRejectedInQueryString(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	admin := setupAdmin(t, hs)
	cookies := createAndLoginUser(t, hs, admin, "bob")
	_, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/tokens", map[string]string{"name": "plugin"}, cookies, "")
	token, _ := m["token"].(string)

	req, err := http.NewRequest(http.MethodPost, hs.URL+"/api/v1/devices/register?token="+token, strings.NewReader(`{"name":"Mac","platform":"macOS"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	out, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Body.Close()
	if out.StatusCode != http.StatusUnauthorized {
		t.Fatalf("query token should be ignored, got %d", out.StatusCode)
	}
}

// TestCommunityPluginManifestAtRepoRoot checks git-root copies of plugin metadata.
// It does not write files into tests/; BRAT downloads GitHub Release attachments instead.
func TestCommunityPluginManifestAtRepoRoot(t *testing.T) {
	root := repoRoot(t)
	rootManifest := filepath.Join(root, "manifest.json")
	pluginManifest := filepath.Join(root, "plugin", "manifest.json")
	rootVersions := filepath.Join(root, "versions.json")
	pluginVersions := filepath.Join(root, "plugin", "versions.json")

	if diffFiles(t, pluginManifest, rootManifest) {
		t.Fatal("manifest.json at the repo root must match plugin/manifest.json (Obsidian reads the root file)")
	}
	if diffFiles(t, pluginVersions, rootVersions) {
		t.Fatal("versions.json at the repo root must match plugin/versions.json")
	}

	raw, err := os.ReadFile(rootManifest)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Version       string `json:"version"`
		MinAppVersion string `json:"minAppVersion"`
		Description   string `json:"description"`
		Author        string `json:"author"`
		IsDesktopOnly *bool  `json:"isDesktopOnly"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "syncidian" {
		t.Fatalf("plugin id %q", m.ID)
	}
	if strings.Contains(strings.ToLower(m.ID), "obsidian") {
		t.Fatal("plugin id must not contain obsidian")
	}
	if m.Name != "Syncidian" {
		t.Fatalf("plugin name %q", m.Name)
	}
	if m.Version == "" || m.MinAppVersion == "" || m.Description == "" || m.Author == "" {
		t.Fatalf("incomplete manifest: %+v", m)
	}
	if m.IsDesktopOnly == nil || *m.IsDesktopOnly {
		t.Fatal("isDesktopOnly must be false so Android and iOS can install the plugin")
	}

	pkgRaw, err := os.ReadFile(filepath.Join(root, "plugin", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(pkgRaw, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Version != m.Version {
		t.Fatalf("package.json version %s != manifest %s", pkg.Version, m.Version)
	}

	verRaw, err := os.ReadFile(rootVersions)
	if err != nil {
		t.Fatal(err)
	}
	var versions map[string]string
	if err := json.Unmarshal(verRaw, &versions); err != nil {
		t.Fatal(err)
	}
	if versions[m.Version] != m.MinAppVersion {
		t.Fatalf("versions.json %s => %s, want minAppVersion %s", m.Version, versions[m.Version], m.MinAppVersion)
	}

	for _, name := range []string{
		filepath.Join(root, ".github", "workflows", "release.yml"),
		filepath.Join(root, "LICENSE"),
		filepath.Join(root, "plugin", "main.js"),
		filepath.Join(root, "plugin", "styles.css"),
	} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("community plugin file missing: %s", name)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("go.mod not found from test working directory")
	return ""
}

func diffFiles(t *testing.T, a, b string) bool {
	t.Helper()
	left, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	return !bytes.Equal(left, right)
}

func TestWebsocketTicketNotRawToken(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	admin := setupAdmin(t, hs)
	cookies := createAndLoginUser(t, hs, admin, "bob")
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/tokens", map[string]string{"name": "plugin"}, cookies, "")
	token, _ := m["token"].(string)

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/ws?token="+token, nil, nil, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("raw token in WS URL: want 400, got %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/ws/ticket", map[string]any{}, nil, token)
	ticket, _ := m["ticket"].(string)
	if res.StatusCode != 200 || !strings.HasPrefix(ticket, "wst_") {
		t.Fatalf("ticket: %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/ws?ticket=wst_nope", nil, nil, "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad ticket: want 401, got %d %v", res.StatusCode, m)
	}
}

func TestMCPLoginAndTools(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	adminCookies := setupAdmin(t, hs)
	_ = createAndLoginUser(t, hs, adminCookies, "vault1")

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/mcp/login", map[string]string{
		"username": "vault1", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mcp login: %d %v", res.StatusCode, m)
	}
	token, _ := m["token"].(string)
	if !strings.HasPrefix(token, "sk_sync_") {
		t.Fatalf("expected sk_sync_ token: %v", m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/mcp/login", map[string]string{
		"username": "ada", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("admin mcp login: want 403, got %d %v", res.StatusCode, m)
	}

	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{},
	})
	req, err := http.NewRequest(http.MethodPost, hs.URL+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("tools/list: %d %s", resp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "search_notes") || !strings.Contains(string(raw), "get_graph") {
		t.Fatalf("expected graph/search tools: %s", raw)
	}
}

func TestMCPUsageShowsOnDashboard(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	adminCookies := setupAdmin(t, hs)
	cookies := createAndLoginUser(t, hs, adminCookies, "bob")
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/tokens", map[string]string{"name": "Claude"}, cookies, "")
	token, _ := m["token"].(string)
	if !strings.HasPrefix(token, "sk_sync_") {
		t.Fatalf("token %v", m)
	}

	postMCP := func(body map[string]any) {
		t.Helper()
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, hs.URL+"/mcp", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "claude-code/2.0")
		out, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(out.Body)
		out.Body.Close()
		if out.StatusCode != 200 {
			t.Fatalf("mcp status %d %s", out.StatusCode, raw)
		}
	}

	postMCP(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"clientInfo":      map[string]string{"name": "claude-code", "version": "2.0.0"},
		},
	})
	postMCP(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": "list_notes", "arguments": map[string]any{}},
	})
	postMCP(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": "search_notes", "arguments": map[string]any{"query": "x"}},
	})

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/mcp", nil, cookies, "")
	if res.StatusCode != 200 {
		t.Fatalf("mcp status %d %v", res.StatusCode, m)
	}
	usage, _ := m["usage"].(map[string]any)
	if usage == nil {
		t.Fatalf("missing usage: %v", m)
	}
	if usage["client_count"] != float64(1) {
		t.Fatalf("client_count: %v", usage)
	}
	if usage["total_calls"] != float64(2) {
		t.Fatalf("total_calls: %v", usage)
	}
	clients, _ := usage["clients"].([]any)
	if len(clients) != 1 {
		t.Fatalf("clients: %v", usage)
	}
	c, _ := clients[0].(map[string]any)
	if c["name"] != "claude-code" {
		t.Fatalf("client name: %v", c)
	}
	if c["call_count"] != float64(2) {
		t.Fatalf("client call_count: %v", c)
	}
	tools, _ := usage["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("tools: %v", usage)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/stats", nil, cookies, "")
	if res.StatusCode != 200 {
		t.Fatalf("stats %d %v", res.StatusCode, m)
	}
	if m["mcp_client_count"] != float64(1) || m["mcp_total_calls"] != float64(2) {
		t.Fatalf("stats mcp fields: %v", m)
	}

	res, m = doJSON(t, http.MethodGet, hs.URL+"/api/v1/mcp", nil, adminCookies, "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("admin mcp usage: want 403, got %d %v", res.StatusCode, m)
	}
}

func TestGitHubAppSetupRejectsSpoofedInstallation(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	adminCookies := setupAdmin(t, hs)
	bob := createAndLoginUser(t, hs, adminCookies, "bob")

	getSetup := func(cookies []*http.Cookie, rawQuery string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, hs.URL+"/api/v1/github/app/setup?"+rawQuery, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		res, err := noFollowClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := getSetup(bob, "installation_id=99999")
	res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("missing state: want 302, got %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "github=error") || !strings.Contains(loc, "expired") {
		t.Fatalf("missing state should fail closed, got %q", loc)
	}
	for _, c := range res.Cookies() {
		if c.Name == "syncidian_pending_install" && c.Value != "" && c.MaxAge >= 0 {
			t.Fatalf("must not stash a spoofed installation_id: %+v", c)
		}
	}

	bob = append(bob, &http.Cookie{Name: "syncidian_github_state", Value: "nonce-from-connect"})
	res = getSetup(bob, "installation_id=99999&state=attacker-guess")
	res.Body.Close()
	if loc = res.Header.Get("Location"); !strings.Contains(loc, "github=error") {
		t.Fatalf("mismatched state should fail closed, got %q", loc)
	}

	res = getSetup(bob, "installation_id=99999&state=nonce-from-connect")
	res.Body.Close()
	loc = res.Header.Get("Location")
	if strings.Contains(loc, "expired") {
		t.Fatalf("matching state should pass CSRF check, got %q", loc)
	}
	if !strings.Contains(loc, "github=error") {
		t.Fatalf("without an app, matching state should still error on missing credentials, got %q", loc)
	}

	anon := getSetup(nil, "installation_id=99999")
	anon.Body.Close()
	if loc = anon.Header.Get("Location"); !strings.Contains(loc, "github=error") {
		t.Fatalf("anonymous spoof should fail closed, got %q", loc)
	}
	for _, c := range anon.Cookies() {
		if c.Name == "syncidian_pending_install" && c.Value != "" && c.MaxAge >= 0 {
			t.Fatalf("anonymous spoof must not set pending_install: %+v", c)
		}
	}

	anonOK := getSetup([]*http.Cookie{{Name: "syncidian_github_state", Value: "nonce-from-connect"}}, "installation_id=4242&state=nonce-from-connect")
	anonOK.Body.Close()
	if loc = anonOK.Header.Get("Location"); !strings.Contains(loc, "/api/v1/auth/github/start") {
		t.Fatalf("anonymous with valid state should continue to GitHub login, got %q", loc)
	}
	gotPending := false
	for _, c := range anonOK.Cookies() {
		if c.Name == "syncidian_pending_install" && c.Value == "4242" {
			gotPending = true
		}
	}
	if !gotPending {
		t.Fatalf("anonymous with valid state should stash installation, cookies=%v", anonOK.Cookies())
	}
}

func TestCORSAllowsObsidianMobileOrigins(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	origins := []string{
		"app://obsidian.md",
		"capacitor://localhost",
		"ionic://localhost",
		"http://localhost",
		"https://localhost",
	}
	for _, origin := range origins {
		req, err := http.NewRequest(http.MethodOptions, hs.URL+"/api/v1/devices/register", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-syncidian-client")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("%s OPTIONS: want 204, got %d", origin, res.StatusCode)
		}
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("%s Access-Control-Allow-Origin: got %q", origin, got)
		}
		allow := strings.ToLower(res.Header.Get("Access-Control-Allow-Headers"))
		if !strings.Contains(allow, "authorization") || !strings.Contains(allow, "x-syncidian-client") {
			t.Fatalf("%s Allow-Headers: %q", origin, allow)
		}
		methods := res.Header.Get("Access-Control-Allow-Methods")
		if !strings.Contains(methods, "POST") {
			t.Fatalf("%s Allow-Methods: %q", origin, methods)
		}
		if res.Header.Get("Access-Control-Allow-Credentials") != "true" {
			t.Fatalf("%s missing Allow-Credentials", origin)
		}
	}
}

func TestCORSDoesNotReflectArbitraryOrigins(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()
	evil := "https://evil.example"
	paths := []string{"/api/v1/auth/login", "/api/v1/mcp/login", "/api/v1/devices/register"}
	for _, path := range paths {
		req, err := http.NewRequest(http.MethodOptions, hs.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", evil)
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "content-type")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("%s reflected Origin %q", path, got)
		}

		req, err = http.NewRequest(http.MethodPost, hs.URL+path, strings.NewReader(`{"username":"x","password":"y"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", evil)
		req.Header.Set("Content-Type", "application/json")
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("%s POST reflected Origin %q", path, got)
		}
	}

	req, err := http.NewRequest(http.MethodOptions, hs.URL+"/api/v1/mcp/login", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "http://localhost")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost" {
		t.Fatalf("allowlisted origin: got %q", got)
	}
}

func TestMCPWriteRequiresGitHubAndSkipsServerVault(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "http://localhost"}, st, nil)
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()

	adminCookies := setupAdmin(t, hs)
	cookies := createAndLoginUser(t, hs, adminCookies, "bob")
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/mcp", map[string]any{
		"search": true, "read": true, "create": true, "modify": true,
	}, cookies, "")
	if res.StatusCode != 200 {
		t.Fatalf("set mcp: %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/tokens", map[string]string{"name": "MCP"}, cookies, "")
	token, _ := m["token"].(string)

	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "create_note",
			"arguments": map[string]any{"path": "Weekly Focus.md", "content": "# Weekly Focus\n"},
		},
	})
	req, err := http.NewRequest(http.MethodPost, hs.URL+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	out, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(out.Body)
	out.Body.Close()
	if out.StatusCode != 200 {
		t.Fatalf("status %d %s", out.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "does not store notes on the server") && !strings.Contains(string(raw), "connect a GitHub") {
		t.Fatalf("expected GitHub-required error, got %s", raw)
	}
	bob, err := st.GetUserByUsername("bob")
	if err != nil || bob == nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.VaultDir(bob.ID), "Weekly Focus.md")); err == nil {
		t.Fatal("MCP must not save notes on the server")
	}
}
