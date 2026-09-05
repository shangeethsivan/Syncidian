package config

import (
	"encoding/json"
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
	AdminPrivate       bool
	GitName            string
	GitEmail           string
	GitHubAppID        int64
	GitHubAppSlug      string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubAppPEM       string
	// GitHubAllowedEmails, when non-empty, is the only set of GitHub account
	// emails that may complete GitHub sign-in. Empty means any GitHub user.
	// Set SYNCIDIAN_GITHUB_ALLOWED_EMAILS on Railway to grant waitlist people
	// access without opening sign-in to everyone. SYNCIDIAN_GITHUB_ALLOWED_EMAIL
	// is still accepted as an alias. PRODUCTION: unset both (and show the
	// landing GitHub buttons) when opening sign-in to everyone. See AGENT.md.
	GitHubAllowedEmails []string
	// GAID is a Google Analytics 4 measurement ID (G-…). Empty disables gtag.
	// Set SYNCIDIAN_GA_ID on hosted Railway to count landing CTA clicks.
	GAID string
}

func FromEnv() Config {
	c := Config{
		Addr:                firstEnv("SYNCIDIAN_ADDR", "PORT"),
		DataDir:             resolveDataDir(),
		PublicURL:           publicURL(),
		AdminPath:           NormalizeAdminPath(env("SYNCIDIAN_ADMIN_PATH", "")),
		AdminHost:           "",
		AdminListenIP:       "",
		AdminPrivate:        false,
		GitName:             env("SYNCIDIAN_GIT_NAME", "Syncidian"),
		GitEmail:            env("SYNCIDIAN_GIT_EMAIL", "syncidian@localhost"),
		GitHubAppSlug:       githubapp.NormalizeAppSlug(env("SYNCIDIAN_GITHUB_APP_SLUG", "")),
		GitHubClientID:      env("SYNCIDIAN_GITHUB_CLIENT_ID", ""),
		GitHubClientSecret:  env("SYNCIDIAN_GITHUB_CLIENT_SECRET", ""),
		GitHubAppPEM:        strings.ReplaceAll(env("SYNCIDIAN_GITHUB_APP_PRIVATE_KEY", ""), `\n`, "\n"),
		GitHubAllowedEmails: GitHubAllowedEmailsFromEnv(),
		GAID:                NormalizeGAID(env("SYNCIDIAN_GA_ID", "")),
	}
	if id, err := strconv.ParseInt(env("SYNCIDIAN_GITHUB_APP_ID", "0"), 10, 64); err == nil {
		c.GitHubAppID = id
	}
	c.AdminHost, c.AdminListenIP, c.AdminPrivate = adminPrivateFromEnv()
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

// ParseOptionalBool reads a 0/1 (or true/false/on/off) env var.
// ok is false when the variable is unset or empty.
func ParseOptionalBool(key string) (ok, val bool) {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return false, false
	}
	switch v {
	case "0", "false", "no", "off":
		return true, false
	case "1", "true", "yes", "on":
		return true, true
	default:
		return true, true
	}
}

// adminPrivateFromEnv is opt-in. Self-hosters skip Tailscale by leaving these
// unset, or set SYNCIDIAN_ADMIN_PRIVATE=0 to ignore a copied host/IP.
func adminPrivateFromEnv() (host, listenIP string, on bool) {
	if set, val := ParseOptionalBool("SYNCIDIAN_ADMIN_PRIVATE"); set && !val {
		return "", "", false
	}
	host = NormalizeAdminHost(env("SYNCIDIAN_ADMIN_HOST", ""))
	listenIP = NormalizeListenIP(env("SYNCIDIAN_ADMIN_LISTEN_IP", ""))
	if listenIP == "" && host != "" {
		listenIP = NormalizeListenIP(env("TAILSCALE_IP", ""))
	}
	return host, listenIP, host != "" || listenIP != ""
}

// NormalizeGAID accepts a GA4 measurement ID (G-XXXXXXXX). Anything else is empty.
func NormalizeGAID(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if !strings.HasPrefix(s, "G-") || len(s) < 4 {
		return ""
	}
	for _, c := range s[2:] {
		ok := (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !ok {
			return ""
		}
	}
	return s
}

// GitHubAllowedEmailsFromEnv reads SYNCIDIAN_GITHUB_ALLOWED_EMAILS (preferred)
// and the singular SYNCIDIAN_GITHUB_ALLOWED_EMAIL alias, then merges them.
func GitHubAllowedEmailsFromEnv() []string {
	return MergeEmailLists(
		ParseEmailList(os.Getenv("SYNCIDIAN_GITHUB_ALLOWED_EMAILS")),
		ParseEmailList(os.Getenv("SYNCIDIAN_GITHUB_ALLOWED_EMAIL")),
	)
}

// ParseEmailList turns an environment value into unique, lowercased emails.
// It accepts a JSON array, commas, semicolons, or one address per line so the
// allowlist can be edited as an array on Railway without a code change.
func ParseEmailList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var arr []string
		if json.Unmarshal([]byte(raw), &arr) == nil {
			return uniqueLowerEmails(arr)
		}
	}
	replacer := strings.NewReplacer("\r\n", ",", "\n", ",", "\r", ",", ";", ",")
	return uniqueLowerEmails(strings.Split(replacer.Replace(raw), ","))
}

// MergeEmailLists concatenates allowlists, dropping blanks and duplicates.
func MergeEmailLists(lists ...[]string) []string {
	var all []string
	for _, list := range lists {
		all = append(all, list...)
	}
	return uniqueLowerEmails(all)
}

func uniqueLowerEmails(parts []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, part := range parts {
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

// FormatEmailListJSON is the value to paste into SYNCIDIAN_GITHUB_ALLOWED_EMAILS.
func FormatEmailListJSON(emails []string) string {
	emails = uniqueLowerEmails(emails)
	if len(emails) == 0 {
		return "[]"
	}
	b, err := json.Marshal(emails)
	if err != nil {
		return "[]"
	}
	return string(b)
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
