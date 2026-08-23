package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/shangeethsivan/Syncidian/internal/store"
	"github.com/shangeethsivan/Syncidian/internal/syncengine"
)

type Server struct {
	Store *store.Store
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) Handle(user *store.User, body []byte) ([]byte, error) {
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return marshal(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
	}
	var id any
	if len(req.ID) > 0 {
		_ = json.Unmarshal(req.ID, &id)
	}
	result, err := s.dispatch(user, req.Method, req.Params)
	if err != nil {
		return marshal(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32000, Message: err.Error()}})
	}
	return marshal(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func marshal(v rpcResponse) ([]byte, error) {
	return json.Marshal(v)
}

func (s *Server) dispatch(user *store.User, method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "syncidian",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}, nil
	case "notifications/initialized", "initialized":
		return map[string]any{}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.tools(user)}, nil
	case "tools/call":
		return s.callTool(user, params)
	default:
		return nil, fmt.Errorf("unknown method %s", method)
	}
}

func (s *Server) tools(user *store.User) []map[string]any {
	perms, _ := s.Store.GetMCP(user.ID)
	if perms == nil {
		perms = &store.MCPPermissions{Search: true, Read: true}
	}
	var tools []map[string]any
	if perms.Search {
		tools = append(tools, tool("search_notes", "Search vault notes by text query.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Case-insensitive substring to find"},
			},
			"required": []string{"query"},
		}))
		tools = append(tools, tool("list_notes", "List markdown notes in the vault.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prefix": map[string]any{"type": "string", "description": "Optional path prefix filter"},
			},
		}))
	}
	if perms.Read {
		tools = append(tools, tool("read_note", "Read a note by vault-relative path.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Note path, e.g. Projects/Ideas.md"},
			},
			"required": []string{"path"},
		}))
	}
	if perms.Create {
		tools = append(tools, tool("create_note", "Create a new markdown note.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
			},
			"required": []string{"path", "content"},
		}))
	}
	if perms.Modify {
		tools = append(tools, tool("update_note", "Overwrite an existing note.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
			},
			"required": []string{"path", "content"},
		}))
	}
	return tools
}

func tool(name, desc string, schema map[string]any) map[string]any {
	return map[string]any{
		"name":        name,
		"description": desc,
		"inputSchema": schema,
	}
}

func (s *Server) callTool(user *store.User, params json.RawMessage) (any, error) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params")
	}
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}
	perms, _ := s.Store.GetMCP(user.ID)
	if perms == nil {
		perms = &store.MCPPermissions{Search: true, Read: true}
	}
	switch p.Name {
	case "list_notes":
		if !perms.Search {
			return nil, fmt.Errorf("permission denied")
		}
		prefix, _ := p.Arguments["prefix"].(string)
		return s.listNotes(user.ID, prefix)
	case "search_notes":
		if !perms.Search {
			return nil, fmt.Errorf("permission denied")
		}
		q, _ := p.Arguments["query"].(string)
		return s.searchNotes(user.ID, q)
	case "read_note":
		if !perms.Read {
			return nil, fmt.Errorf("permission denied")
		}
		path, _ := p.Arguments["path"].(string)
		return s.readNote(user.ID, path)
	case "create_note":
		if !perms.Create {
			return nil, fmt.Errorf("permission denied")
		}
		return s.writeNote(user.ID, str(p.Arguments["path"]), str(p.Arguments["content"]), false)
	case "update_note":
		if !perms.Modify {
			return nil, fmt.Errorf("permission denied")
		}
		return s.writeNote(user.ID, str(p.Arguments["path"]), str(p.Arguments["content"]), true)
	default:
		return nil, fmt.Errorf("unknown tool %s", p.Name)
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func textResult(text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}
}

func (s *Server) listNotes(userID, prefix string) (any, error) {
	files, err := s.Store.ListFiles(userID, false)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, f := range files {
		if !strings.HasSuffix(strings.ToLower(f.Path), ".md") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(f.Path, prefix) {
			continue
		}
		lines = append(lines, f.Path)
	}
	if len(lines) == 0 {
		return textResult("(no notes)"), nil
	}
	return textResult(strings.Join(lines, "\n")), nil
}

func (s *Server) searchNotes(userID, query string) (any, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return textResult("query is required"), nil
	}
	files, err := s.Store.ListFiles(userID, false)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var hits []string
	root := s.Store.VaultDir(userID)
	for _, f := range files {
		if !strings.HasSuffix(strings.ToLower(f.Path), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path)))
		if err != nil || !utf8.Valid(b) {
			continue
		}
		body := string(b)
		if strings.Contains(strings.ToLower(f.Path), q) || strings.Contains(strings.ToLower(body), q) {
			hits = append(hits, f.Path)
		}
		if len(hits) >= 50 {
			break
		}
	}
	if len(hits) == 0 {
		return textResult("No matches."), nil
	}
	return textResult(strings.Join(hits, "\n")), nil
}

func (s *Server) readNote(userID, path string) (any, error) {
	path = strings.TrimPrefix(filepath.ToSlash(path), "/")
	if syncengine.Ignore(path) {
		return nil, fmt.Errorf("path is ignored")
	}
	b, err := os.ReadFile(filepath.Join(s.Store.VaultDir(userID), filepath.FromSlash(path)))
	if err != nil {
		return nil, fmt.Errorf("note not found")
	}
	return textResult(string(b)), nil
}

func (s *Server) writeNote(userID, path, content string, mustExist bool) (any, error) {
	path = strings.TrimPrefix(filepath.ToSlash(path), "/")
	if path == "" || syncengine.Ignore(path) {
		return nil, fmt.Errorf("invalid path")
	}
	if !strings.HasSuffix(strings.ToLower(path), ".md") {
		path += ".md"
	}
	full := filepath.Join(s.Store.VaultDir(userID), filepath.FromSlash(path))
	if mustExist {
		if _, err := os.Stat(full); err != nil {
			return nil, fmt.Errorf("note not found")
		}
	} else if _, err := os.Stat(full); err == nil {
		return nil, fmt.Errorf("note already exists")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		return nil, err
	}
	return textResult("Wrote " + path), nil
}
