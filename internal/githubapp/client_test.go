package githubapp

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
)

func TestSignJWT(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	tok, err := SignJWT(12345, pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt parts: %q", tok)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatal(err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"iss":12345`) {
		t.Fatalf("payload %s", payload)
	}
}

func TestNewManifestRequestsWriteAccess(t *testing.T) {
	m := NewManifest("https://sync.example.com", "Syncidian-test")
	if m.DefaultPermissions["contents"] != "write" {
		t.Fatalf("contents permission: %#v", m.DefaultPermissions)
	}
	if m.DefaultPermissions["metadata"] != "read" {
		t.Fatalf("metadata permission: %#v", m.DefaultPermissions)
	}
	if m.RedirectURL != "https://sync.example.com/api/v1/github/app/callback" {
		t.Fatalf("redirect %s", m.RedirectURL)
	}
	if m.SetupURL != "https://sync.example.com/api/v1/github/app/setup" {
		t.Fatalf("setup %s", m.SetupURL)
	}
	if m.Public {
		t.Fatal("app should not be public")
	}
}
