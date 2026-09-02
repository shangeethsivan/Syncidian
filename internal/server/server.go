package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/shangeethsivan/Syncidian/internal/config"
	"github.com/shangeethsivan/Syncidian/internal/gitx"
	"github.com/shangeethsivan/Syncidian/internal/mcp"
	"github.com/shangeethsivan/Syncidian/internal/store"
	"github.com/shangeethsivan/Syncidian/internal/web"
	"golang.org/x/crypto/bcrypt"
)

var errInvalidToken = errors.New("invalid or revoked access token")

type Server struct {
	Cfg       config.Config
	Store     *store.Store
	Git       *gitx.Manager
	MCP       *mcp.Server
	Log       *slog.Logger
	hub       *Hub
	start     time.Time
	gitMu     sync.Map // userID -> *sync.Mutex
	tickets   sync.Map // ticket hash -> wsTicket
	authLimit *authLimiter
}

func New(cfg config.Config, st *store.Store, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	s := &Server{
		Cfg:       cfg,
		Store:     st,
		Git:       &gitx.Manager{Name: cfg.GitName, Email: cfg.GitEmail},
		MCP:       &mcp.Server{Store: st},
		Log:       log,
		hub:       NewHub(),
		start:     time.Now(),
		authLimit: newAuthLimiter(defaultAuthLimits),
	}
	s.MCP.Notes = &githubNotes{s: s}
	s.MCP.OnChange = func(userID, path, hash string, deleted bool, content []byte) {
		msg := map[string]any{
			"type":    "file_changed",
			"path":    path,
			"hash":    hash,
			"deleted": deleted,
		}
		// Large binaries (images) are pulled over HTTP instead of the WebSocket.
		const maxLiveContent = 256 * 1024
		if len(content) > 0 && !deleted && len(content) <= maxLiveContent {
			msg["content"] = base64.StdEncoding.EncodeToString(content)
		}
		s.hub.Broadcast(userID, "", msg)
	}
	s.MCP.OnBatch = func(userID string) {
		if _, err := s.importGitHubVault(userID); err != nil {
			s.Log.Error("import github after mcp batch", "user", userID, "err", err)
		}
		s.hub.Broadcast(userID, "", map[string]any{"type": "github_synced"})
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleHealth)

	mux.HandleFunc("GET /api/v1/setup", s.handleSetupStatus)
	mux.HandleFunc("POST /api/v1/setup", s.handleSetup)
	mux.HandleFunc("POST /api/v1/waitlist", s.handleJoinWaitlist)
	mux.HandleFunc("GET /api/v1/waitlist", s.adminAuthed(s.handleListWaitlist))
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/signup", s.handleSignup)
	mux.HandleFunc("POST /api/v1/auth/logout", s.authed(s.handleLogout))
	mux.HandleFunc("GET /api/v1/auth/github/start", s.handleGitHubAuthStart)
	mux.HandleFunc("GET /api/v1/auth/github/callback", s.handleGitHubAuthCallback)
	mux.HandleFunc("GET /api/v1/me", s.authed(s.handleMe))
	mux.HandleFunc("GET /api/v1/stats", s.sessionVaultAuthed(s.handleStats))

	mux.HandleFunc("GET /api/v1/users", s.authed(s.handleListUsers))
	mux.HandleFunc("POST /api/v1/users", s.authed(s.handleCreateUser))
	mux.HandleFunc("POST /api/v1/users/tokens", s.adminAuthed(s.handleAdminIssueToken))

	mux.HandleFunc("GET /api/v1/tokens", s.sessionVaultAuthed(s.handleListTokens))
	mux.HandleFunc("POST /api/v1/tokens", s.sessionVaultAuthed(s.handleCreateToken))
	mux.HandleFunc("POST /api/v1/tokens/{id}/revoke", s.sessionVaultAuthed(s.handleRevokeToken))

	mux.HandleFunc("POST /api/v1/devices/register", s.vaultAuthed(s.handleRegisterDevice))
	mux.HandleFunc("GET /api/v1/devices", s.sessionVaultAuthed(s.handleListDevices))
	mux.HandleFunc("POST /api/v1/devices/{id}/heartbeat", s.vaultAuthed(s.handleHeartbeat))
	mux.HandleFunc("DELETE /api/v1/devices/{id}", s.sessionVaultAuthed(s.handleDeleteDevice))

	mux.HandleFunc("POST /api/v1/sync/plan", s.vaultAuthed(s.handleSyncPlan))
	mux.HandleFunc("POST /api/v1/sync/push", s.vaultAuthed(s.handleSyncPush))
	mux.HandleFunc("GET /api/v1/sync/file", s.vaultAuthed(s.handleSyncFile))
	mux.HandleFunc("GET /api/v1/sync/manifest", s.vaultAuthed(s.handleManifest))

	mux.HandleFunc("GET /api/v1/conflicts", s.vaultAuthed(s.handleListConflicts))
	mux.HandleFunc("GET /api/v1/conflicts/{id}", s.vaultAuthed(s.handleGetConflict))
	mux.HandleFunc("POST /api/v1/conflicts/{id}/resolve", s.vaultAuthed(s.handleResolveConflict))

	mux.HandleFunc("GET /api/v1/github", s.sessionVaultAuthed(s.handleGetGitHub))
	mux.HandleFunc("POST /api/v1/github", s.sessionVaultAuthed(s.handleSetGitHub))
	mux.HandleFunc("DELETE /api/v1/github", s.sessionVaultAuthed(s.handleDeleteGitHub))
	mux.HandleFunc("GET /api/v1/github/tree", s.vaultAuthed(s.handleGitHubTree))
	mux.HandleFunc("POST /api/v1/github/sync", s.vaultAuthed(s.handleGitHubSyncNow))
	mux.HandleFunc("POST /api/v1/github/app/start", s.sessionVaultAuthed(s.handleGitHubAppStart))
	mux.HandleFunc("POST /api/v1/github/app/register/start", s.adminAuthed(s.handleGitHubAppRegisterStart))
	mux.HandleFunc("POST /api/v1/github/app/register", s.adminAuthed(s.handleGitHubAppRegisterSave))
	mux.HandleFunc("GET /api/v1/github/app/callback", s.handleGitHubAppCallback)
	mux.HandleFunc("GET /api/v1/github/app/setup", s.handleGitHubAppSetup)
	mux.HandleFunc("POST /api/v1/github/app/webhook", s.handleGitHubAppWebhook)
	mux.HandleFunc("GET /api/v1/github/app/webhook", s.handleGitHubAppWebhook)
	mux.HandleFunc("GET /api/v1/github/app/urls", s.handleGitHubAppURLs)

	mux.HandleFunc("GET /api/v1/mcp", s.sessionVaultAuthed(s.handleGetMCP))
	mux.HandleFunc("POST /api/v1/mcp", s.sessionVaultAuthed(s.handleSetMCP))
	mux.HandleFunc("POST /api/v1/mcp/login", s.handleMCPLogin)
	mux.HandleFunc("POST /mcp", s.vaultAuthed(s.handleMCP))
	mux.HandleFunc("GET /mcp", s.vaultAuthed(s.handleMCPInfo))

	mux.HandleFunc("GET /api/v1/activity", s.sessionVaultAuthed(s.handleActivity))
	mux.HandleFunc("POST /api/v1/ws/ticket", s.vaultAuthed(s.handleWSTicket))
	mux.HandleFunc("GET /api/v1/ws", s.handleWS)

	static, err := fs.Sub(web.FS, "static")
	if err != nil {
		s.Log.Error("embed static", "err", err)
	} else {
		fileServer := http.FileServer(http.FS(static))
		mux.HandleFunc("GET /assets/obsidian.zip", s.handlePluginZip)
		mux.Handle("GET /assets/", fileServer)
		serveHTML := func(w http.ResponseWriter, r *http.Request) {
			// Operator HTML is unlisted: no discovery Link header, noindex via middleware.
			http.ServeFileFS(w, r, static, "index.html")
		}
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			if !s.isAdminHost(r) {
				w.Header().Set("Link", s.discoveryLinkHeader(s.publicOrigin(r)))
			}
			w.Header().Set("Vary", "Accept")
			if wantsMarkdown(r.Header.Get("Accept")) {
				s.handleLandingMarkdown(w, r)
				return
			}
			http.ServeFileFS(w, r, static, "index.html")
		})
		admin := s.adminPath()
		mux.HandleFunc("GET "+admin, serveHTML)
		mux.HandleFunc("GET "+admin+"/{$}", serveHTML)
		if admin != "/admin" {
			mux.HandleFunc("GET /admin", http.NotFound)
			mux.HandleFunc("GET /admin/{$}", http.NotFound)
		}
		mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		mux.HandleFunc("GET /robots.txt", s.handleRobots)
		mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
		mux.HandleFunc("GET /auth.md", s.handleAuthMD)
		mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)
		mux.HandleFunc("GET /.well-known/api-catalog", s.handleAPICatalog)
		mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.handleOAuthProtectedResource)
		mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleOAuthAS)
		mux.HandleFunc("GET /.well-known/openid-configuration", s.handleOAuthAS)
		mux.HandleFunc("GET /.well-known/mcp/server-card.json", s.handleMCPServerCard)
		mux.HandleFunc("GET /.well-known/mcp.json", s.handleMCPServerCard)
		mux.HandleFunc("GET /.well-known/mcp/server-cards.json", s.handleMCPServerCards)
		mux.HandleFunc("GET /.well-known/agent-skills/index.json", s.handleAgentSkillsIndex)
		mux.HandleFunc("GET /.well-known/skills/index.json", s.handleAgentSkillsIndex)
		mux.HandleFunc("GET /.well-known/agent-skills/syncidian-mcp/SKILL.md", s.handleAgentSkillMD)
		mux.HandleFunc("GET /.well-known/ai-catalog.json", s.handleAICatalog)
	}

	return s.noIndexOperator(s.adminHostGate(s.cors(mux)))
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if s.originMayUseCookies(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Syncidian-Client")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originMayUseCookies(origin string) bool {
	origin = strings.TrimRight(origin, "/")
	if origin == "" {
		return false
	}
	if strings.HasPrefix(origin, "app://") || strings.HasPrefix(origin, "capacitor://") || strings.HasPrefix(origin, "ionic://") {
		return true
	}
	if s.Cfg.PublicURL != "" && origin == strings.TrimRight(s.Cfg.PublicURL, "/") {
		return true
	}
	if s.Cfg.AdminHost != "" {
		adminOrigin := "https://" + s.Cfg.AdminHost
		if origin == adminOrigin || origin == "http://"+s.Cfg.AdminHost {
			return true
		}
	}
	host := rHost(origin)
	return host == "localhost" || strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "[::1]")
}

