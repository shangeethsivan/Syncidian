package syncengine

import (
	"reflect"
	"sort"
	"testing"
)

func TestIgnore(t *testing.T) {
	cases := map[string]bool{
		"Notes/Hello.md":                      false,
		".obsidian/app.json":                  false,
		".obsidian/workspace.json":            true,
		".obsidian/workspace-mobile.json":     true,
		".obsidian/plugins/syncidian/main.js": true,
		".trash/gone.md":                      true,
		".git/HEAD":                           true,
		".DS_Store":                           true,
	}
	for p, want := range cases {
		if got := Ignore(p); got != want {
			t.Errorf("Ignore(%q)=%v want %v", p, got, want)
		}
	}
}

func TestPlanSync(t *testing.T) {
	client := map[string]string{"a.md": "1", "b.md": "2", "c.md": "x"}
	server := map[string]string{"a.md": "1", "c.md": "3", "d.md": "4"}
	base := map[string]string{"a.md": "1", "c.md": "1"}
	got := PlanSync(client, server, base)
	sort.Strings(got.Push)
	sort.Strings(got.Pull)
	sort.Strings(got.Conflicts)
	wantPush := []string{"b.md"}
	wantPull := []string{"d.md"}
	wantConflicts := []string{"c.md"}
	if !reflect.DeepEqual(got.Push, wantPush) {
		t.Errorf("push %v want %v", got.Push, wantPush)
	}
	if !reflect.DeepEqual(got.Pull, wantPull) {
		t.Errorf("pull %v want %v", got.Pull, wantPull)
	}
	if !reflect.DeepEqual(got.Conflicts, wantConflicts) {
		t.Errorf("conflicts %v want %v", got.Conflicts, wantConflicts)
	}
	if len(got.Delete) != 0 {
		t.Errorf("delete %v want none", got.Delete)
	}
}

func TestPlanSyncLocalDelete(t *testing.T) {
	client := map[string]string{
		"keep.md":     "1",
		"gone.md":     "",
		"folder/a.md": "",
	}
	server := map[string]string{
		"keep.md":     "1",
		"gone.md":     "abc",
		"folder/a.md": "aaa",
		"folder/b.md": "bbb",
	}
	base := map[string]string{
		"gone.md":     "abc",
		"folder/a.md": "aaa",
	}
	got := PlanSync(client, server, base)
	sort.Strings(got.Push)
	sort.Strings(got.Pull)
	sort.Strings(got.Delete)
	sort.Strings(got.Conflicts)
	if len(got.Push) != 0 {
		t.Errorf("push %v want none", got.Push)
	}
	if !reflect.DeepEqual(got.Delete, []string{"folder/a.md", "gone.md"}) {
		t.Errorf("delete %v", got.Delete)
	}
	if !reflect.DeepEqual(got.Pull, []string{"folder/b.md"}) {
		t.Errorf("pull %v want [folder/b.md] (no tombstone sent)", got.Pull)
	}
	if len(got.Conflicts) != 0 {
		t.Errorf("conflicts %v", got.Conflicts)
	}
}

func TestPlanSyncDeleteVsRemoteEdit(t *testing.T) {
	client := map[string]string{"note.md": ""}
	server := map[string]string{"note.md": "new"}
	base := map[string]string{"note.md": "old"}
	got := PlanSync(client, server, base)
	if len(got.Conflicts) != 1 || got.Conflicts[0] != "note.md" {
		t.Fatalf("want conflict, got %+v", got)
	}
}

func TestPlanSyncServerTombstone(t *testing.T) {
	client := map[string]string{"alive.md": "1"}
	server := map[string]string{"alive.md": "1", "dead.md": ""}
	got := PlanSync(client, server, nil)
	if len(got.Pull) != 0 || len(got.Delete) != 0 || len(got.Push) != 0 {
		t.Fatalf("tombstone should be ignored: %+v", got)
	}
}

func TestClassifyPush(t *testing.T) {
	if got := ClassifyPush("aaa", "", "", false); got != "accept" {
		t.Fatalf("new file: %s", got)
	}
	if got := ClassifyPush("aaa", "aaa", "aaa", true); got != "noop" {
		t.Fatalf("unchanged: %s", got)
	}
	if got := ClassifyPush("bbb", "aaa", "aaa", true); got != "accept" {
		t.Fatalf("fast-forward: %s", got)
	}
	if got := ClassifyPush("bbb", "ccc", "aaa", true); got != "conflict" {
		t.Fatalf("diverged: %s", got)
	}
}
