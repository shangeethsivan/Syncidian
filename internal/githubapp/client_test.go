package githubapp

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
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
	if m.DefaultPermissions["email_addresses"] != "read" {
		t.Fatalf("email_addresses permission: %#v", m.DefaultPermissions)
	}
	if m.RedirectURL != "https://sync.example.com/api/v1/github/app/callback" {
		t.Fatalf("redirect %s", m.RedirectURL)
	}
	if m.CallbackURLs[0] != "https://sync.example.com/api/v1/auth/github/callback" {
		t.Fatalf("oauth callback %v", m.CallbackURLs)
	}
	if m.SetupURL != "https://sync.example.com/api/v1/github/app/setup" {
		t.Fatalf("setup %s", m.SetupURL)
	}
	if m.HookAttributes["url"] != "https://sync.example.com/api/v1/github/app/webhook" {
		t.Fatalf("webhook %v", m.HookAttributes)
	}
	if !m.RequestOAuthOnInstall {
		t.Fatal("expected OAuth during install")
	}
	if m.Public {
		t.Fatal("app should not be public")
	}
}

func TestSelfHostGitHubAppGuideListsURLs(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "github-app.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/auth/github/callback",
		"/api/v1/github/app/setup",
		"/api/v1/github/app/webhook",
	} {
		if !strings.Contains(string(doc), path) {
			t.Fatalf("docs/github-app.md missing %s", path)
		}
		if !strings.Contains(string(readme), path) {
			t.Fatalf("README.md missing %s", path)
		}
	}
	if !strings.Contains(string(readme), "docs/github-app.md") {
		t.Fatal("README.md should link to docs/github-app.md")
	}
}
