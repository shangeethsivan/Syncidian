package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	encPrefix  = "enc:v1:"
	keyFile    = "secret.key"
	dataKeyEnv = "SYNCIDIAN_DATA_KEY"
)

type crypter struct {
	key [32]byte
}

func newCrypter(dataDir string) (*crypter, error) {
	raw, err := loadMasterKey(dataDir)
	if err != nil {
		return nil, err
	}
	c := &crypter{}
	copy(c.key[:], raw)
	return c, nil
}

func loadMasterKey(dataDir string) ([]byte, error) {
	if v := strings.TrimSpace(os.Getenv(dataKeyEnv)); v != "" {
		key, err := decodeKey(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dataKeyEnv, err)
		}
		return key, nil
	}
	path := filepath.Join(dataDir, keyFile)
	b, err := os.ReadFile(path)
	if err == nil {
		key, err := decodeKey(strings.TrimSpace(string(b)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func decodeKey(v string) ([]byte, error) {
	v = strings.TrimSpace(v)
	if b, err := hex.DecodeString(v); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(v); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(v); err == nil && len(b) == 32 {
		return b, nil
	}
	return nil, fmt.Errorf("need 32 bytes as hex or base64")
}

func (c *crypter) Seal(plain string) (string, error) {
	if plain == "" || c == nil {
		return plain, nil
	}
	if strings.HasPrefix(plain, encPrefix) {
		return plain, nil
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(out), nil
}

func (c *crypter) Open(stored string) (string, error) {
	if stored == "" || c == nil || !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("decrypt: ciphertext too short")
	}
	plain, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: wrong data key or corrupt secret")
	}
	return string(plain), nil
}

func (s *Store) seal(plain string) string {
	out, err := s.crypt.Seal(plain)
	if err != nil {
		return plain
	}
	return out
}

func (s *Store) openSecret(stored string) (string, error) {
	return s.crypt.Open(stored)
}
