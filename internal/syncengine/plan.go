package syncengine

import "path"

// Ignore reports whether a vault-relative path should be excluded from sync.
func Ignore(p string) bool {
	p = path.Clean("/" + p)
	p = p[1:]
	if p == "" || p == "." {
		return true
	}
	base := path.Base(p)
	if base == ".DS_Store" || base == "desktop.ini" || base == "Thumbs.db" {
		return true
	}
	if p == ".obsidian/workspace.json" || p == ".obsidian/workspace-mobile.json" {
		return true
	}
	if hasPrefix(p, ".obsidian/workspace-") {
		return true
	}
	if hasPrefix(p, ".obsidian/plugins/syncidian/") || p == ".obsidian/plugins/syncidian" {
		return true
	}
	if hasPrefix(p, ".trash/") || p == ".trash" {
		return true
	}
	if hasPrefix(p, ".git/") || p == ".git" {
		return true
	}
	return false
}

func hasPrefix(p, prefix string) bool {
	if p == prefix {
		return true
	}
	if len(p) > len(prefix) && p[:len(prefix)] == prefix {
		return true
	}
	return false
}

type PlanFile struct {
	Path string
	Hash string
}

type Plan struct {
	Pull      []string `json:"Pull"`
	Push      []string `json:"Push"`
	Delete    []string `json:"Delete"`
	Conflicts []string `json:"Conflicts"`
}

// PlanSync compares a client's file hashes against the server source of truth.
// base is the client's last-known server hash per path (may be empty).
// An empty client hash means the path is gone locally (tombstone). An empty
// server hash is a server-side tombstone from a previous delete.
func PlanSync(client, server, base map[string]string) Plan {
	plan := Plan{Pull: []string{}, Push: []string{}, Delete: []string{}, Conflicts: []string{}}
	seen := map[string]struct{}{}
	for p, ch := range client {
		seen[p] = struct{}{}
		sh, onServer := server[p]
		clientGone := ch == ""
		serverGone := !onServer || sh == ""

		if clientGone && serverGone {
			continue
		}
		if clientGone {
			bh := base[p]
			if !onServer || bh == sh || bh == "" {
				plan.Delete = append(plan.Delete, p)
				continue
			}
			plan.Conflicts = append(plan.Conflicts, p)
			continue
		}
		if !onServer {
			plan.Push = append(plan.Push, p)
			continue
		}
		if sh == "" {
			// Server tombstone: pull the delete if this device still has the last synced bytes.
			bh := base[p]
			if bh != "" && bh == ch {
				plan.Pull = append(plan.Pull, p)
				continue
			}
			plan.Push = append(plan.Push, p)
			continue
		}
		if ch == sh {
			continue
		}
		bh := base[p]
		if bh == sh {
			plan.Push = append(plan.Push, p)
			continue
		}
		if bh == ch {
			plan.Pull = append(plan.Pull, p)
			continue
		}
		plan.Conflicts = append(plan.Conflicts, p)
	}
	for p, sh := range server {
		if _, ok := seen[p]; ok {
			continue
		}
		if sh == "" {
			continue
		}
		plan.Pull = append(plan.Pull, p)
	}
	return plan
}

// ClassifyPush decides whether an incoming client file should be accepted,
// treated as a no-op, or raised as a conflict.
func ClassifyPush(clientHash, serverHash, baseHash string, serverExists bool) string {
	if !serverExists || serverHash == "" {
		return "accept"
	}
	if clientHash == serverHash {
		return "noop"
	}
	if baseHash == serverHash || baseHash == "" {
		// Empty base on first push of a known file is a conflict if hashes differ
		// unless the client never knew about the file — treat empty+mismatch as conflict
		// only when base is empty AND client is overwriting existing server content.
		if baseHash == "" && clientHash != serverHash {
			return "conflict"
		}
		return "accept"
	}
	if baseHash == clientHash {
		return "noop"
	}
	return "conflict"
}
