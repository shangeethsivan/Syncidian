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
		Addr:      env("SYNCIDIAN_ADDR", ":8080"),
		DataDir:   env("SYNCIDIAN_DATA", "./data"),
		PublicURL: env("SYNCIDIAN_PUBLIC_URL", "http://localhost:8080"),
		GitName:   env("SYNCIDIAN_GIT_NAME", "Syncidian"),
		GitEmail:  env("SYNCIDIAN_GIT_EMAIL", "syncidian@localhost"),
	}
	if !strings.HasPrefix(c.Addr, ":") && !strings.Contains(c.Addr, ":") {
		c.Addr = ":" + c.Addr
	}
	abs, err := filepath.Abs(c.DataDir)
	if err == nil {
		c.DataDir = abs
	}
	return c
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
