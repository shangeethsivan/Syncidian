package mcp

import (
	"path"
	"strings"

	"github.com/shangeethsivan/Syncidian/internal/syncengine"
)

// vaultRel normalizes a vault-relative path and rejects escapes / ignored paths.
func vaultRel(rel string) (string, bool) {
	rel = strings.ReplaceAll(rel, "\\", "/")
	// Reject any ".." segment before Clean so agents cannot smuggle traversal.
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return "", false
		}
	}
	rel = path.Clean("/" + rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	if syncengine.Ignore(rel) {
		return "", false
	}
	return rel, true
}

// vaultJoin joins root and a vault-relative path safely.
func vaultJoin(root, rel string) (string, bool) {
	rel, ok := vaultRel(rel)
	if !ok {
		return "", false
	}
	root = path.Clean(strings.ReplaceAll(root, "\\", "/"))
	full := path.Clean(root + "/" + rel)
	if full != root && !strings.HasPrefix(full, root+"/") {
		return "", false
	}
	return full, true
}

func ensureMD(p string) string {
	if !strings.HasSuffix(strings.ToLower(p), ".md") {
		return p + ".md"
	}
	return p
}

func noteStem(p string) string {
	p = path.Base(p)
	if strings.HasSuffix(strings.ToLower(p), ".md") {
		return p[:len(p)-3]
	}
	return p
}
