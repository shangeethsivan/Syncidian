package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shangeethsivan/Syncidian/internal/store"
)

func setupMCP(t *testing.T) (*Server, *store.User) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	u, err := st.CreateUser("alice", "hash", false)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.SetMCP(store.MCPPermissions{UserID: u.ID, Search: true, Read: true, Create: true, Modify: true})
	return &Server{Store: st, Notes: NewMemoryNotes()}, u
}

func call(t *testing.T, s *Server, u *store.User, name string, args map[string]any) string {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"name": name, "arguments": args})
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  json.RawMessage(params),
	})
	resp, err := s.Handle(u, body, ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		t.Fatal(err)
	}
	if out.Error != nil {
		t.Fatalf("rpc error: %s", out.Error.Message)
	}
	if len(out.Result.Content) == 0 {
		t.Fatal("empty content")
	}
	return out.Result.Content[0].Text
}

func TestCreateIndexesAndSearch(t *testing.T) {
	s, u := setupMCP(t)
	text := call(t, s, u, "create_note", map[string]any{
		"path": "Ideas/Widget.md", "content": "# Widget\n\nAn idea about widgets.\n",
	})
	if !strings.Contains(text, "Ideas/Widget.md") {
		t.Fatalf("unexpected: %s", text)
	}
	listed := call(t, s, u, "list_notes", map[string]any{"prefix": "Ideas/"})
	if !strings.Contains(listed, "Ideas/Widget.md") {
		t.Fatalf("list missed new note: %s", listed)
	}
	found := call(t, s, u, "search_notes", map[string]any{"query": "widgets"})
	if !strings.Contains(found, "Ideas/Widget.md") {
		t.Fatalf("search missed note: %s", found)
	}
}

func TestBacklinksAndGraph(t *testing.T) {
	s, u := setupMCP(t)
	_ = call(t, s, u, "create_note", map[string]any{
		"path": "A.md", "content": "See [[B]] and [[Ideas/C]].\n",
	})
	_ = call(t, s, u, "create_note", map[string]any{
		"path": "B.md", "content": "Hello\n",
	})
	_ = call(t, s, u, "create_note", map[string]any{
		"path": "Ideas/C.md", "content": "Linked from A\n",
	})

	out := call(t, s, u, "get_outgoing_links", map[string]any{"path": "A.md"})
	if !strings.Contains(out, "B.md") || !strings.Contains(out, "Ideas/C.md") {
		t.Fatalf("outgoing: %s", out)
	}
	back := call(t, s, u, "get_backlinks", map[string]any{"path": "B.md"})
	if !strings.Contains(back, "A.md") {
		t.Fatalf("backlinks: %s", back)
	}
	graph := call(t, s, u, "get_graph", map[string]any{"format": "json"})
	if !strings.Contains(graph, "mermaid") || !strings.Contains(graph, "edges") {
		t.Fatalf("graph: %s", graph)
	}
	_ = call(t, s, u, "add_backlink", map[string]any{"from": "B.md", "to": "A.md"})
	body := call(t, s, u, "read_note", map[string]any{"path": "B.md"})
	if !strings.Contains(body, "[[A]]") && !strings.Contains(body, "[[A.md]]") {
		t.Fatalf("missing backlink text: %s", body)
	}
}

func TestSuggestAndAppend(t *testing.T) {
	s, u := setupMCP(t)
	_ = call(t, s, u, "create_note", map[string]any{
		"path": "Ideas/Product.md", "content": "---\ntype: idea\n---\n\n# Product ideas\n",
	})
	sug := call(t, s, u, "suggest_note_path", map[string]any{"topic": "product launch", "kind": "idea"})
	if !strings.Contains(sug, "Ideas/Product.md") && !strings.Contains(sug, "suggested_new") {
		t.Fatalf("suggest: %s", sug)
	}
	_ = call(t, s, u, "append_to_note", map[string]any{
		"path": "Ideas/Product.md", "content": "- new bullet", "heading": "Product ideas",
	})
	body := call(t, s, u, "read_note", map[string]any{"path": "Ideas/Product.md"})
	if !strings.Contains(body, "- new bullet") {
		t.Fatalf("append failed: %s", body)
	}
}

