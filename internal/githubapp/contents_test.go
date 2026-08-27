package githubapp

import (
	"strings"
	"testing"
)

func TestContentsURLKeepsSlashes(t *testing.T) {
	u := contentsURL("owner/repo", "Ideas/Widget.md", "main")
	if !strings.Contains(u, "/contents/Ideas/Widget.md") {
		t.Fatalf("path slashes: %s", u)
	}
	if !strings.Contains(u, "ref=main") {
		t.Fatalf("ref: %s", u)
	}
	u = contentsURL("owner/repo", "Weekly Focus.md", "")
	if !strings.Contains(u, "Weekly%20Focus.md") {
		t.Fatalf("space encoding: %s", u)
	}
}
