package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSchemaVersionOnFreshInstall(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	version, err := st.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != latestMigrationVersion() {
		t.Fatalf("schema version = %d, want %d", version, latestMigrationVersion())
	}
}

func TestUsersRetainedAcrossDeployments(t *testing.T) {
	dir := t.TempDir()

	st1, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ada, err := st1.CreateUser("ada", "hash", true)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := st1.CreateUser("bob", "hash", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st1.CreateToken(bob.ID, "laptop", "sk_sync_retain", "sk_sync_…"); err != nil {
		t.Fatal(err)
	}
	if err := st1.SetGitHub(GitHubConfig{UserID: bob.ID, Token: "tok", Repo: "bob/vault", Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := st1.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()

	version, err := st2.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != latestMigrationVersion() {
		t.Fatalf("schema version after reopen = %d, want %d", version, latestMigrationVersion())
	}

	gotAda, err := st2.GetUser(ada.ID)
	if err != nil || gotAda == nil || gotAda.Username != "ada" || !gotAda.IsAdmin {
		t.Fatalf("ada after reopen: %+v %v", gotAda, err)
	}
	gotBob, err := st2.GetUserByUsername("bob")
	if err != nil || gotBob == nil || gotBob.ID != bob.ID {
		t.Fatalf("bob after reopen: %+v %v", gotBob, err)
	}
	tok, err := st2.GetTokenByHash(HashToken("sk_sync_retain"))
	if err != nil || tok == nil || tok.UserID != bob.ID {
		t.Fatalf("token after reopen: %+v %v", tok, err)
	}
	gh, err := st2.GetGitHub(bob.ID)
	if err != nil || gh == nil || gh.Repo != "bob/vault" {
		t.Fatalf("github config after reopen: %+v %v", gh, err)
	}
	n, err := st2.UserCount()
	if err != nil || n != 2 {
		t.Fatalf("user count after reopen = %d, want 2 (%v)", n, err)
	}
}

func TestLegacyDatabaseGetsMigrationStamp(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "syncidian.db")
	dsn := "file:" + filepath.ToSlash(dbPath) + "?_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := migration001InitialSchema(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(
		`INSERT INTO users (id, username, password_hash, is_admin, created_at) VALUES (?, ?, ?, ?, ?)`,
		"legacy-user-id", "legacy", "hash", 1, createdAt,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	version, err := st.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != latestMigrationVersion() {
		t.Fatalf("legacy schema version = %d, want %d", version, latestMigrationVersion())
	}
	u, err := st.GetUserByUsername("legacy")
	if err != nil || u == nil || u.ID != "legacy-user-id" {
		t.Fatalf("legacy user retained: %+v %v", u, err)
	}

	// GitHub App columns from migration 002 must exist on upgraded legacy DBs.
	if err := st.SetInstanceGitHubApp(GitHubApp{
		AppID: 1, Slug: "syncidian", PEM: "pem", ClientID: "cid", ClientSecret: "sec",
	}); err != nil {
		t.Fatalf("instance github app after legacy upgrade: %v", err)
	}
}

func TestUpgradeFromV1AddsGitHubAppSchema(t *testing.T) {
	dir := t.TempDir()
	st1, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st1.CreateUser("ada", "hash", true); err != nil {
		t.Fatal(err)
	}
	// Simulate a DB that only applied migration 1 (pre-GitHub-App).
	if _, err := st1.db.Exec(`DELETE FROM schema_migrations WHERE version > 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := st1.db.Exec(`DROP TABLE IF EXISTS instance_github_app`); err != nil {
		t.Fatal(err)
	}
	if err := st1.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	v, err := st2.SchemaVersion()
	if err != nil || v != latestMigrationVersion() {
		t.Fatalf("schema version after v1 upgrade = %d, want %d (%v)", v, latestMigrationVersion(), err)
	}
	if err := st2.SetInstanceGitHubApp(GitHubApp{
		AppID: 42, Slug: "upgraded", PEM: "pem", ClientID: "cid", ClientSecret: "sec",
	}); err != nil {
		t.Fatalf("github app after v1→v2: %v", err)
	}
	u, err := st2.GetUserByUsername("ada")
	if err != nil || u == nil {
		t.Fatalf("user retained after v1→v2: %+v %v", u, err)
	}
}