func TestBulkMove(t *testing.T) {
	s, u := setupMCP(t)
	_ = call(t, s, u, "create_note", map[string]any{"path": "Inbox/One.md", "content": "1\n"})
	_ = call(t, s, u, "create_note", map[string]any{"path": "Inbox/Two.md", "content": "2\n"})
	out := call(t, s, u, "bulk_move", map[string]any{
		"from_prefix": "Inbox", "to_prefix": "Archive",
	})
	if !strings.Contains(out, `"count": 2`) && !strings.Contains(out, `"count":2`) {
		t.Fatalf("bulk move: %s", out)
	}
	listed := call(t, s, u, "list_notes", map[string]any{"prefix": "Archive/"})
	if !strings.Contains(listed, "Archive/One.md") || !strings.Contains(listed, "Archive/Two.md") {
		t.Fatalf("list after move: %s", listed)
	}
	if _, err := s.Notes.Get(u.ID, "Inbox/One.md"); err == nil {
		t.Fatal("old path still exists")
	}
}

func TestPermissionsGateWrites(t *testing.T) {
	s, u := setupMCP(t)
	_ = s.Store.SetMCP(store.MCPPermissions{UserID: u.ID, Search: true, Read: true, Create: false, Modify: false})
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "create_note",
			"arguments": map[string]any{"path": "X.md", "content": "hi"},
		},
	})
	resp, err := s.Handle(u, body, ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp), "permission denied") {
		t.Fatalf("expected denial: %s", resp)
	}
}

func TestPathTraversalRejected(t *testing.T) {
	s, u := setupMCP(t)
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "create_note",
			"arguments": map[string]any{"path": "../escape.md", "content": "nope"},
		},
	})
	resp, err := s.Handle(u, body, ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp), "invalid path") {
		t.Fatalf("expected invalid path: %s", resp)
	}
}

func TestExtractWikiTargets(t *testing.T) {
	got := extractWikiTargets("See [[Foo]] and [[Bar|alias]] plus [[Baz#Head|x]].")
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
}

func TestVaultRel(t *testing.T) {
	if _, ok := vaultRel("../x.md"); ok {
		t.Fatal("should reject")
	}
	if p, ok := vaultRel("a/b.md"); !ok || p != "a/b.md" {
		t.Fatalf("got %q %v", p, ok)
	}
	if p, ok := vaultRel("Content/pic.png"); !ok || p != "Content/pic.png" {
		t.Fatalf("png path: %q %v", p, ok)
	}
}

func TestMoveAnyFileType(t *testing.T) {
	s, u := setupMCP(t)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a}
	if err := s.Notes.Put(u.ID, "Inbox/shot.png", png, false); err != nil {
		t.Fatal(err)
	}
	out := call(t, s, u, "move_note", map[string]any{
		"from": "Inbox/shot.png", "to": "Content/shot.png",
	})
	if !strings.Contains(out, "Content/shot.png") {
		t.Fatalf("move: %s", out)
	}
	if _, err := s.Notes.Get(u.ID, "Inbox/shot.png"); err == nil {
		t.Fatal("old image path still exists")
	}
	got, err := s.Notes.Get(u.ID, "Content/shot.png")
	if err != nil || string(got) != string(png) {
		t.Fatalf("new image: %v %v", got, err)
	}
	listed := call(t, s, u, "list_files", map[string]any{"ext": "png"})
	if !strings.Contains(listed, "Content/shot.png") {
		t.Fatalf("list_files: %s", listed)
	}
}

func TestBulkMoveIncludesAttachments(t *testing.T) {
	s, u := setupMCP(t)
	_ = call(t, s, u, "create_note", map[string]any{"path": "Inbox/One.md", "content": "1\n"})
	if err := s.Notes.Put(u.ID, "Inbox/shot.png", []byte("img"), false); err != nil {
		t.Fatal(err)
	}
	out := call(t, s, u, "bulk_move", map[string]any{
		"from_prefix": "Inbox", "to_prefix": "Archive",
	})
	if !strings.Contains(out, `"count": 2`) && !strings.Contains(out, `"count":2`) {
		t.Fatalf("bulk move: %s", out)
	}
	if _, err := s.Notes.Get(u.ID, "Archive/shot.png"); err != nil {
		t.Fatalf("png not moved: %v", err)
	}
}

func TestCreateDoesNotWriteServerDisk(t *testing.T) {
	s, u := setupMCP(t)
	_ = call(t, s, u, "create_note", map[string]any{
		"path": "Weekly Focus.md", "content": "# Weekly Focus\n",
	})
	root := s.Store.VaultDir(u.ID)
	if _, err := os.Stat(filepath.Join(root, "Weekly Focus.md")); err == nil {
		t.Fatal("MCP must not write notes onto the server working copy")
	}
	got, err := s.Notes.Get(u.ID, "Weekly Focus.md")
	if err != nil || !strings.Contains(string(got), "Weekly Focus") {
		t.Fatalf("note should live in the notes backend: %s %v", got, err)
	}
}
