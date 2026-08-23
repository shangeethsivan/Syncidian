package server

import (
	"net/http"
	"strings"

	"github.com/shangeethsivan/Syncidian/internal/gitx"
	"github.com/shangeethsivan/Syncidian/internal/store"
	"github.com/shangeethsivan/Syncidian/internal/syncengine"
)

func (s *Server) handleGetGitHub(w http.ResponseWriter, r *http.Request, u *store.User) {
	cfg, err := s.Store.GetGitHub(u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cfg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"repo":       cfg.Repo,
		"branch":     cfg.Branch,
		"last_push":  cfg.LastPush,
		"last_pull":  cfg.LastPull,
		"last_error": cfg.LastError,
		"token_set":  cfg.Token != "",
	})
}

func (s *Server) handleSetGitHub(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Token      string `json:"token"`
		Repo       string `json:"repo"`
		Branch     string `json:"branch"`
		CreateRepo bool   `json:"create_repo"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Repo = strings.TrimSpace(req.Repo)
	req.Repo = strings.TrimPrefix(req.Repo, "https://github.com/")
	req.Repo = strings.TrimSuffix(req.Repo, ".git")
	if req.Repo == "" || !strings.Contains(req.Repo, "/") {
		writeError(w, http.StatusBadRequest, "repo must be owner/name")
		return
	}
	existing, _ := s.Store.GetGitHub(u.ID)
	if req.Token == "" && existing != nil {
		req.Token = existing.Token
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "GitHub token is required")
		return
	}
	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.CreateRepo {
		if err := gitx.CreatePrivateRepo(req.Token, req.Repo, "Syncidian vault backup"); err != nil {
			s.Log.Warn("create github repo", "err", err)
		}
	}
	dir := s.Store.VaultDir(u.ID)
	if err := s.Git.CloneOrOpen(dir, req.Repo, req.Token, req.Branch); err != nil {
		writeError(w, http.StatusBadRequest, "could not open repository: "+err.Error())
		return
	}
	if err := s.Store.SetGitHub(store.GitHubConfig{
		UserID: u.ID,
		Token:  req.Token,
		Repo:   req.Repo,
		Branch: req.Branch,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.AddActivity(store.Activity{UserID: u.ID, Action: "github.configure", Detail: req.Repo})
	_, gitErr := s.commitAndMaybePush(u.ID, "Syncidian: connect GitHub")
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"repo":       req.Repo,
		"branch":     req.Branch,
		"git_error":  gitErr,
	})
}

func (s *Server) handleDeleteGitHub(w http.ResponseWriter, r *http.Request, u *store.User) {
	if err := s.Store.DeleteGitHub(u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGitHubSyncNow(w http.ResponseWriter, r *http.Request, u *store.User) {
	cfg, err := s.Store.GetGitHub(u.ID)
	if err != nil || cfg == nil {
		writeError(w, http.StatusBadRequest, "GitHub is not configured")
		return
	}
	dir := s.Store.VaultDir(u.ID)
	if err := s.Git.Pull(dir, cfg.Token, cfg.Branch); err != nil {
		_ = s.Store.UpdateGitHubStatus(u.ID, false, false, err.Error())
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.reindexVault(u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.UpdateGitHubStatus(u.ID, false, true, "")
	commit, gitErr := s.commitAndMaybePush(u.ID, "Syncidian: pull from GitHub")
	s.hub.Broadcast(u.ID, "", map[string]any{"type": "github_synced"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "commit": commit, "git_error": gitErr})
}

func (s *Server) reindexVault(userID string) error {
	root := s.Store.VaultDir(userID)
	return walkVault(root, func(rel string, b []byte, mtime int64) error {
		if syncengine.Ignore(rel) {
			return nil
		}
		return s.Store.UpsertFile(store.FileMeta{
			UserID: userID,
			Path:   rel,
			Hash:   fileSHA256(b),
			Size:   int64(len(b)),
			Mtime:  mtime,
		})
	})
}
