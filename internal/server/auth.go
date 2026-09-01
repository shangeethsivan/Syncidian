package server

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/shangeethsivan/Syncidian/internal/githubapp"
	"github.com/shangeethsivan/Syncidian/internal/mcp"
	"github.com/shangeethsivan/Syncidian/internal/store"
)

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.UserCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	app := s.instanceGitHubApp()
	urls := githubapp.AppURLs(s.githubBase(r))
	out := map[string]any{
		"needs_setup":    n == 0,
		"public_url":     s.Cfg.PublicURL,
		"github_login":   app.Configured(),
		"persistence":    s.persistence(),
		"waitlist":       s.isHostedPublic(r),
		"waitlist_admin": s.hostedWaitlistAdmin(r),
		"github_app": map[string]any{
			"configured": app.Configured(),
			"slug":       "",
			"urls":       urls,
		},
	}
	if s.isAdminHost(r) && s.Cfg.AdminHost != "" {
		out["admin_host"] = s.Cfg.AdminHost
	}
	if app.Configured() {
		gh, _ := out["github_app"].(map[string]any)
		gh["slug"] = app.Slug
		out["github_app"] = gh
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.requireAdminHost(w, r) {
		return
	}
	if s.denySetup(w, r) {
		return
	}
	s.noteSetup(r)
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
	setSessionCookie(w, sess.ID, r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
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
	username := strings.TrimSpace(req.Username)
	if s.denyPassword(w, r, username) {
		return
	}
	u, err := s.Store.GetUserByUsername(username)
	if err != nil || u == nil || u.PasswordHash == "" || !checkPassword(u.PasswordHash, req.Password) {
		s.notePasswordFail(r, username)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if s.adminHostConfigured() {
		if u.IsAdmin && !s.isAdminHost(r) {
			http.NotFound(w, r)
			return
		}
		if !u.IsAdmin && s.isAdminHost(r) {
			writeError(w, http.StatusForbidden, "operators only")
			return
		}
	}
	s.notePasswordOK(username)
	sess, err := s.Store.CreateSession(u.ID, 30*24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	setSessionCookie(w, sess.ID, r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(u)})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	if s.denySignup(w, r) {
		return
	}
	n, err := s.Store.UserCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n == 0 {
		writeError(w, http.StatusBadRequest, "Create the first admin before signing up.")
		return
	}
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	s.noteSignup(r)
	if req.Username == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "username required and password must be at least 8 characters")
		return
	}
	if req.Email != "" {
		if existing, _ := s.Store.GetUserByEmail(req.Email); existing != nil {
			writeError(w, http.StatusBadRequest, "email already in use")
			return
		}
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	u, err := s.Store.CreateUser(req.Username, hash, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Email != "" {
		_ = s.Store.SetUserGitHub(u.ID, 0, req.Email)
		u.Email = req.Email
	}
	sess, err := s.Store.CreateSession(u.ID, 30*24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	setSessionCookie(w, sess.ID, r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
	_ = s.Store.AddActivity(store.Activity{UserID: u.ID, Action: "signup", Detail: u.Username})
	writeJSON(w, http.StatusCreated, map[string]any{"user": publicUser(u)})
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
	if u.IsAdmin {
		if s.requireAdminHost(w, r) {
			return
		}
	}
	if !u.IsAdmin {
		writeJSON(w, http.StatusOK, []any{adminUserSummary(u)})
		return
	}
	users, err := s.Store.ListUsersPublic()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Admins may manage accounts but never receive vault, token, activity, or GitHub fields.
	out := make([]any, 0, len(users))
	for i := range users {
		out = append(out, adminUserSummary(&users[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request, actor *store.User) {
	if s.requireAdminHost(w, r) {
		return
	}
	if !actor.IsAdmin {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		Admin      bool   `json:"admin"`
		IssueToken bool   `json:"issue_token"`
		TokenName  string `json:"token_name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Username) == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "username required and password must be at least 8 characters")
		return
	}
	if req.Admin && req.IssueToken {
		writeError(w, http.StatusBadRequest, "cannot issue an Obsidian token for an admin account")
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
	out := publicUser(u)
	if req.IssueToken && !u.IsAdmin {
		raw, prefix, tok, err := s.issueVaultToken(u, req.TokenName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out["token"] = raw
		out["token_id"] = tok.ID
		out["token_prefix"] = prefix
		out["token_note"] = "This Obsidian access token is shown only once. Paste it into the plugin."
		_ = s.Store.AddActivity(store.Activity{UserID: actor.ID, Action: "token.create", Detail: u.Username + ":" + tok.Name})
	}
	writeJSON(w, http.StatusCreated, out)
}

// handleAdminIssueToken mints a one-time sk_sync_ token for a vault user.
// Admins never list existing tokens or vault files — they only receive the raw
// value at creation time so they can onboard the Obsidian plugin.
func (s *Server) handleAdminIssueToken(w http.ResponseWriter, r *http.Request, actor *store.User) {
	var req struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username required")
		return
	}
	u, err := s.Store.GetUserByUsername(req.Username)
	if err != nil || u == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if u.IsAdmin {
		writeError(w, http.StatusBadRequest, "admins cannot hold vault access tokens — pick a vault user")
		return
	}
	raw, prefix, tok, err := s.issueVaultToken(u, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.AddActivity(store.Activity{UserID: actor.ID, Action: "token.create", Detail: u.Username + ":" + tok.Name})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       tok.ID,
		"name":     tok.Name,
		"prefix":   prefix,
		"username": u.Username,
		"token":    raw,
		"note":     "This token is shown only once. Store it in the Obsidian plugin settings.",
	})
}

func (s *Server) issueVaultToken(u *store.User, name string) (raw, prefix string, tok *store.Token, err error) {
	if strings.TrimSpace(name) == "" {
		name = "Obsidian"
	}
	raw, prefix, err = randomToken()
	if err != nil {
		return "", "", nil, err
	}
	tok, err = s.Store.CreateToken(u.ID, name, raw, prefix)
	return raw, prefix, tok, err
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
	raw, prefix, t, err := s.issueVaultToken(u, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.AddActivity(store.Activity{UserID: u.ID, Action: "token.create", Detail: req.Name})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     t.ID,
		"name":   t.Name,
		"prefix": prefix,
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
	usage, err := s.Store.MCPUsage(u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": p.UserID,
		"search":  p.Search,
		"read":    p.Read,
		"create":  p.Create,
		"modify":  p.Modify,
		"usage":   usage,
	})
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

// handleMCPLogin exchanges username/password for a Bearer token usable with POST /mcp.
// Admins are rejected — MCP is vault-user only.
func (s *Server) handleMCPLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		TokenName string `json:"token_name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	if s.denyPassword(w, r, req.Username) {
		return
	}
	u, err := s.Store.GetUserByUsername(req.Username)
	if err != nil || u == nil || u.PasswordHash == "" || !checkPassword(u.PasswordHash, req.Password) {
		s.notePasswordFail(r, req.Username)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	s.notePasswordOK(req.Username)
	if u.IsAdmin {
		writeError(w, http.StatusForbidden, "admins cannot use MCP; sign in as a vault user")
		return
	}
	name := strings.TrimSpace(req.TokenName)
	if name == "" {
		name = "MCP"
	}
	raw, prefix, tok, err := s.issueVaultToken(u, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.AddActivity(store.Activity{UserID: u.ID, Action: "mcp.login", Detail: name})
	writeJSON(w, http.StatusOK, map[string]any{
		"token":    raw,
		"prefix":   prefix,
		"id":       tok.ID,
		"name":     tok.Name,
		"endpoint": "/mcp",
		"note":     "Use Authorization: Bearer <token> on POST /mcp. This token is shown only once.",
	})
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request, u *store.User) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	meta := mcp.ClientMeta{UserAgent: r.Header.Get("User-Agent")}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		if i := strings.IndexByte(h, ' '); i >= 0 {
			raw := strings.TrimSpace(h[i+1:])
			if tok, err := s.Store.GetTokenByHash(store.HashToken(raw)); err == nil && tok != nil {
				meta.TokenID = tok.ID
				meta.TokenName = tok.Name
				meta.TokenPrefix = tok.Prefix
			}
		}
	}
	resp, err := s.MCP.Handle(u, body, meta)
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
		"version":   "0.2.0",
		"transport": "streamable-http",
		"endpoint":  "/mcp",
		"auth": map[string]any{
			"bearer_token": "Authorization: Bearer sk_sync_… (create via dashboard Tokens or POST /api/v1/mcp/login)",
			"session":      "Dashboard cookie syncidian_session after POST /api/v1/auth/login",
			"login":        "POST /api/v1/mcp/login with {username, password} returns a Bearer token",
		},
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
