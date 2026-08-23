package config

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr      string
	DataDir   string
	PublicURL string
	GitName   string
	GitEmail  string
}

func FromEnv() Config {
	c := Config{
		Addr:      firstEnv("SYNCIDIAN_ADDR", "PORT"),
		DataDir:   firstEnv("SYNCIDIAN_DATA", "RAILWAY_VOLUME_MOUNT_PATH"),
		PublicURL: publicURL(),
		GitName:   env("SYNCIDIAN_GIT_NAME", "Syncidian"),
		GitEmail:  env("SYNCIDIAN_GIT_EMAIL", "syncidian@localhost"),
	}
	if c.Addr == "" {
		c.Addr = ":8080"
	} else if !strings.HasPrefix(c.Addr, ":") && !strings.Contains(c.Addr, ":") {
		c.Addr = ":" + c.Addr
	}
	if c.DataDir == "" {
		c.DataDir = "./data"
	}
	abs, err := filepath.Abs(c.DataDir)
	if err == nil {
		c.DataDir = abs
	}
	return c
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