func (s *Server) adminPath() string {
	return config.NormalizeAdminPath(s.Cfg.AdminPath)
}

func (s *Server) adminURL(r *http.Request) string {
	return s.requestBase(r) + s.adminPath()
}

func rHost(origin string) string {
	origin = strings.TrimPrefix(origin, "https://")
	origin = strings.TrimPrefix(origin, "http://")
	if i := strings.IndexByte(origin, '/'); i >= 0 {
		origin = origin[:i]
	}
	return origin
}

type ctxKey int

const userKey ctxKey = 1

func (s *Server) authed(fn func(http.ResponseWriter, *http.Request, *store.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.authenticate(r)
		if err != nil || user == nil {
			msg := "unauthorized"
			if errors.Is(err, errInvalidToken) {
				msg = err.Error()
			}
			writeError(w, http.StatusUnauthorized, msg)
			return
		}
		fn(w, r.WithContext(context.WithValue(r.Context(), userKey, user)), user)
	}
}

// vaultAuthed is for a regular user's own vault, tokens, devices, and GitHub repo.
// Admins manage accounts only and cannot read or write that private data.
func (s *Server) vaultAuthed(fn func(http.ResponseWriter, *http.Request, *store.User)) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if u.IsAdmin {
			writeError(w, http.StatusForbidden, "admins manage users and cannot access private vault or GitHub data")
			return
		}
		fn(w, r, u)
	})
}

