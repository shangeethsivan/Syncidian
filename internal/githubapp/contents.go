package githubapp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// File is a repository file from the GitHub Contents API.
type File struct {
	Path    string
	Content []byte
	SHA     string
}

type contentsResp struct {
	Type     string `json:"type"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
	SHA      string `json:"sha"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

func contentsURL(repo, path, ref string) string {
	u := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", repo, url.PathEscape(strings.TrimPrefix(path, "/")))
	// PathEscape encodes slashes; GitHub wants slashes in the path. Revert %2F.
	u = strings.ReplaceAll(u, "%2F", "/")
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	return u
}

func GetFile(token, repo, path, ref string) (*File, error) {
	if ref == "" {
		ref = "main"
	}
	body, err := do(http.MethodGet, contentsURL(repo, path, ref), "Bearer "+token, nil)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("not found")
		}
		return nil, err
	}
	var out contentsResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Type != "" && out.Type != "file" {
		return nil, fmt.Errorf("not a file")
	}
	raw := strings.ReplaceAll(out.Content, "\n", "")
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode content: %w", err)
	}
	return &File{Path: path, Content: data, SHA: out.SHA}, nil
}

func PutFile(token, repo, path, message string, content []byte, sha, branch string) (string, error) {
	if branch == "" {
		branch = "main"
	}
	if message == "" {
		message = "Syncidian: MCP"
	}
	payload := map[string]any{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  branch,
	}
	if sha != "" {
		payload["sha"] = sha
	}
	body, err := do(http.MethodPut, contentsURL(repo, path, ""), "Bearer "+token, payload)
	if err != nil {
		return "", err
	}
	var out struct {
		Content struct {
			SHA string `json:"sha"`
		} `json:"content"`
	}
	_ = json.Unmarshal(body, &out)
	return out.Content.SHA, nil
}

func DeleteFile(token, repo, path, message, sha, branch string) error {
	if branch == "" {
		branch = "main"
	}
	if message == "" {
		message = "Syncidian: MCP delete"
	}
	if sha == "" {
		return fmt.Errorf("sha is required to delete")
	}
	payload := map[string]any{
		"message": message,
		"sha":     sha,
		"branch":  branch,
	}
	_, err := do(http.MethodDelete, contentsURL(repo, path, ""), "Bearer "+token, payload)
	return err
}

type TreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

func ListFiles(token, repo, ref string) ([]string, error) {
	if ref == "" {
		ref = "main"
	}
	u := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s?recursive=1", repo, url.PathEscape(ref))
	body, err := do(http.MethodGet, u, "Bearer "+token, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Tree      []TreeEntry `json:"tree"`
		Truncated bool        `json:"truncated"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range out.Tree {
		if e.Type != "blob" || e.Path == "" {
			continue
		}
		paths = append(paths, e.Path)
	}
	return paths, nil
}
