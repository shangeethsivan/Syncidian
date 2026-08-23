package server

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/shangeethsivan/Syncidian/internal/store"
)

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.UserCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"needs_setup": n == 0,
		"public_url":  s.Cfg.PublicURL,
	})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.UserCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, "already set up")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "username required and password must be at least 8 characters")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	u, err := s.Store.CreateUser(req.Username, hash, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sess, err := s.Store.CreateSession(u.ID, 30*24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	setSessionCookie(w, sess.ID, r.TLS != nil)
	_ = s.Store.AddActivity(store.Activity{UserID: u.ID, Action: "setup", Detail: "server initialized"})
	writeJSON(w, http.StatusCreated, map[string]any{"user": publicUser(u)})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := s.Store.GetUserByUsername(strings.TrimSpace(req.Username))
	if err != nil || u == nil || !checkPassword(u.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	sess, err := s.Store.CreateSession(u.ID, 30*24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	setSessionCookie(w, sess.ID, r.TLS != nil)
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(u)})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if c, err := r.Cookie("syncidian_session"); err == nil {
		_ = s.Store.DeleteSession(c.Value)
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, u *store.User) {
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(u), "public_url": s.Cfg.PublicURL})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request, u *store.User) {
	st, err := s.Store.Stats(u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request, u *store.User) {
	if !u.IsAdmin {
		writeJSON(w, http.StatusOK, []any{publicUser(u)})
		return
	}
	users, err := s.Store.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Admins may manage accounts but never receive vault, token, activity, or GitHub fields.
	out := make([]any, 0, len(users))
	for i := range users {
		out = append(out, publicUser(&users[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request, actor *store.User) {
	if !actor.IsAdmin {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Admin    bool   `json:"admin"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Username) == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "username required and password must be at least 8 characters")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	u, err := s.Store.CreateUser(req.Username, hash, req.Admin)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.Store.AddActivity(store.Activity{UserID: actor.ID, Action: "user.create", Detail: u.Username})
	writeJSON(w, http.StatusCreated, publicUser(u))
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request, u *store.User) {
	tokens, err := s.Store.ListTokens(u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, map[string]any{
			"id":           t.ID,
			"prefix":       t.Prefix,
			"name":         t.Name,
			"created_at":   t.CreatedAt,
			"last_used_at": t.LastUsedAt,
			"revoked":      t.RevokedAt != nil,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Name string `json:"name"`
	}
	_ = readJSON(r, &req)
	if strings.TrimSpace(req.Name) == "" {
		req.Name = "Obsidian"
	}
	raw, prefix, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	t, err := s.Store.CreateToken(u.ID, req.Name, raw, prefix)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.AddActivity(store.Activity{UserID: u.ID, Action: "token.create", Detail: req.Name})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     t.ID,
		"name":   t.Name,
		"prefix": t.Prefix,
		"token":  raw,
		"note":   "This token is shown only once. Store it in the Obsidian plugin settings.",
	})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if err := s.Store.RevokeToken(u.ID, id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGetMCP(w http.ResponseWriter, r *http.Request, u *store.User) {
	p, err := s.Store.GetMCP(u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleSetMCP(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req store.MCPPermissions
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.UserID = u.ID
	if err := s.Store.SetMCP(req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request, u *store.User) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	resp, err := s.MCP.Handle(u, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}

func (s *Server) handleMCPInfo(w http.ResponseWriter, r *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":      "syncidian",
		"version":   "0.1.0",
		"transport": "streamable-http",
		"endpoint":  "/mcp",
	})
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request, u *store.User) {
	items, err := s.Store.ListActivity(u.ID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []store.Activity{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) BootstrapFromEnv() {
	user := strings.TrimSpace(os.Getenv("SYNCIDIAN_BOOTSTRAP_USER"))
	pass := os.Getenv("SYNCIDIAN_BOOTSTRAP_PASSWORD")
	if user == "" || pass == "" {
		return
	}
	n, err := s.Store.UserCount()
	if err != nil || n > 0 {
		return
	}
	hash, err := hashPassword(pass)
	if err != nil {
		s.Log.Error("bootstrap hash", "err", err)
		return
	}
	u, err := s.Store.CreateUser(user, hash, true)
	if err != nil {
		s.Log.Error("bootstrap user", "err", err)
		return
	}
	s.Log.Info("bootstrapped admin user", "username", u.Username)
}
