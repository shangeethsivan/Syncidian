package mcp

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrNoteNotFound = errors.New("note not found")
	ErrNoteExists   = errors.New("note already exists")
	ErrNoGitHub     = errors.New("connect a GitHub repository first; Syncidian does not store notes on the server")
)

// Notes is the durable store for MCP note bodies. Production writes to GitHub.
// Tests use MemoryNotes. The server working copy is never used.
type Notes interface {
	Get(userID, path string) ([]byte, error)
	Put(userID, path string, data []byte, mustExist bool) error
	Delete(userID, path string) error
	List(userID string) ([]string, error)
}

// MemoryNotes holds notes in process for tests. It is not used in production.
type MemoryNotes struct {
	mu   sync.Mutex
	data map[string]map[string][]byte
}

func NewMemoryNotes() *MemoryNotes {
	return &MemoryNotes{data: map[string]map[string][]byte{}}
}

func (m *MemoryNotes) bucket(userID string) map[string][]byte {
	if m.data[userID] == nil {
		m.data[userID] = map[string][]byte{}
	}
	return m.data[userID]
}

func (m *MemoryNotes) Get(userID, path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bucket(userID)[path]
	if !ok {
		return nil, ErrNoteNotFound
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

func (m *MemoryNotes) Put(userID, path string, data []byte, mustExist bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.bucket(userID)
	_, exists := b[path]
	if mustExist && !exists {
		return ErrNoteNotFound
	}
	if !mustExist && exists {
		return ErrNoteExists
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	b[path] = cp
	return nil
}

func (m *MemoryNotes) Delete(userID, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bucket(userID), path)
	return nil
}

func (m *MemoryNotes) List(userID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.bucket(userID)
	out := make([]string, 0, len(b))
	for p := range b {
		out = append(out, p)
	}
	return out, nil
}

func noteErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNoteNotFound) {
		return fmt.Errorf("note not found")
	}
	if errors.Is(err, ErrNoteExists) {
		return fmt.Errorf("note already exists")
	}
	return err
}

func mdPaths(paths []string, prefix string) []string {
	var out []string
	for _, p := range paths {
		if !strings.HasSuffix(strings.ToLower(p), ".md") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(p, prefix) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (s *Server) loadNote(userID, path string) ([]byte, error) {
	b, err := s.notes().Get(userID, path)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Server) noteExists(userID, path string) bool {
	_, err := s.notes().Get(userID, path)
	return err == nil
}
