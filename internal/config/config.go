package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shangeethsivan/Syncidian/internal/githubapp"
)

// DefaultContainerDataDir is the image default (Dockerfile ENV SYNCIDIAN_DATA).
// An attached Railway volume wins over this path so operators do not have to
// match the mount path exactly.
const DefaultContainerDataDir = "/data"

type Config struct {
	Addr               string
	DataDir            string
	PublicURL          string
	AdminPath          string
	AdminHost          string
	AdminListenIP      string
	GitName            string
	GitEmail           string
	GitHubAppID        int64
	GitHubAppSlug      string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubAppPEM       string
	// GitHubAllowedEmails, when non-empty, is the only set of GitHub account
	// emails that may complete GitHub sign-in. Empty means any GitHub user.
	// Set SYNCIDIAN_GITHUB_ALLOWED_EMAIL on Railway to restrict hosted sign-in
	// until public launch. PRODUCTION: unset it (and show the landing GitHub
	// buttons) when opening sign-in to everyone. See AGENT.md.
	GitHubAllowedEmails []string
}

func FromEnv() Config {
	c := Config{
		Addr:                firstEnv("SYNCIDIAN_ADDR", "PORT"),
		DataDir:             resolveDataDir(),
		PublicURL:           publicURL(),
		AdminPath:           NormalizeAdminPath(env("SYNCIDIAN_ADMIN_PATH", "")),
		AdminHost:           NormalizeAdminHost(env("SYNCIDIAN_ADMIN_HOST", "")),
		AdminListenIP:       NormalizeListenIP(firstEnv("SYNCIDIAN_ADMIN_LISTEN_IP", "TAILSCALE_IP")),
		GitName:             env("SYNCIDIAN_GIT_NAME", "Syncidian"),
		GitEmail:            env("SYNCIDIAN_GIT_EMAIL", "syncidian@localhost"),
		GitHubAppSlug:       githubapp.NormalizeAppSlug(env("SYNCIDIAN_GITHUB_APP_SLUG", "")),
		GitHubClientID:      env("SYNCIDIAN_GITHUB_CLIENT_ID", ""),
		GitHubClientSecret:  env("SYNCIDIAN_GITHUB_CLIENT_SECRET", ""),
		GitHubAppPEM:        strings.ReplaceAll(env("SYNCIDIAN_GITHUB_APP_PRIVATE_KEY", ""), `\n`, "\n"),
		GitHubAllowedEmails: ParseEmailList(env("SYNCIDIAN_GITHUB_ALLOWED_EMAIL", "")),
	}
	if id, err := strconv.ParseInt(env("SYNCIDIAN_GITHUB_APP_ID", "0"), 10, 64); err == nil {
		c.GitHubAppID = id
	}
	if c.Addr == "" {
		c.Addr = ":8080"
	} else if !strings.HasPrefix(c.Addr, ":") && !strings.Contains(c.Addr, ":") {
		c.Addr = ":" + c.Addr
	}
	abs, err := filepath.Abs(c.DataDir)
	if err == nil {
		c.DataDir = abs
	}
	return c
}

func resolveDataDir() string {
	explicit := strings.TrimSpace(os.Getenv("SYNCIDIAN_DATA"))
	volume := strings.TrimSpace(os.Getenv("RAILWAY_VOLUME_MOUNT_PATH"))
	// The image sets SYNCIDIAN_DATA=/data. That hid Railway volumes mounted at
	// any other path, so users/vaults/GitHub App config vanished on redeploy.
	if volume != "" && (explicit == "" || explicit == DefaultContainerDataDir) {
		return volume
	}
	if explicit != "" {
		return explicit
	}
	return "./data"
}

func publicURL() string {
	if v := env("SYNCIDIAN_PUBLIC_URL", ""); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := env("RAILWAY_PUBLIC_DOMAIN", ""); v != "" {
		v = strings.TrimRight(v, "/")
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return v
		}
		return "https://" + v
	}
	return "http://localhost:8080"
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func env(key, fallback string) string {
	if v := firstEnv(key); v != "" {
		return v
	}
	return fallback
}

// ParseEmailList splits a comma-separated email list, lowercased and trimmed.
func ParseEmailList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, part := range strings.Split(raw, ",") {
		e := strings.ToLower(strings.TrimSpace(part))
		if e == "" {
			continue
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	return out
}

// EmailAllowed reports whether any of emails is on allowlist. An empty
// allowlist allows every address (production GitHub sign-in for all users).
func EmailAllowed(allowlist []string, emails ...string) bool {
	if len(allowlist) == 0 {
		return true
	}
	allowed := make(map[string]struct{}, len(allowlist))
	for _, a := range allowlist {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" {
			allowed[a] = struct{}{}
		}
	}
	for _, e := range emails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if _, ok := allowed[e]; ok {
			return true
		}
	}
	return false
}

// NormalizeAdminPath returns a single-segment operator path. Default is /admin.
// The public landing does not link here; set SYNCIDIAN_ADMIN_PATH to an
// unguessable value if the instance is on the public internet.
func NormalizeAdminPath(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "/admin"
	}
	p = strings.TrimRight(p, "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if p == "/" || strings.Contains(p, "//") || strings.Contains(p, "..") || strings.Count(p, "/") != 1 {
		return "/admin"
	}
	lower := strings.ToLower(p)
	reserved := []string{
		"/api", "/assets", "/mcp", "/health", "/ready", "/auth.md",
		"/openapi.json", "/robots.txt", "/sitemap.xml", "/favicon.ico",
	}
	for _, r := range reserved {
		if lower == r {
			return "/admin"
		}
	}
	for _, c := range p[1:] {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
		if !ok {
			return "/admin"
		}
	}
	return p
}