// sessionVaultAuthed is dashboard-only. Access tokens cannot connect/disconnect
// GitHub, mint more tokens, or change MCP permissions — those bind the private repo.
func (s *Server) sessionVaultAuthed(fn func(http.ResponseWriter, *http.Request, *store.User)) http.HandlerFunc {
	return s.vaultAuthed(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if bearerRequest(r) {
			writeError(w, http.StatusForbidden, "access tokens cannot manage GitHub or account settings; sign in on the dashboard")
			return
		}
		fn(w, r, u)
	})
}

func bearerRequest(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	return strings.HasPrefix(strings.ToLower(h), "bearer ")
}

func (s *Server) authenticate(r *http.Request) (*store.User, error) {
	raw := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		// Slice the original header after the first space so "Bearer"/"bearer"/"BEARER" all work.
		if i := strings.IndexByte(h, ' '); i >= 0 {
			raw = strings.TrimSpace(h[i+1:])
		}
	}
	if raw != "" {
		tok, err := s.Store.GetTokenByHash(store.HashToken(raw))
		if err != nil {
			return nil, err
		}
		if tok == nil || tok.RevokedAt != nil {
			return nil, errInvalidToken
		}
		s.Store.TouchToken(tok.ID)
		u, err := s.Store.GetUser(tok.UserID)
		if err != nil || u == nil {
			return nil, errInvalidToken
		}
		return u, nil
	}
	c, err := r.Cookie("syncidian_session")
	if err != nil || c.Value == "" {
		return nil, nil
	}
	sess, err := s.Store.GetSession(c.Value)
	if err != nil || sess == nil {
		return nil, err
	}
	return s.Store.GetUser(sess.UserID)
}

