package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

type Server struct {
	Cfg   config.Config
	Store *store.Store
	Git   *gitx.Manager
	MCP   *mcp.Server
	Log   *slog.Logger
	hub   *Hub
	start time.Time
	gitMu sync.Map // userID -> *sync.Mutex
}

func New(cfg config.Config, st *store.Store, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	s := &Server{
		Cfg:   cfg,
		Store: st,
		Git:   &gitx.Manager{Name: cfg.GitName, Email: cfg.GitEmail},
		MCP:   &mcp.Server{Store: st},
		Log:   log,
		hub:   NewHub(),
		start: time.Now(),
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleHealth)

	mux.HandleFunc("GET /api/v1/setup", s.handleSetupStatus)
	mux.HandleFunc("POST /api/v1/setup", s.handleSetup)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.authed(s.handleLogout))
	mux.HandleFunc("GET /api/v1/me", s.authed(s.handleMe))
	mux.HandleFunc("GET /api/v1/stats", s.vaultAuthed(s.handleStats))

	mux.HandleFunc("GET /api/v1/users", s.authed(s.handleListUsers))
	mux.HandleFunc("POST /api/v1/users", s.authed(s.handleCreateUser))

	mux.HandleFunc("GET /api/v1/tokens", s.vaultAuthed(s.handleListTokens))
	mux.HandleFunc("POST /api/v1/tokens", s.vaultAuthed(s.handleCreateToken))
	mux.HandleFunc("POST /api/v1/tokens/{id}/revoke", s.vaultAuthed(s.handleRevokeToken))

	mux.HandleFunc("POST /api/v1/devices/register", s.vaultAuthed(s.handleRegisterDevice))
	mux.HandleFunc("GET /api/v1/devices", s.vaultAuthed(s.handleListDevices))
	mux.HandleFunc("POST /api/v1/devices/{id}/heartbeat", s.vaultAuthed(s.handleHeartbeat))
	mux.HandleFunc("DELETE /api/v1/devices/{id}", s.vaultAuthed(s.handleDeleteDevice))

	mux.HandleFunc("POST /api/v1/sync/plan", s.vaultAuthed(s.handleSyncPlan))
	mux.HandleFunc("POST /api/v1/sync/push", s.vaultAuthed(s.handleSyncPush))
	mux.HandleFunc("GET /api/v1/sync/file", s.vaultAuthed(s.handleSyncFile))
	mux.HandleFunc("GET /api/v1/sync/manifest", s.vaultAuthed(s.handleManifest))

	mux.HandleFunc("GET /api/v1/conflicts", s.vaultAuthed(s.handleListConflicts))
	mux.HandleFunc("GET /api/v1/conflicts/{id}", s.vaultAuthed(s.handleGetConflict))
	mux.HandleFunc("POST /api/v1/conflicts/{id}/resolve", s.vaultAuthed(s.handleResolveConflict))

	mux.HandleFunc("GET /api/v1/github", s.vaultAuthed(s.handleGetGitHub))
	mux.HandleFunc("POST /api/v1/github", s.vaultAuthed(s.handleSetGitHub))
	mux.HandleFunc("DELETE /api/v1/github", s.vaultAuthed(s.handleDeleteGitHub))
	mux.HandleFunc("POST /api/v1/github/sync", s.vaultAuthed(s.handleGitHubSyncNow))

	mux.HandleFunc("GET /api/v1/mcp", s.vaultAuthed(s.handleGetMCP))
	mux.HandleFunc("POST /api/v1/mcp", s.vaultAuthed(s.handleSetMCP))
	mux.HandleFunc("POST /mcp", s.vaultAuthed(s.handleMCP))
	mux.HandleFunc("GET /mcp", s.vaultAuthed(s.handleMCPInfo))

	mux.HandleFunc("GET /api/v1/activity", s.vaultAuthed(s.handleActivity))
	mux.HandleFunc("GET /api/v1/ws", s.vaultAuthed(s.handleWS))

	static, err := fs.Sub(web.FS, "static")
	if err != nil {
		s.Log.Error("embed static", "err", err)
	} else {
		fileServer := http.FileServer(http.FS(static))
		mux.Handle("GET /assets/", fileServer)
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, static, "index.html")
		})
		mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}

	return s.cors(mux)
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ctxKey int

const userKey ctxKey = 1

func (s *Server) authed(fn func(http.ResponseWriter, *http.Request, *store.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.authenticate(r)
		if err != nil || user == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
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

func (s *Server) authenticate(r *http.Request) (*store.User, error) {
	raw := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		raw = strings.TrimSpace(h[7:])
	}
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if raw != "" {
		tok, err := s.Store.GetTokenByHash(store.HashToken(raw))
		if err != nil || tok == nil || tok.RevokedAt != nil {
			return nil, err
		}
		s.Store.TouchToken(tok.ID)
		return s.Store.GetUser(tok.UserID)
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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "syncidian",
		"uptime":  time.Since(s.start).Round(time.Second).String(),
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
