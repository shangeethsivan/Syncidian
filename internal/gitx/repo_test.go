package gitx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func testMgr() *Manager {
	return &Manager{Name: "Syncidian", Email: "syncidian@local"}
}

func headNames(t *testing.T, dir string) map[string]struct{} {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]struct{}{}
	if err := tree.Files().ForEach(func(f *object.File) error {
		out[f.Name] = struct{}{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func writeRel(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCommitAllGitOperations(t *testing.T) {
	dir := t.TempDir()
	m := testMgr()

	writeRel(t, dir, "Inbox/Hello.md", "# hello\n")
	if _, err := m.CommitAll(dir, "add"); err != nil {
		t.Fatal(err)
	}
	names := headNames(t, dir)
	if _, ok := names["Inbox/Hello.md"]; !ok {
		t.Fatalf("add missing: %v", names)
	}

	writeRel(t, dir, "Inbox/Hello.md", "# hello world\n")
	if _, err := m.CommitAll(dir, "modify"); err != nil {
		t.Fatal(err)
	}

	writeRel(t, dir, "Projects/a.md", "a\n")
	writeRel(t, dir, "Projects/sub/b.md", "b\n")
	if _, err := m.CommitAll(dir, "add folder"); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(dir, "Inbox", "Hello.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CommitAll(dir, "delete"); err != nil {
		t.Fatal(err)
	}
	names = headNames(t, dir)
	if _, ok := names["Inbox/Hello.md"]; ok {
		t.Fatalf("delete left file in git: %v", names)
	}

	old := filepath.Join(dir, "Projects")
	newDir := filepath.Join(dir, "Archive")
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(old, filepath.Join(newDir, "Projects")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CommitAll(dir, "move folder"); err != nil {
		t.Fatal(err)
	}
	names = headNames(t, dir)
	if _, ok := names["Projects/a.md"]; ok {
		t.Fatalf("folder move left old path: %v", names)
	}
	if _, ok := names["Archive/Projects/a.md"]; !ok {
		t.Fatalf("folder move missing new path: %v", names)
	}
	if _, ok := names["Archive/Projects/sub/b.md"]; !ok {
		t.Fatalf("nested file missing after folder move: %v", names)
	}

	from := filepath.Join(dir, "Archive", "Projects", "a.md")
	to := filepath.Join(dir, "Archive", "Projects", "renamed.md")
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CommitAll(dir, "rename file"); err != nil {
		t.Fatal(err)
	}
	names = headNames(t, dir)
	if _, ok := names["Archive/Projects/a.md"]; ok {
		t.Fatalf("file rename left old path: %v", names)
	}
	if _, ok := names["Archive/Projects/renamed.md"]; !ok {
		t.Fatalf("file rename missing new path: %v", names)
	}
}
