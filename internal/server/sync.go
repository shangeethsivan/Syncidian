package server

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shangeethsivan/Syncidian/internal/store"
	"github.com/shangeethsivan/Syncidian/internal/syncengine"
)

type planRequest struct {
	DeviceID string `json:"device_id"`
	Files    []struct {
		Path     string `json:"path"`
		Hash     string `json:"hash"`
		BaseHash string `json:"base_hash"`
	} `json:"files"`
}

type pushRequest struct {
	DeviceID string     `json:"device_id"`
	Message  string     `json:"message"`
	Files    []pushFile `json:"files"`
}

type pushFile struct {
	Path     string `json:"path"`
	Hash     string `json:"hash"`
	Mtime    int64  `json:"mtime"`
	Content  string `json:"content"`
	Deleted  bool   `json:"deleted"`
	BaseHash string `json:"base_hash"`
}

func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Platform      string `json:"platform"`
		PluginVersion string `json:"plugin_version"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = "Obsidian"
	}
	if req.ID == "" {
		req.ID = store.NewID()
	}
	d := &store.Device{
		ID:            req.ID,
		UserID:        u.ID,
		Name:          req.Name,
		Platform:      req.Platform,
		PluginVersion: req.PluginVersion,
	}
	if existing, _ := s.Store.GetDevice(d.ID); existing != nil && existing.UserID != u.ID {
		d.ID = store.NewID()
	}
	if err := s.Store.UpsertDevice(d); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = os.MkdirAll(s.Store.VaultDir(u.ID), 0o700)
	_ = s.Store.AddActivity(store.Activity{UserID: u.ID, DeviceID: d.ID, Action: "device.register", Detail: d.Name})
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       d.ID,
		"name":     d.Name,
		"platform": d.Platform,
		"status":   "active",
	})
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request, u *store.User) {
	devices, err := s.Store.ListDevices(u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(devices))
	for _, d := range devices {
		out = append(out, map[string]any{
			"id":             d.ID,
			"name":           d.Name,
			"platform":       d.Platform,
			"plugin_version": d.PluginVersion,
			"last_seen_at":   d.LastSeenAt,
			"last_sync_at":   d.LastSyncAt,
			"sync_count":     d.SyncCount,
			"files_synced":   d.FilesSynced,
			"status":         deviceStatus(d.LastSeenAt),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	d, err := s.Store.GetDevice(id)
	if err != nil || d == nil || d.UserID != u.ID {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	_ = s.Store.UpsertDevice(d)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "active"})
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if err := s.Store.DeleteDevice(u.ID, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request, u *store.User) {
	files, err := s.Store.ListFiles(u.ID, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type item struct {
		Path  string `json:"path"`
		Hash  string `json:"hash"`
		Size  int64  `json:"size"`
		Mtime int64  `json:"mtime"`
	}
	out := make([]item, 0, len(files))
	for _, f := range files {
		out = append(out, item{Path: f.Path, Hash: f.Hash, Size: f.Size, Mtime: f.Mtime})
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out})
}

func (s *Server) handleSyncPlan(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req planRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	serverFiles, err := s.Store.ListFiles(u.ID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	server := map[string]string{}
	for _, f := range serverFiles {
		server[f.Path] = f.Hash
	}
	client := map[string]string{}
	base := map[string]string{}
	for _, f := range req.Files {
		if syncengine.Ignore(f.Path) {
			continue
		}
		client[f.Path] = f.Hash
		if f.BaseHash != "" {
			base[f.Path] = f.BaseHash
		}
	}
	plan := syncengine.PlanSync(client, server, base)
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleSyncFile(w http.ResponseWriter, r *http.Request, u *store.User) {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" || syncengine.Ignore(p) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	meta, err := s.Store.GetFile(u.ID, p)
	if err != nil || meta == nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if meta.Deleted {
		writeJSON(w, http.StatusOK, map[string]any{
			"path":    p,
			"hash":    "",
			"deleted": true,
		})
		return
	}
	full, ok := s.vaultPath(u.ID, p)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	b, err := os.ReadFile(full)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    p,
		"hash":    meta.Hash,
		"mtime":   meta.Mtime,
		"size":    meta.Size,
		"content": base64.StdEncoding.EncodeToString(b),
	})
}

func (s *Server) handleSyncPush(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req pushRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	accepted := make([]string, 0)
	var conflicts []map[string]any
	changed := 0
	for _, f := range req.Files {
		f.Path = strings.TrimPrefix(filepath.ToSlash(f.Path), "/")
		if f.Path == "" || syncengine.Ignore(f.Path) {
			continue
		}
		existing, err := s.Store.GetFile(u.ID, f.Path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		serverHash := ""
		serverExists := existing != nil && !existing.Deleted
		if existing != nil {
			serverHash = existing.Hash
		}
		kind := syncengine.ClassifyPush(f.Hash, serverHash, f.BaseHash, serverExists)
		if f.Deleted {
			if serverExists && f.BaseHash != "" && f.BaseHash != serverHash {
				kind = "conflict"
			} else {
				kind = "accept"
			}
		}
		if kind == "noop" {
			accepted = append(accepted, f.Path)
			continue
		}
		if kind == "conflict" {
			local, _ := s.readVaultFile(u.ID, f.Path)
			incoming, _ := decodeContent(f.Content)
			c := &store.Conflict{
				UserID:        u.ID,
				Path:          f.Path,
				LocalHash:     f.Hash,
				RemoteHash:    serverHash,
				LocalContent:  incoming,
				RemoteContent: local,
				DeviceID:      req.DeviceID,
			}
			if err := s.Store.CreateConflict(c); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			conflicts = append(conflicts, map[string]any{
				"id":          c.ID,
				"path":        f.Path,
				"local_hash":  f.Hash,
				"remote_hash": serverHash,
			})
			continue
		}
		if err := s.applyFile(u.ID, f); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		accepted = append(accepted, f.Path)
		changed++
		s.hub.Broadcast(u.ID, req.DeviceID, map[string]any{
			"type":    "file_changed",
			"path":    f.Path,
			"hash":    f.Hash,
			"deleted": f.Deleted,
		})
	}
	if req.DeviceID != "" {
		_ = s.Store.TouchDevice(req.DeviceID, changed)
	}
	if changed > 0 {
		msg := req.Message
		if msg == "" {
			msg = "Syncidian: vault update"
		}
		commit, gitErr := s.commitAndMaybePush(u.ID, msg)
		_ = s.Store.AddActivity(store.Activity{
			UserID:   u.ID,
			DeviceID: req.DeviceID,
			Action:   "sync.push",
			Detail:   msg,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"accepted":  accepted,
			"conflicts": conflicts,
			"commit":    commit,
			"git_error": gitErr,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted":  accepted,
		"conflicts": conflicts,
	})
}

func (s *Server) applyFile(userID string, f pushFile) error {
	full, ok := s.vaultPath(userID, f.Path)
	if !ok {
		return errInvalidPath
	}
	if f.Deleted {
		_ = os.Remove(full)
		return s.Store.UpsertFile(store.FileMeta{
			UserID:  userID,
			Path:    f.Path,
			Hash:    "",
			Deleted: true,
			Mtime:   f.Mtime,
		})
	}
	b, err := decodeContent(f.Content)
	if err != nil {
		return err
	}
	if f.Hash == "" {
		f.Hash = fileSHA256(b)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(full, b, 0o600); err != nil {
		return err
	}
	return s.Store.UpsertFile(store.FileMeta{
		UserID: userID,
		Path:   f.Path,
		Hash:   f.Hash,
		Size:   int64(len(b)),
		Mtime:  f.Mtime,
	})
}

func decodeContent(s string) ([]byte, error) {
	if s == "" {
		return []byte{}, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

var errInvalidPath = os.ErrInvalid

func (s *Server) vaultPath(userID, rel string) (string, bool) {
	root := s.Store.VaultDir(userID)
	rel = filepath.ToSlash(rel)
	full, ok := safeJoin(filepath.ToSlash(root), rel)
	if !ok {
		return "", false
	}
	return filepath.FromSlash(full), true
}

func (s *Server) readVaultFile(userID, rel string) ([]byte, error) {
	full, ok := s.vaultPath(userID, rel)
	if !ok {
		return nil, errInvalidPath
	}
	return os.ReadFile(full)
}

func (s *Server) handleListConflicts(w http.ResponseWriter, r *http.Request, u *store.User) {
	open := r.URL.Query().Get("open") != "0"
	items, err := s.Store.ListConflicts(u.ID, open)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []store.Conflict{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleGetConflict(w http.ResponseWriter, r *http.Request, u *store.User) {
	c, err := s.Store.GetConflict(r.PathValue("id"))
	if err != nil || c == nil || c.UserID != u.ID {
		writeError(w, http.StatusNotFound, "conflict not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             c.ID,
		"path":           c.Path,
		"local_hash":     c.LocalHash,
		"remote_hash":    c.RemoteHash,
		"local_content":  string(c.LocalContent),
		"remote_content": string(c.RemoteContent),
		"created_at":     c.CreatedAt,
		"resolved_at":    c.ResolvedAt,
		"resolution":     c.Resolution,
	})
}

func (s *Server) handleResolveConflict(w http.ResponseWriter, r *http.Request, u *store.User) {
	c, err := s.Store.GetConflict(r.PathValue("id"))
	if err != nil || c == nil || c.UserID != u.ID {
		writeError(w, http.StatusNotFound, "conflict not found")
		return
	}
	var req struct {
		Resolution string `json:"resolution"` // local | remote | merged
		Content    string `json:"content"`
		DeviceID   string `json:"device_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var body []byte
	switch req.Resolution {
	case "local":
		body = c.LocalContent
	case "remote":
		body = c.RemoteContent
	case "merged":
		body = []byte(req.Content)
	default:
		writeError(w, http.StatusBadRequest, "resolution must be local, remote, or merged")
		return
	}
	hash := fileSHA256(body)
	err = s.applyFile(u.ID, pushFile{
		Path:    c.Path,
		Hash:    hash,
		Mtime:   time.Now().Unix(),
		Content: base64.StdEncoding.EncodeToString(body),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.ResolveConflict(c.ID, req.Resolution)
	_, _ = s.commitAndMaybePush(u.ID, "Syncidian: resolve conflict "+c.Path)
	s.hub.Broadcast(u.ID, req.DeviceID, map[string]any{
		"type":    "file_changed",
		"path":    c.Path,
		"hash":    hash,
		"deleted": false,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "hash": hash, "path": c.Path})
}

func (s *Server) gitLock(userID string) *sync.Mutex {
	v, _ := s.gitMu.LoadOrStore(userID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func (s *Server) commitAndMaybePush(userID, message string) (commit string, gitErr string) {
	mu := s.gitLock(userID)
	mu.Lock()
	defer mu.Unlock()
	dir := s.Store.VaultDir(userID)
	hash, err := s.Git.CommitAll(dir, message)
	if err != nil {
		return "", err.Error()
	}
	cfg, _ := s.Store.GetGitHub(userID)
	if cfg == nil || cfg.Token == "" || cfg.Repo == "" {
		return hash, ""
	}
	if err := s.Git.Push(dir, cfg.Token, cfg.Branch); err != nil {
		_ = s.Store.UpdateGitHubStatus(userID, false, false, err.Error())
		return hash, err.Error()
	}
	_ = s.Store.UpdateGitHubStatus(userID, true, false, "")
	return hash, ""
}
