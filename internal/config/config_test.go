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
