package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shangeethsivan/Syncidian/internal/store"
	"nhooyr.io/websocket"
)

type Hub struct {
	mu      sync.Mutex
	clients map[string]map[*wsClient]struct{}
}

type wsClient struct {
	userID   string
	deviceID string
	conn     *websocket.Conn
}

func NewHub() *Hub {
	return &Hub{clients: map[string]map[*wsClient]struct{}{}}
}

func (h *Hub) add(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.userID] == nil {
		h.clients[c.userID] = map[*wsClient]struct{}{}
	}
	h.clients[c.userID][c] = struct{}{}
}

func (h *Hub) remove(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.clients[c.userID]; m != nil {
		delete(m, c)
		if len(m) == 0 {
			delete(h.clients, c.userID)
		}
	}
}

func (h *Hub) Broadcast(userID, skipDevice string, msg any) {
	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients[userID] {
		if skipDevice != "" && c.deviceID == skipDevice {
			continue
		}
		cc := c
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = cc.conn.Write(ctx, websocket.MessageText, raw)
		}()
	}
}

func (s *Server) handleWSTicket(w http.ResponseWriter, r *http.Request, u *store.User) {
	raw, err := randomHex(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue websocket ticket")
		return
	}
	ticket := "wst_" + raw
	s.tickets.Store(store.HashToken(ticket), wsTicket{
		userID:  u.ID,
		expires: time.Now().Add(60 * time.Second),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ticket": ticket, "expires_in": 60})
}

type wsTicket struct {
	userID  string
	expires time.Time
}

func (s *Server) redeemWSTicket(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	v, ok := s.tickets.LoadAndDelete(store.HashToken(raw))
	if !ok {
		return "", false
	}
	t, _ := v.(wsTicket)
	if t.userID == "" || time.Now().After(t.expires) {
		return "", false
	}
	return t.userID, true
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("token")) != "" {
		writeError(w, http.StatusBadRequest, "do not put access tokens in the WebSocket URL; POST /api/v1/ws/ticket and connect with the ticket")
		return
	}
	userID := ""
	if ticket := strings.TrimSpace(r.URL.Query().Get("ticket")); ticket != "" {
		id, ok := s.redeemWSTicket(ticket)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid or expired websocket ticket")
			return
		}
		userID = id
	} else {
		u, err := s.authenticate(r)
		if err != nil || u == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if u.IsAdmin {
			writeError(w, http.StatusForbidden, "admins manage users and cannot access private vault or GitHub data")
			return
		}
		userID = u.ID
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		OriginPatterns:     []string{"*"},
	})
	if err != nil {
		return
	}
	deviceID := r.URL.Query().Get("device_id")
	client := &wsClient{userID: userID, deviceID: deviceID, conn: conn}
	s.hub.add(client)
	defer func() {
		s.hub.remove(client)
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	}()
	for {
		_, _, err := conn.Read(r.Context())
		if err != nil {
			return
		}
	}
}

func walkVault(root string, fn func(rel string, b []byte, mtime int64) error) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return fn(rel, b, info.ModTime().Unix())
	})
}
