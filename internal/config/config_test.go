package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("SYNCIDIAN_ADDR", "")
	t.Setenv("PORT", "")
	t.Setenv("SYNCIDIAN_DATA", "")
	t.Setenv("RAILWAY_VOLUME_MOUNT_PATH", "")
	t.Setenv("SYNCIDIAN_PUBLIC_URL", "")
	t.Setenv("RAILWAY_PUBLIC_DOMAIN", "")
	t.Setenv("SYNCIDIAN_GITHUB_ALLOWED_EMAIL", "")

	c := FromEnv()
	if c.Addr != ":8080" {
		t.Fatalf("Addr=%q, want :8080", c.Addr)
	}
	if !strings.HasSuffix(c.DataDir, string(filepath.Separator)+"data") && !strings.HasSuffix(c.DataDir, "/data") {
		t.Fatalf("DataDir=%q, want .../data", c.DataDir)
	}
	if c.PublicURL != "http://localhost:8080" {
		t.Fatalf("PublicURL=%q", c.PublicURL)
	}
	if c.AdminPath != "/admin" {
		t.Fatalf("AdminPath=%q, want /admin", c.AdminPath)
	}
}

func TestFromEnvPortWhenAddrUnset(t *testing.T) {
	t.Setenv("SYNCIDIAN_ADDR", "")
	t.Setenv("PORT", "9090")
	c := FromEnv()
	if c.Addr != ":9090" {
		t.Fatalf("Addr=%q, want :9090 (Railway/Heroku PORT)", c.Addr)
	}
}

func TestFromEnvAddrWinsOverPort(t *testing.T) {
	t.Setenv("SYNCIDIAN_ADDR", ":7070")
	t.Setenv("PORT", "9090")
	c := FromEnv()
	if c.Addr != ":7070" {
		t.Fatalf("Addr=%q, want :7070", c.Addr)
	}
}

func TestFromEnvRailwayVolumeAndDomain(t *testing.T) {
	t.Setenv("SYNCIDIAN_DATA", "")
	t.Setenv("RAILWAY_VOLUME_MOUNT_PATH", "/mnt/syncidian")
	t.Setenv("SYNCIDIAN_PUBLIC_URL", "")
	t.Setenv("RAILWAY_PUBLIC_DOMAIN", "syncidian-prod.up.railway.app")

	c := FromEnv()
	if c.DataDir != "/mnt/syncidian" {
		t.Fatalf("DataDir=%q, want /mnt/syncidian", c.DataDir)
	}
	if c.PublicURL != "https://syncidian-prod.up.railway.app" {
		t.Fatalf("PublicURL=%q", c.PublicURL)
	}
}

func TestFromEnvExplicitValuesWin(t *testing.T) {
	t.Setenv("SYNCIDIAN_DATA", "/custom-data")
	t.Setenv("RAILWAY_VOLUME_MOUNT_PATH", "/mnt/syncidian")
	t.Setenv("SYNCIDIAN_PUBLIC_URL", "https://sync.example.com/")
	t.Setenv("RAILWAY_PUBLIC_DOMAIN", "ignored.up.railway.app")

	c := FromEnv()
	if c.DataDir != "/custom-data" {
		t.Fatalf("DataDir=%q, want /custom-data", c.DataDir)
	}
	if c.PublicURL != "https://sync.example.com" {
		t.Fatalf("PublicURL=%q", c.PublicURL)
	}
}

func TestFromEnvGitHubApp(t *testing.T) {
	t.Setenv("SYNCIDIAN_GITHUB_APP_ID", "123")
	t.Setenv("SYNCIDIAN_GITHUB_APP_SLUG", "syncidian")
	t.Setenv("SYNCIDIAN_GITHUB_CLIENT_ID", "Iv1.x")
	t.Setenv("SYNCIDIAN_GITHUB_CLIENT_SECRET", "s")
	t.Setenv("SYNCIDIAN_GITHUB_APP_PRIVATE_KEY", "-----BEGIN KEY-----\\nABC\\n-----END KEY-----")
	c := FromEnv()
	if c.GitHubAppID != 123 || c.GitHubAppSlug != "syncidian" || c.GitHubClientID != "Iv1.x" {
		t.Fatalf("github app env: %+v", c)
	}
	if !strings.Contains(c.GitHubAppPEM, "\n") {
		t.Fatalf("PEM newlines should be unescaped: %q", c.GitHubAppPEM)
	}
}

func TestFromEnvGitHubAppSlugNormalizesURL(t *testing.T) {
	t.Setenv("SYNCIDIAN_GITHUB_APP_ID", "123")
	t.Setenv("SYNCIDIAN_GITHUB_APP_SLUG", "https://github.com/apps/syncidian/installations/new")
	t.Setenv("SYNCIDIAN_GITHUB_CLIENT_ID", "Iv1.x")
	t.Setenv("SYNCIDIAN_GITHUB_CLIENT_SECRET", "s")
	t.Setenv("SYNCIDIAN_GITHUB_APP_PRIVATE_KEY", "pem")
	c := FromEnv()
	if c.GitHubAppSlug != "syncidian" {
		t.Fatalf("GitHubAppSlug=%q, want syncidian", c.GitHubAppSlug)
	}
}

func TestParseEmailList(t *testing.T) {
	if got := ParseEmailList(""); got != nil {
		t.Fatalf("empty: %v", got)
	}
	got := ParseEmailList("  Shangeeth95@gmail.com, other@example.com, SHANGEETH95@gmail.com ")
	if len(got) != 2 || got[0] != "shangeeth95@gmail.com" || got[1] != "other@example.com" {
		t.Fatalf("got %v", got)
	}
}

func TestEmailAllowed(t *testing.T) {
	if !EmailAllowed(nil, "anyone@example.com") {
		t.Fatal("empty allowlist should allow all")
	}
	list := []string{"shangeeth95@gmail.com"}
	if EmailAllowed(list, "other@example.com") {
		t.Fatal("other email must be denied")
	}
	if !EmailAllowed(list, "Shangeeth95@gmail.com") {
		t.Fatal("case-insensitive match")
	}
	if !EmailAllowed(list, "noreply@users.noreply.github.com", "shangeeth95@gmail.com") {
		t.Fatal("any matching email in the list should pass")
	}
}

func TestFromEnvGitHubAllowedEmail(t *testing.T) {
	t.Setenv("SYNCIDIAN_GITHUB_ALLOWED_EMAIL", "Shangeeth95@gmail.com")
	c := FromEnv()
	if len(c.GitHubAllowedEmails) != 1 || c.GitHubAllowedEmails[0] != "shangeeth95@gmail.com" {
		t.Fatalf("GitHubAllowedEmails=%v", c.GitHubAllowedEmails)
	}
}

func TestNormalizeAdminPath(t *testing.T) {
	if got := NormalizeAdminPath(""); got != "/admin" {
		t.Fatalf("empty: %q", got)
	}
	if got := NormalizeAdminPath("ops-secret"); got != "/ops-secret" {
		t.Fatalf("relative: %q", got)
	}
	if got := NormalizeAdminPath("/api"); got != "/admin" {
		t.Fatalf("reserved: %q", got)
	}
	if got := NormalizeAdminPath("/ops/nested"); got != "/admin" {
		t.Fatalf("nested: %q", got)
	}
	t.Setenv("SYNCIDIAN_ADMIN_PATH", "gate-9f3")
	c := FromEnv()
	if c.AdminPath != "/gate-9f3" {
		t.Fatalf("from env: %q", c.AdminPath)
	}
}
