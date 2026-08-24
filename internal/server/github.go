package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shangeethsivan/Syncidian/internal/githubapp"
	"github.com/shangeethsivan/Syncidian/internal/store"
	"github.com/shangeethsivan/Syncidian/internal/syncengine"
)

// GitHubBranch is the only branch Syncidian syncs. Other branches are ignored
// so the dashboard can stay locked to a single, simple backup target.
const GitHubBranch = "main"

func (s *Server) handleGetGitHub(w http.ResponseWriter, r *http.Request, u *store.User) {
	cfg, err := s.Store.GetGitHub(u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := map[string]any{
		"configured":   false,
		"branch":       GitHubBranch,
		"instance_app": false,
	}
	if inst := s.instanceGitHubApp(); inst.Configured() {
		out["instance_app"] = true
		if inst.Slug != "" {
			out["install_url"] = "https://github.com/apps/" + inst.Slug + "/installations/new"
		}
	}
	if cfg == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["repo"] = cfg.Repo
	out["app_slug"] = cfg.AppSlug
	out["has_app"] = cfg.HasApp()
	out["installed"] = cfg.InstallationID != 0
	out["last_push"] = cfg.LastPush
	out["last_pull"] = cfg.LastPull
	out["last_error"] = cfg.LastError
	if _, ok := out["install_url"]; !ok && cfg.AppSlug != "" {
		out["install_url"] = "https://github.com/apps/" + cfg.AppSlug + "/installations/new"
	}
	if cfg.Configured() {
		out["configured"] = true
		writeJSON(w, http.StatusOK, out)
		return
	}
	if cfg.HasApp() && cfg.InstallationID != 0 {
		repos, listErr := s.listInstallRepos(cfg)
		if listErr != nil {
			out["last_error"] = listErr.Error()
		} else {
			names := make([]string, 0, len(repos))
			for _, repo := range repos {
				names = append(names, repo.FullName)
			}
			out["repos"] = names
			out["needs_repo"] = true
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetGitHub binds one repository from an existing GitHub App installation.
// Personal access tokens are not accepted.
func (s *Server) handleSetGitHub(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Token string `json:"token"`
		Repo  string `json:"repo"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Token) != "" {
		writeError(w, http.StatusBadRequest, "Personal access tokens are not supported. Connect with the GitHub App.")
		return
	}
	cfg, err := s.Store.GetGitHub(u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !cfg.HasApp() || cfg.InstallationID == 0 {
		writeError(w, http.StatusBadRequest, "Connect with GitHub first, then select a repository. Personal access tokens are not supported.")
		return
	}
	if err := s.bindGitHubRepo(u, cfg, req.Repo); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.Store.AddActivity(store.Activity{UserID: u.ID, Action: "github.configure", Detail: cfg.Repo})
	_, gitErr := s.commitAndMaybePush(u.ID, "Syncidian: connect GitHub")
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"repo":       cfg.Repo,
		"branch":     GitHubBranch,
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
	if err != nil || !cfg.Configured() {
		writeError(w, http.StatusBadRequest, "GitHub is not configured")
		return
	}
	token, err := s.gitAccessToken(cfg)
	if err != nil {
		_ = s.Store.UpdateGitHubStatus(u.ID, false, false, err.Error())
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	dir := s.Store.VaultDir(u.ID)
	if err := s.Git.Pull(dir, token, GitHubBranch); err != nil {
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

func (s *Server) gitAccessToken(cfg *store.GitHubConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("GitHub App is not connected")
	}
	if inst := s.instanceGitHubApp(); inst.Configured() && cfg.AppPEM == "" {
		cfg.AppID = inst.AppID
		cfg.AppPEM = inst.PEM
		cfg.AppSlug = inst.Slug
		cfg.ClientID = inst.ClientID
		cfg.ClientSecret = inst.ClientSecret
	}
	if !cfg.HasApp() || cfg.InstallationID == 0 {
		return "", fmt.Errorf("GitHub App is not connected")
	}
	if cfg.Token != "" && time.Now().Add(2*time.Minute).Before(cfg.InstallTokenExpires) {
		return cfg.Token, nil
	}
	tok, exp, err := githubapp.InstallationToken(cfg.AppID, []byte(cfg.AppPEM), cfg.InstallationID)
	if err != nil {
		return "", err
	}
	cfg.Token = tok
	cfg.InstallTokenExpires = exp
	_ = s.Store.UpdateGitHubInstallToken(cfg.UserID, tok, exp)
	return tok, nil
}

func (s *Server) listInstallRepos(cfg *store.GitHubConfig) ([]githubapp.Repo, error) {
	token, err := s.gitAccessToken(cfg)
	if err != nil {
		return nil, err
	}
	return githubapp.ListRepos(token)
}

func (s *Server) bindGitHubRepo(u *store.User, cfg *store.GitHubConfig, raw string) error {
	repo := normalizeGitHubRepo(raw)
	if repo == "" || !strings.Contains(repo, "/") {
		return fmt.Errorf("repo must be owner/name")
	}
	repos, err := s.listInstallRepos(cfg)
	if err != nil {
		return err
	}
	allowed := false
	for _, item := range repos {
		if strings.EqualFold(item.FullName, repo) {
			repo = item.FullName
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("repository %s is not in this GitHub App installation; install the app on that repository", repo)
	}
	token, err := s.gitAccessToken(cfg)
	if err != nil {
		return err
	}
	if err := s.Git.CloneOrOpen(s.Store.VaultDir(u.ID), repo, token, GitHubBranch); err != nil {
		return fmt.Errorf("could not open repository: %w", err)
	}
	cfg.Repo = repo
	cfg.Branch = GitHubBranch
	cfg.UserID = u.ID
	return s.Store.SetGitHub(*cfg)
}

func normalizeGitHubRepo(repo string) string {
	repo = strings.TrimSpace(repo)
	repo = strings.TrimPrefix(repo, "https://github.com/")
	repo = strings.TrimPrefix(repo, "http://github.com/")
	repo = strings.TrimPrefix(repo, "git@github.com:")
	repo = strings.TrimSuffix(repo, ".git")
	return strings.Trim(repo, "/")
}
