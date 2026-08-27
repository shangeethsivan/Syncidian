package server

import (
	"strings"

	"github.com/shangeethsivan/Syncidian/internal/githubapp"
	"github.com/shangeethsivan/Syncidian/internal/mcp"
)

// githubNotes stores MCP note bodies in the user's GitHub repo. Nothing is
// written to the server working copy.
type githubNotes struct {
	s *Server
}

func (g *githubNotes) creds(userID string) (token, repo string, err error) {
	cfg, err := g.s.Store.GetGitHub(userID)
	if err != nil {
		return "", "", err
	}
	if !cfg.Configured() {
		return "", "", mcp.ErrNoGitHub
	}
	token, err = g.s.gitAccessToken(cfg)
	if err != nil {
		return "", "", err
	}
	return token, cfg.Repo, nil
}

func (g *githubNotes) Get(userID, path string) ([]byte, error) {
	token, repo, err := g.creds(userID)
	if err != nil {
		return nil, err
	}
	f, err := githubapp.GetFile(token, repo, path, GitHubBranch)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, mcp.ErrNoteNotFound
		}
		return nil, err
	}
	return f.Content, nil
}

func (g *githubNotes) Put(userID, path string, data []byte, mustExist bool) error {
	token, repo, err := g.creds(userID)
	if err != nil {
		return err
	}
	branch := GitHubBranch
	existing, getErr := githubapp.GetFile(token, repo, path, branch)
	if getErr != nil && !strings.Contains(strings.ToLower(getErr.Error()), "not found") {
		return getErr
	}
	exists := getErr == nil
	if mustExist && !exists {
		return mcp.ErrNoteNotFound
	}
	if !mustExist && exists {
		return mcp.ErrNoteExists
	}
	sha := ""
	msg := "Syncidian: create " + path
	if exists {
		sha = existing.SHA
		msg = "Syncidian: update " + path
	}
	_, err = githubapp.PutFile(token, repo, path, msg, data, sha, branch)
	return err
}

func (g *githubNotes) Delete(userID, path string) error {
	token, repo, err := g.creds(userID)
	if err != nil {
		return err
	}
	existing, err := githubapp.GetFile(token, repo, path, GitHubBranch)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil
		}
		return err
	}
	return githubapp.DeleteFile(token, repo, path, "Syncidian: delete "+path, existing.SHA, GitHubBranch)
}

func (g *githubNotes) List(userID string) ([]string, error) {
	token, repo, err := g.creds(userID)
	if err != nil {
		return nil, err
	}
	return githubapp.ListFiles(token, repo, GitHubBranch)
}

func (s *Server) fetchNoteFromGitHub(userID, path string) ([]byte, error) {
	n := &githubNotes{s: s}
	return n.Get(userID, path)
}
