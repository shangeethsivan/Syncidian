package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
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

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request, u *store.User) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		OriginPatterns:     []string{"*"},
	})
	if err != nil {
		return
	}
	deviceID := r.URL.Query().Get("device_id")
	client := &wsClient{userID: u.ID, deviceID: deviceID, conn: conn}
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
