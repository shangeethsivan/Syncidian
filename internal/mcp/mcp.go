package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shangeethsivan/Syncidian/internal/store"
)

// OnChange notifies listeners after MCP mutates a vault file.
// content is the new note body (nil when deleted). It is not stored on the server.
type OnChange func(userID, path, hash string, deleted bool, content []byte)

type ClientMeta struct {
	UserAgent   string
	TokenID     string
	TokenName   string
	TokenPrefix string
}

type Server struct {
	Store    *store.Store
	Notes    Notes
	OnChange OnChange
}

func (s *Server) notes() Notes {
	if s.Notes != nil {
		return s.Notes
	}
	return NewMemoryNotes()
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

func (s *Server) Handle(user *store.User, body []byte, meta ClientMeta) ([]byte, error) {
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return marshal(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
	}
	var id any
	if len(req.ID) > 0 {
		_ = json.Unmarshal(req.ID, &id)
	}
	result, err := s.dispatch(user, req.Method, req.Params, meta)
	if err != nil {
		return marshal(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32000, Message: err.Error()}})
	}
	return marshal(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func marshal(v rpcResponse) ([]byte, error) {
	return json.Marshal(v)
}

func (s *Server) notify(userID, path, hash string, deleted bool, content []byte) {
	if s.OnChange != nil {
		s.OnChange(userID, path, hash, deleted, content)
	}
}

func (s *Server) dispatch(user *store.User, method string, params json.RawMessage, meta ClientMeta) (any, error) {
	name, version := clientInfo(params)
	tool := ""
	if method == "tools/call" {
		tool = toolName(params)
	}
	s.record(user, method, tool, name, version, meta)
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "syncidian",
				"version": "0.2.0",
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

func (s *Server) record(user *store.User, method, tool, name, version string, meta ClientMeta) {
	if s.Store == nil || user == nil {
		return
	}
	switch method {
	case "initialize", "tools/list", "tools/call":
	default:
		return
	}
	_ = s.Store.RecordMCPEvent(store.MCPEvent{
		UserID:      user.ID,
		Name:        name,
		Version:     version,
		UserAgent:   meta.UserAgent,
		TokenID:     meta.TokenID,
		TokenName:   meta.TokenName,
		TokenPrefix: meta.TokenPrefix,
		Method:      method,
		Tool:        tool,
	})
}

func clientInfo(params json.RawMessage) (name, version string) {
	if len(params) == 0 {
		return "", ""
	}
	var p struct {
		ClientInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}
	_ = json.Unmarshal(params, &p)
	return strings.TrimSpace(p.ClientInfo.Name), strings.TrimSpace(p.ClientInfo.Version)
}

func toolName(params json.RawMessage) string {
	var p struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(params, &p)
	return strings.TrimSpace(p.Name)
}

func (s *Server) perms(userID string) *store.MCPPermissions {
	perms, _ := s.Store.GetMCP(userID)
	if perms == nil {
		return &store.MCPPermissions{Search: true, Read: true}
	}
	return perms
}

func (s *Server) tools(user *store.User) []map[string]any {
	perms := s.perms(user.ID)
	var tools []map[string]any
	if perms.Search {
		tools = append(tools,
			tool("search_notes", "Search vault notes by text query (path and content).", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Case-insensitive substring to find"},
				},
				"required": []string{"query"},
			}),
			tool("list_notes", "List markdown notes in the vault.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prefix": map[string]any{"type": "string", "description": "Optional path prefix filter"},
				},
			}),
			tool("find_related", "Find notes related to a topic or note path via links and text overlap.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Topic text or note path"},
					"limit": map[string]any{"type": "number", "description": "Max results (default 15)"},
				},
				"required": []string{"query"},
			}),
			tool("suggest_note_path", "Suggest where to store a new idea/note based on existing vault structure and content.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"topic": map[string]any{"type": "string", "description": "Idea or topic to place"},
					"kind":  map[string]any{"type": "string", "description": "Optional kind: idea, project, meeting, note"},
				},
				"required": []string{"topic"},
			}),
		)
	}
	if perms.Read {
		tools = append(tools,
			tool("read_note", "Read a note by vault-relative path.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Note path, e.g. Projects/Ideas.md"},
				},
				"required": []string{"path"},
			}),
			tool("get_outgoing_links", "List Obsidian [[wikilinks]] leaving a note.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			}),
			tool("get_backlinks", "List notes that link to the given note.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			}),
			tool("get_graph", "Return the vault note graph (nodes, edges) plus a Mermaid diagram for visualization.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prefix": map[string]any{"type": "string", "description": "Optional folder prefix to scope the graph"},
					"format": map[string]any{"type": "string", "description": "json (default) or mermaid"},
				},
			}),
		)
	}
	if perms.Create {
		tools = append(tools,
			tool("create_note", "Create a new markdown note.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"content": map[string]any{"type": "string"},
				},
				"required": []string{"path", "content"},
			}),
		)
	}
	if perms.Modify {
		tools = append(tools,
			tool("update_note", "Overwrite an existing note.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"content": map[string]any{"type": "string"},
				},
				"required": []string{"path", "content"},
			}),
			tool("add_backlink", "Append a [[wikilink]] from one note to another.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"from":  map[string]any{"type": "string", "description": "Note that will contain the link"},
					"to":    map[string]any{"type": "string", "description": "Note being linked"},
					"alias": map[string]any{"type": "string", "description": "Optional display alias"},
				},
				"required": []string{"from", "to"},
			}),
			tool("move_note", "Rename or move a note within the vault.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"from": map[string]any{"type": "string"},
					"to":   map[string]any{"type": "string"},
				},
				"required": []string{"from", "to"},
			}),
			tool("delete_note", "Delete a note from the vault.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			}),
			tool("bulk_move", "Move many notes from one folder prefix to another (organize / clean up).", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"from_prefix": map[string]any{"type": "string"},
					"to_prefix":   map[string]any{"type": "string"},
					"paths":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional subset; defaults to all notes under from_prefix"},
				},
				"required": []string{"from_prefix", "to_prefix"},
			}),
			tool("bulk_add_links", "Add a wikilink to the same target note from many source notes.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"link_to": map[string]any{"type": "string"},
					"paths":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"link_to", "paths"},
			}),
		)
	}
	if perms.Create || perms.Modify {
		tools = append(tools, tool("append_to_note", "Append content to a note (creates it if missing and create is allowed).", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
				"heading": map[string]any{"type": "string", "description": "Optional heading to append under"},
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
	perms := s.perms(user.ID)
	switch p.Name {
	case "list_notes":
		if !perms.Search {
			return nil, fmt.Errorf("permission denied")
		}
		return s.listNotes(user.ID, str(p.Arguments["prefix"]))
	case "search_notes":
		if !perms.Search {
			return nil, fmt.Errorf("permission denied")
		}
		return s.searchNotes(user.ID, str(p.Arguments["query"]))
	case "find_related":
		if !perms.Search {
			return nil, fmt.Errorf("permission denied")
		}
		return s.findRelated(user.ID, str(p.Arguments["query"]), intArg(p.Arguments["limit"]))
	case "suggest_note_path":
		if !perms.Search {
			return nil, fmt.Errorf("permission denied")
		}
		return s.suggestNotePath(user.ID, str(p.Arguments["topic"]), str(p.Arguments["kind"]))
	case "read_note":
		if !perms.Read {
			return nil, fmt.Errorf("permission denied")
		}
		return s.readNote(user.ID, str(p.Arguments["path"]))
	case "get_outgoing_links":
		if !perms.Read {
			return nil, fmt.Errorf("permission denied")
		}
		return s.outgoingLinks(user.ID, str(p.Arguments["path"]))
	case "get_backlinks":
		if !perms.Read {
			return nil, fmt.Errorf("permission denied")
		}
		return s.backlinks(user.ID, str(p.Arguments["path"]))
	case "get_graph":
		if !perms.Read {
			return nil, fmt.Errorf("permission denied")
		}
		return s.buildGraph(user.ID, str(p.Arguments["prefix"]), str(p.Arguments["format"]))
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
	case "append_to_note":
		return s.callAppend(user.ID, perms, str(p.Arguments["path"]), str(p.Arguments["content"]), str(p.Arguments["heading"]))
	case "add_backlink":
		if !perms.Modify {
			return nil, fmt.Errorf("permission denied")
		}
		return s.addBacklink(user.ID, str(p.Arguments["from"]), str(p.Arguments["to"]), str(p.Arguments["alias"]))
	case "move_note":
		if !perms.Modify {
			return nil, fmt.Errorf("permission denied")
		}
		return s.moveNote(user.ID, str(p.Arguments["from"]), str(p.Arguments["to"]))
	case "delete_note":
		if !perms.Modify {
			return nil, fmt.Errorf("permission denied")
		}
		return s.deleteNote(user.ID, str(p.Arguments["path"]))
	case "bulk_move":
		if !perms.Modify {
			return nil, fmt.Errorf("permission denied")
		}
		return s.bulkMove(user.ID, str(p.Arguments["from_prefix"]), str(p.Arguments["to_prefix"]), strSlice(p.Arguments["paths"]))
	case "bulk_add_links":
		if !perms.Modify {
			return nil, fmt.Errorf("permission denied")
		}
		return s.bulkAddLinks(user.ID, str(p.Arguments["link_to"]), strSlice(p.Arguments["paths"]))
	default:
		return nil, fmt.Errorf("unknown tool %s", p.Name)
	}
}

func (s *Server) callAppend(userID string, perms *store.MCPPermissions, path, content, heading string) (any, error) {
	path, ok := vaultRel(ensureMD(path))
	if !ok {
		return nil, fmt.Errorf("invalid path")
	}
	exists := s.noteExists(userID, path)
	if exists {
		if !perms.Modify {
			return nil, fmt.Errorf("permission denied")
		}
	} else if !perms.Create {
		return nil, fmt.Errorf("permission denied")
	}
	return s.appendToNote(userID, path, content, heading)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func intArg(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func strSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func textResult(text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}
}

func (s *Server) listNotes(userID, prefix string) (any, error) {
	paths, err := s.notes().List(userID)
	if err != nil {
		return nil, noteErr(err)
	}
	lines := mdPaths(paths, prefix)
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
	paths, err := s.notes().List(userID)
	if err != nil {
		return nil, noteErr(err)
	}
	q := strings.ToLower(query)
	type hit struct {
		Path    string `json:"path"`
		Snippet string `json:"snippet,omitempty"`
	}
	var hits []hit
	for _, p := range mdPaths(paths, "") {
		b, err := s.notes().Get(userID, p)
		if err != nil || !utf8.Valid(b) {
			continue
		}
		body := string(b)
		pathHit := strings.Contains(strings.ToLower(p), q)
		bodyLower := strings.ToLower(body)
		bodyHit := strings.Contains(bodyLower, q)
		if !pathHit && !bodyHit {
			continue
		}
		snippet := ""
		if bodyHit {
			idx := strings.Index(bodyLower, q)
			start := idx - 40
			if start < 0 {
				start = 0
			}
			end := idx + len(q) + 40
			if end > len(body) {
				end = len(body)
			}
			snippet = strings.ReplaceAll(body[start:end], "\n", " ")
		}
		hits = append(hits, hit{Path: p, Snippet: snippet})
		if len(hits) >= 50 {
			break
		}
	}
	if len(hits) == 0 {
		return textResult("No matches."), nil
	}
	raw, _ := json.MarshalIndent(hits, "", "  ")
	return textResult(string(raw)), nil
}

func (s *Server) readNote(userID, path string) (any, error) {
	body, err := s.readVaultText(userID, ensureMD(path))
	if err != nil {
		return nil, err
	}
	return textResult(body), nil
}

func (s *Server) writeNote(userID, path, content string, mustExist bool) (any, error) {
	path, ok := vaultRel(ensureMD(path))
	if !ok {
		return nil, fmt.Errorf("invalid path")
	}
	data := []byte(content)
	if err := s.notes().Put(userID, path, data, mustExist); err != nil {
		return nil, noteErr(err)
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	mtime := time.Now().UnixMilli()
	if err := s.Store.UpsertFile(store.FileMeta{
		UserID: userID,
		Path:   path,
		Hash:   hash,
		Size:   int64(len(data)),
		Mtime:  mtime,
	}); err != nil {
		return nil, err
	}
	s.notify(userID, path, hash, false, data)
	action := "mcp.create"
	if mustExist {
		action = "mcp.update"
	}
	_ = s.Store.AddActivity(store.Activity{UserID: userID, Action: action, Detail: path})
	return textResult("Wrote " + path), nil
}
