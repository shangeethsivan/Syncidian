package store

import (
	"database/sql"
	"fmt"
	"time"
)

type migration struct {
	version int
	name    string
	up      func(tx *sql.Tx) error
}

var migrations = []migration{
	{version: 1, name: "initial_schema", up: migration001InitialSchema},
}

func (s *Store) migrate() error {
	if err := s.ensureMigrationsTable(); err != nil {
		return err
	}
	current, err := s.currentSchemaVersion()
	if err != nil {
		return err
	}
	if current == 0 && s.hasLegacySchema() {
		return s.stampLegacySchema(latestMigrationVersion())
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := s.runMigration(m); err != nil {
			return fmt.Errorf("migration %d_%s: %w", m.version, m.name, err)
		}
	}
	return nil
}

func (s *Store) SchemaVersion() (int, error) {
	if err := s.ensureMigrationsTable(); err != nil {
		return 0, err
	}
	return s.currentSchemaVersion()
}

func latestMigrationVersion() int {
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].version
}

func (s *Store) ensureMigrationsTable() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TEXT NOT NULL
)`)
	return err
}

func (s *Store) currentSchemaVersion() (int, error) {
	var version sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return 0, err
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

func (s *Store) hasLegacySchema() bool {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'users'`).Scan(&n)
	return err == nil && n > 0
}

func (s *Store) stampLegacySchema(version int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	appliedAt := time.Now().UTC().Format(time.RFC3339Nano)
	for _, m := range migrations {
		if m.version > version {
			break
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			m.version, m.name, appliedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) runMigration(m migration) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := m.up(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func migration001InitialSchema(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  is_admin INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT UNIQUE NOT NULL,
  prefix TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  last_used_at TEXT,
  revoked_at TEXT
)`,
		`CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  platform TEXT NOT NULL DEFAULT '',
  plugin_version TEXT NOT NULL DEFAULT '',
  last_seen_at TEXT NOT NULL,
  last_sync_at TEXT,
  sync_count INTEGER NOT NULL DEFAULT 0,
  files_synced INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS files (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  hash TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0,
  mtime INTEGER NOT NULL DEFAULT 0,
  deleted INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (user_id, path)
)`,
		`CREATE TABLE IF NOT EXISTS conflicts (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  local_hash TEXT NOT NULL,
  remote_hash TEXT NOT NULL,
  local_content BLOB,
  remote_content BLOB,
  device_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  resolved_at TEXT,
  resolution TEXT NOT NULL DEFAULT ''
)`,
		`CREATE TABLE IF NOT EXISTS activity (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  path TEXT NOT NULL DEFAULT '',
  detail TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS github_config (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  token TEXT NOT NULL,
  repo TEXT NOT NULL,
  branch TEXT NOT NULL DEFAULT 'main',
  last_push TEXT,
  last_pull TEXT,
  last_error TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS mcp_permissions (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  search INTEGER NOT NULL DEFAULT 1,
  read INTEGER NOT NULL DEFAULT 1,
  allow_create INTEGER NOT NULL DEFAULT 0,
  allow_modify INTEGER NOT NULL DEFAULT 0
)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_user ON activity(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tokens_user ON tokens(user_id)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
