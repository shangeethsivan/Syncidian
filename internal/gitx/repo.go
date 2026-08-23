package gitx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	gitcfg "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

type Manager struct {
	Name  string
	Email string
}

func (m *Manager) Ensure(dir string) (*git.Repository, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	repo, err := git.PlainOpen(dir)
	if err == nil {
		return repo, nil
	}
	return git.PlainInit(dir, false)
}

func (m *Manager) auth(token string) *githttp.BasicAuth {
	return &githttp.BasicAuth{Username: "x-access-token", Password: token}
}

func repoURL(ownerRepo string) string {
	ownerRepo = strings.TrimSpace(ownerRepo)
	ownerRepo = strings.TrimPrefix(ownerRepo, "https://github.com/")
	ownerRepo = strings.TrimPrefix(ownerRepo, "git@github.com:")
	ownerRepo = strings.TrimSuffix(ownerRepo, ".git")
	return "https://github.com/" + strings.Trim(ownerRepo, "/") + ".git"
}

func (m *Manager) ConfigureOrigin(dir, ownerRepo string) error {
	repo, err := m.Ensure(dir)
	if err != nil {
		return err
	}
	_ = repo.DeleteRemote("origin")
	_, err = repo.CreateRemote(&gitcfg.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL(ownerRepo)},
	})
	return err
}

func (m *Manager) CommitAll(dir, message string) (string, error) {
	repo, err := m.Ensure(dir)
	if err != nil {
		return "", err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return "", err
	}
	status, err := wt.Status()
	if err != nil {
		return "", err
	}
	if status.IsClean() {
		head, err := repo.Head()
		if err != nil {
			return "", nil
		}
		return head.Hash().String(), nil
	}
	if message == "" {
		message = "Syncidian: vault update"
	}
	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  m.Name,
			Email: m.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

func (m *Manager) Push(dir, token, branch string) error {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return err
	}
	if strings.TrimSpace(branch) == "" {
		branch = "main"
	}
	head, err := repo.Head()
	if err == nil && head != nil {
		ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), head.Hash())
		_ = repo.Storer.SetReference(ref)
	}
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       m.auth(token),
	})
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}
	return err
}

func (m *Manager) Pull(dir, token, branch string) error {
	repo, err := m.Ensure(dir)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if strings.TrimSpace(branch) == "" {
		branch = "main"
	}
	err = wt.Pull(&git.PullOptions{
		RemoteName:    "origin",
		Auth:          m.auth(token),
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
	})
	if err == nil || err == git.NoErrAlreadyUpToDate {
		return nil
	}
	return err
}

func (m *Manager) CloneOrOpen(dir, ownerRepo, token, branch string) error {
	url := repoURL(ownerRepo)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return m.ConfigureOrigin(dir, ownerRepo)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	if strings.TrimSpace(branch) == "" {
		branch = "main"
	}
	_, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:           url,
		Auth:          m.auth(token),
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
	})
	if err == nil {
		return nil
	}
	if _, e2 := m.Ensure(dir); e2 != nil {
		return fmt.Errorf("clone: %w", err)
	}
	return m.ConfigureOrigin(dir, ownerRepo)
}

func CreatePrivateRepo(token, ownerRepo, description string) error {
	parts := strings.SplitN(strings.Trim(ownerRepo, "/"), "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("repository must be owner/name")
	}
	payload, _ := json.Marshal(map[string]any{
		"name":        parts[1],
		"private":     true,
		"description": description,
		"auto_init":   false,
	})
	req, err := http.NewRequest(http.MethodPost, "https://api.github.com/user/repos", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusUnprocessableEntity {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var msg struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &msg)
	if msg.Message == "" {
		msg.Message = string(body)
	}
	return fmt.Errorf("github create repo: %s (%s)", resp.Status, msg.Message)
}
