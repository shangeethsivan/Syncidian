package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shangeethsivan/Syncidian/internal/config"
	"github.com/shangeethsivan/Syncidian/internal/store"
)

func TestReindexVaultTombstonesMissingFiles(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	srv := New(config.Config{Addr: ":0", DataDir: dir}, st, nil)
	u, err := st.CreateUser("alice", "hash", false)
	if err != nil {
		t.Fatal(err)
	}
	root := st.VaultDir(u.ID)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.md"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertFile(store.FileMeta{UserID: u.ID, Path: "keep.md", Hash: "oldkeep"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertFile(store.FileMeta{UserID: u.ID, Path: "gone.md", Hash: "abc"}); err != nil {
		t.Fatal(err)
	}
	if err := srv.reindexVault(u.ID); err != nil {
		t.Fatal(err)
	}
	gone, err := st.GetFile(u.ID, "gone.md")
	if err != nil || gone == nil || !gone.Deleted || gone.Hash != "" {
		t.Fatalf("gone.md should be tombstoned with empty hash: %+v %v", gone, err)
	}
	keep, err := st.GetFile(u.ID, "keep.md")
	if err != nil || keep == nil || keep.Deleted || keep.Hash == "oldkeep" {
		t.Fatalf("keep.md should stay live and be rehashed: %+v %v", keep, err)
	}
}
