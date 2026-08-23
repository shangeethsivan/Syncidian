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
