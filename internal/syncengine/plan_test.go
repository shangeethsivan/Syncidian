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
