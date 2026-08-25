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
	GitName            string
	GitEmail           string
	GitHubAppID        int64
	GitHubAppSlug      string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubAppPEM       string
}

func FromEnv() Config {
	c := Config{
		Addr:               firstEnv("SYNCIDIAN_ADDR", "PORT"),
		DataDir:            resolveDataDir(),
		PublicURL:          publicURL(),
		GitName:            env("SYNCIDIAN_GIT_NAME", "Syncidian"),
		GitEmail:           env("SYNCIDIAN_GIT_EMAIL", "syncidian@localhost"),
		GitHubAppSlug:      githubapp.NormalizeAppSlug(env("SYNCIDIAN_GITHUB_APP_SLUG", "")),
		GitHubClientID:     env("SYNCIDIAN_GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: env("SYNCIDIAN_GITHUB_CLIENT_SECRET", ""),
		GitHubAppPEM:       strings.ReplaceAll(env("SYNCIDIAN_GITHUB_APP_PRIVATE_KEY", ""), `\n`, "\n"),
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
