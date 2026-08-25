package config

import "testing"

func TestPersistenceRailwayVolume(t *testing.T) {
	t.Setenv("RAILWAY_VOLUME_MOUNT_PATH", "/mnt/data")
	t.Setenv("RAILWAY_ENVIRONMENT", "production")
	p := PersistenceStatus("/mnt/data")
	if !p.OK || p.Kind != "volume" {
		t.Fatalf("got %+v", p)
	}
}

func TestPersistenceRailwayWithoutVolume(t *testing.T) {
	t.Setenv("RAILWAY_VOLUME_MOUNT_PATH", "")
	t.Setenv("RAILWAY_ENVIRONMENT", "production")
	t.Setenv("RAILWAY_PROJECT_ID", "proj")
	p := PersistenceStatus("/data")
	if p.OK || p.Kind != "ephemeral" {
		t.Fatalf("got %+v", p)
	}
	if p.Message == "" || p.Hint == "" {
		t.Fatalf("expected operator message and hint, got %+v", p)
	}
	if p.DataDir != "/data" {
		t.Fatalf("DataDir=%q", p.DataDir)
	}
}

func TestPersistenceRenderWithoutVolume(t *testing.T) {
	t.Setenv("RAILWAY_VOLUME_MOUNT_PATH", "")
	t.Setenv("RAILWAY_ENVIRONMENT", "")
	t.Setenv("RAILWAY_PROJECT_ID", "")
	t.Setenv("RENDER", "true")
	p := PersistenceStatus("/data")
	if p.OK || p.Kind != "ephemeral" {
		t.Fatalf("got %+v", p)
	}
	if p.Hint == "" {
		t.Fatal("expected Render hint")
	}
}

func TestFromEnvRailwayVolumeWinsOverImageDefault(t *testing.T) {
	t.Setenv("SYNCIDIAN_DATA", DefaultContainerDataDir)
	t.Setenv("RAILWAY_VOLUME_MOUNT_PATH", "/mnt/syncidian")
	c := FromEnv()
	if c.DataDir != "/mnt/syncidian" {
		t.Fatalf("DataDir=%q, want railway volume so image default /data does not hide it", c.DataDir)
	}
}
