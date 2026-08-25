package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubSecretsEncryptedAtRest(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	u, err := st.CreateUser("bob", "hash", false)
	if err != nil {
		t.Fatal(err)
	}
	pem := "-----BEGIN RSA PRIVATE KEY-----\nSECRETPEM\n-----END RSA PRIVATE KEY-----"
	if err := st.SetInstanceGitHubApp(GitHubApp{
		AppID: 9, Slug: "syncidian", PEM: pem, ClientID: "Iv1.abc", ClientSecret: "super-secret",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetGitHub(GitHubConfig{
		UserID: u.ID, Token: "ghs_install_token", Repo: "bob/vault",
		AppID: 9, AppSlug: "syncidian", AppPEM: pem, ClientID: "Iv1.abc",
		ClientSecret: "super-secret", InstallationID: 44,
	}); err != nil {
		t.Fatal(err)
	}

	var storedPEM, storedSecret, storedTok string
	if err := st.db.QueryRow(`SELECT pem, client_secret FROM instance_github_app WHERE id = 1`).Scan(&storedPEM, &storedSecret); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT token FROM github_config WHERE user_id = ?`, u.ID).Scan(&storedTok); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{storedPEM, storedSecret, storedTok} {
		if !strings.HasPrefix(v, encPrefix) {
			t.Fatalf("expected enc:v1: ciphertext, got %q", v)
		}
	}
	for _, leak := range []string{pem, "SECRETPEM", "super-secret", "ghs_install_token"} {
		if strings.Contains(storedPEM, leak) || strings.Contains(storedSecret, leak) || strings.Contains(storedTok, leak) {
			t.Fatalf("sqlite column leaked %q", leak)
		}
	}

	gotApp, err := st.GetInstanceGitHubApp()
	if err != nil || gotApp == nil || gotApp.PEM != pem || gotApp.ClientSecret != "super-secret" {
		t.Fatalf("decrypt instance app: %+v %v", gotApp, err)
	}
	got, err := st.GetGitHub(u.ID)
	if err != nil || got == nil || got.Token != "ghs_install_token" || got.AppPEM != pem {
		t.Fatalf("decrypt github config: %+v %v", got, err)
	}

	st.Close()
	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	got2, err := st2.GetGitHub(u.ID)
	if err != nil || got2 == nil || got2.ClientSecret != "super-secret" {
		t.Fatalf("reopen decrypt: %+v %v", got2, err)
	}
}

func TestLegacyPlaintextGitHubSecretsStillRead(t *testing.T) {
	c, err := newCrypter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plain, err := c.Open("not-encrypted")
	if err != nil || plain != "not-encrypted" {
		t.Fatalf("got %q %v", plain, err)
	}
}

func TestDataKeyEnv(t *testing.T) {
	key := strings.Repeat("ab", 32)
	t.Setenv("SYNCIDIAN_DATA_KEY", key)
	dir := t.TempDir()
	c, err := newCrypter(dir)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Seal("hello")
	if err != nil || !strings.HasPrefix(sealed, encPrefix) {
		t.Fatalf("seal %q %v", sealed, err)
	}
	out, err := c.Open(sealed)
	if err != nil || out != "hello" {
		t.Fatalf("open %q %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(dir, keyFile)); !os.IsNotExist(err) {
		t.Fatal("env key should not write secret.key")
	}
}