func (s *Server) persistence() config.Persistence {
	return config.PersistenceStatus(s.Cfg.DataDir)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	p := s.persistence()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"service":     "syncidian",
		"uptime":      time.Since(s.start).Round(time.Second).String(),
		"persistence": p,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 32<<20))
	return dec.Decode(v)
}

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func checkPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func randomToken() (raw, prefix string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = "sk_sync_" + hex.EncodeToString(b)
	prefix = raw[:16] + "…"
	return raw, prefix, nil
}

func setSessionCookie(w http.ResponseWriter, id string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "syncidian_session",
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "syncidian_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func fileSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func safeJoin(root, rel string) (string, bool) {
	rel = path.Clean("/" + strings.ReplaceAll(rel, "\\", "/"))
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	full := path.Clean(root + "/" + rel)
	if full != root && !strings.HasPrefix(full, root+"/") {
		return "", false
	}
	return full, true
}

func publicUser(u *store.User) map[string]any {
	return map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"is_admin":   u.IsAdmin,
		"created_at": u.CreatedAt,
	}
}

// adminUserSummary is the only user record an admin may see: no id, vault, tokens, or GitHub.
func adminUserSummary(u *store.User) map[string]any {
	return map[string]any{
		"username":   u.Username,
		"is_admin":   u.IsAdmin,
		"created_at": u.CreatedAt,
	}
}

func deviceStatus(lastSeen time.Time) string {
	if time.Since(lastSeen) < 2*time.Minute {
		return "active"
	}
	return "offline"
}
