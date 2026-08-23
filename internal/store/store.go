package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	DataDir string
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataDir, "syncidian.db")
	dsn := "file:" + filepath.ToSlash(dbPath) + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, DataDir: dataDir}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
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
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func parseTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, v)
	}
	return t.UTC()
}

func parseTimePtr(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	t := parseTime(v.String)
	return &t
}

func (s *Store) UserCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(username, passwordHash string, admin bool) (*User, error) {
	u := &User{
		ID:           NewID(),
		Username:     strings.TrimSpace(username),
		PasswordHash: passwordHash,
		IsAdmin:      admin,
		CreatedAt:    time.Now().UTC(),
	}
	if u.Username == "" {
		return nil, fmt.Errorf("username is required")
	}
	_, err := s.db.Exec(
		`INSERT INTO users (id, username, password_hash, is_admin, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, boolToInt(admin), now(),
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("username already exists")
		}
		return nil, err
	}
	_, _ = s.db.Exec(
		`INSERT INTO mcp_permissions (user_id, search, read, allow_create, allow_modify) VALUES (?, 1, 1, 0, 0)`,
		u.ID,
	)
	return u, nil
}

func (s *Store) GetUser(id string) (*User, error) {
	return s.scanUser(s.db.QueryRow(`SELECT id, username, password_hash, is_admin, created_at FROM users WHERE id = ?`, id))
}

func (s *Store) GetUserByUsername(username string) (*User, error) {
	return s.scanUser(s.db.QueryRow(`SELECT id, username, password_hash, is_admin, created_at FROM users WHERE username = ?`, username))
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, username, password_hash, is_admin, created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// ListUsersPublic returns account records without password hashes. Used by admin listing.
func (s *Store) ListUsersPublic() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, username, is_admin, created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u := User{}
		var admin int
		var created string
		if err := rows.Scan(&u.ID, &u.Username, &admin, &created); err != nil {
			return nil, err
		}
		u.IsAdmin = admin == 1
		u.CreatedAt = parseTime(created)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) scanUser(row *sql.Row) (*User, error) {
	u := &User{}
	var admin int
	var created string
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &admin, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.IsAdmin = admin == 1
	u.CreatedAt = parseTime(created)
	return u, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUserRow(row rowScanner) (*User, error) {
	u := &User{}
	var admin int
	var created string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &admin, &created); err != nil {
		return nil, err
	}
	u.IsAdmin = admin == 1
	u.CreatedAt = parseTime(created)
	return u, nil
}

func (s *Store) CreateToken(userID, name, rawToken, prefix string) (*Token, error) {
	t := &Token{
		ID:        NewID(),
		UserID:    userID,
		TokenHash: HashToken(rawToken),
		Prefix:    prefix,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.Exec(
		`INSERT INTO tokens (id, user_id, token_hash, prefix, name, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.UserID, t.TokenHash, t.Prefix, t.Name, now(),
	)
	return t, err
}

func (s *Store) GetTokenByHash(hash string) (*Token, error) {
	t := &Token{}
	var created string
	var last, revoked sql.NullString
	err := s.db.QueryRow(
		`SELECT id, user_id, token_hash, prefix, name, created_at, last_used_at, revoked_at FROM tokens WHERE token_hash = ?`,
		hash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.Prefix, &t.Name, &created, &last, &revoked)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt = parseTime(created)
	t.LastUsedAt = parseTimePtr(last)
	t.RevokedAt = parseTimePtr(revoked)
	return t, nil
}

func (s *Store) TouchToken(id string) {
	_, _ = s.db.Exec(`UPDATE tokens SET last_used_at = ? WHERE id = ?`, now(), id)
}

func (s *Store) ListTokens(userID string) ([]Token, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, token_hash, prefix, name, created_at, last_used_at, revoked_at FROM tokens WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Token
	for rows.Next() {
		t := Token{}
		var created string
		var last, revoked sql.NullString
		if err := rows.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.Prefix, &t.Name, &created, &last, &revoked); err != nil {
			return nil, err
		}
		t.CreatedAt = parseTime(created)
		t.LastUsedAt = parseTimePtr(last)
		t.RevokedAt = parseTimePtr(revoked)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeToken(userID, tokenID string) error {
	res, err := s.db.Exec(`UPDATE tokens SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, now(), tokenID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("token not found")
	}
	return nil
}

func (s *Store) CreateSession(userID string, ttl time.Duration) (*Session, error) {
	sess := &Session{
		ID:        NewID() + NewID(),
		UserID:    userID,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(ttl),
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		sess.ID, sess.UserID, now(), sess.ExpiresAt.Format(time.RFC3339Nano),
	)
	return sess, err
}

func (s *Store) GetSession(id string) (*Session, error) {
	sess := &Session{}
	var created, expires string
	err := s.db.QueryRow(`SELECT id, user_id, created_at, expires_at FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &created, &expires)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess.CreatedAt = parseTime(created)
	sess.ExpiresAt = parseTime(expires)
	if time.Now().UTC().After(sess.ExpiresAt) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
		return nil, nil
	}
	return sess, nil
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) UpsertDevice(d *Device) error {
	nowS := now()
	_, err := s.db.Exec(`
INSERT INTO devices (id, user_id, name, platform, plugin_version, last_seen_at, last_sync_at, sync_count, files_synced, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  platform = excluded.platform,
  plugin_version = excluded.plugin_version,
  last_seen_at = excluded.last_seen_at
`, d.ID, d.UserID, d.Name, d.Platform, d.PluginVersion, nowS, nil, d.SyncCount, d.FilesSynced, nowS)
	d.LastSeenAt = time.Now().UTC()
	return err
}

func (s *Store) GetDevice(id string) (*Device, error) {
	d := &Device{}
	var lastSeen, created string
	var lastSync sql.NullString
	err := s.db.QueryRow(
		`SELECT id, user_id, name, platform, plugin_version, last_seen_at, last_sync_at, sync_count, files_synced, created_at FROM devices WHERE id = ?`,
		id,
	).Scan(&d.ID, &d.UserID, &d.Name, &d.Platform, &d.PluginVersion, &lastSeen, &lastSync, &d.SyncCount, &d.FilesSynced, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.LastSeenAt = parseTime(lastSeen)
	d.LastSyncAt = parseTimePtr(lastSync)
	d.CreatedAt = parseTime(created)
	return d, nil
}

func (s *Store) ListDevices(userID string) ([]Device, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, name, platform, plugin_version, last_seen_at, last_sync_at, sync_count, files_synced, created_at FROM devices WHERE user_id = ? ORDER BY last_seen_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		d := Device{}
		var lastSeen, created string
		var lastSync sql.NullString
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.Platform, &d.PluginVersion, &lastSeen, &lastSync, &d.SyncCount, &d.FilesSynced, &created); err != nil {
			return nil, err
		}
		d.LastSeenAt = parseTime(lastSeen)
		d.LastSyncAt = parseTimePtr(lastSync)
		d.CreatedAt = parseTime(created)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) TouchDevice(id string, files int) error {
	_, err := s.db.Exec(
		`UPDATE devices SET last_seen_at = ?, last_sync_at = ?, sync_count = sync_count + 1, files_synced = files_synced + ? WHERE id = ?`,
		now(), now(), files, id,
	)
	return err
}

func (s *Store) DeleteDevice(userID, deviceID string) error {
	_, err := s.db.Exec(`DELETE FROM devices WHERE id = ? AND user_id = ?`, deviceID, userID)
	return err
}

func (s *Store) UpsertFile(f FileMeta) error {
	_, err := s.db.Exec(`
INSERT INTO files (user_id, path, hash, size, mtime, deleted, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, path) DO UPDATE SET
  hash = excluded.hash,
  size = excluded.size,
  mtime = excluded.mtime,
  deleted = excluded.deleted,
  updated_at = excluded.updated_at
`, f.UserID, f.Path, f.Hash, f.Size, f.Mtime, boolToInt(f.Deleted), now())
	return err
}

func (s *Store) GetFile(userID, path string) (*FileMeta, error) {
	f := &FileMeta{}
	var deleted int
	var updated string
	err := s.db.QueryRow(
		`SELECT user_id, path, hash, size, mtime, deleted, updated_at FROM files WHERE user_id = ? AND path = ?`,
		userID, path,
	).Scan(&f.UserID, &f.Path, &f.Hash, &f.Size, &f.Mtime, &deleted, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f.Deleted = deleted == 1
	f.UpdatedAt = parseTime(updated)
	return f, nil
}

func (s *Store) ListFiles(userID string, includeDeleted bool) ([]FileMeta, error) {
	q := `SELECT user_id, path, hash, size, mtime, deleted, updated_at FROM files WHERE user_id = ?`
	if !includeDeleted {
		q += ` AND deleted = 0`
	}
	q += ` ORDER BY path`
	rows, err := s.db.Query(q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileMeta
	for rows.Next() {
		f := FileMeta{}
		var deleted int
		var updated string
		if err := rows.Scan(&f.UserID, &f.Path, &f.Hash, &f.Size, &f.Mtime, &deleted, &updated); err != nil {
			return nil, err
		}
		f.Deleted = deleted == 1
		f.UpdatedAt = parseTime(updated)
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) CreateConflict(c *Conflict) error {
	c.ID = NewID()
	c.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO conflicts (id, user_id, path, local_hash, remote_hash, local_content, remote_content, device_id, created_at, resolution) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
		c.ID, c.UserID, c.Path, c.LocalHash, c.RemoteHash, c.LocalContent, c.RemoteContent, c.DeviceID, now(),
	)
	return err
}

func (s *Store) GetConflict(id string) (*Conflict, error) {
	c := &Conflict{}
	var created string
	var resolved sql.NullString
	err := s.db.QueryRow(
		`SELECT id, user_id, path, local_hash, remote_hash, local_content, remote_content, device_id, created_at, resolved_at, resolution FROM conflicts WHERE id = ?`,
		id,
	).Scan(&c.ID, &c.UserID, &c.Path, &c.LocalHash, &c.RemoteHash, &c.LocalContent, &c.RemoteContent, &c.DeviceID, &created, &resolved, &c.Resolution)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt = parseTime(created)
	c.ResolvedAt = parseTimePtr(resolved)
	return c, nil
}

func (s *Store) ListConflicts(userID string, openOnly bool) ([]Conflict, error) {
	q := `SELECT id, user_id, path, local_hash, remote_hash, device_id, created_at, resolved_at, resolution FROM conflicts WHERE user_id = ?`
	if openOnly {
		q += ` AND resolved_at IS NULL`
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.Query(q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Conflict, 0)
	for rows.Next() {
		c := Conflict{}
		var created string
		var resolved sql.NullString
		if err := rows.Scan(&c.ID, &c.UserID, &c.Path, &c.LocalHash, &c.RemoteHash, &c.DeviceID, &created, &resolved, &c.Resolution); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(created)
		c.ResolvedAt = parseTimePtr(resolved)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ResolveConflict(id, resolution string) error {
	_, err := s.db.Exec(`UPDATE conflicts SET resolved_at = ?, resolution = ? WHERE id = ?`, now(), resolution, id)
	return err
}

func (s *Store) AddActivity(a Activity) error {
	if a.ID == "" {
		a.ID = NewID()
	}
	_, err := s.db.Exec(
		`INSERT INTO activity (id, user_id, device_id, action, path, detail, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.UserID, a.DeviceID, a.Action, a.Path, a.Detail, now(),
	)
	return err
}

func (s *Store) ListActivity(userID string, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, user_id, device_id, action, path, detail, created_at FROM activity WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Activity, 0)
	for rows.Next() {
		a := Activity{}
		var created string
		if err := rows.Scan(&a.ID, &a.UserID, &a.DeviceID, &a.Action, &a.Path, &a.Detail, &created); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTime(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) SetGitHub(cfg GitHubConfig) error {
	_, err := s.db.Exec(`
INSERT INTO github_config (user_id, token, repo, branch, last_push, last_pull, last_error, updated_at)
VALUES (?, ?, ?, ?, NULL, NULL, '', ?)
ON CONFLICT(user_id) DO UPDATE SET
  token = excluded.token,
  repo = excluded.repo,
  branch = excluded.branch,
  last_error = '',
  updated_at = excluded.updated_at
`, cfg.UserID, cfg.Token, cfg.Repo, cfg.Branch, now())
	return err
}

func (s *Store) GetGitHub(userID string) (*GitHubConfig, error) {
	c := &GitHubConfig{}
	var lastPush, lastPull sql.NullString
	var updated string
	err := s.db.QueryRow(
		`SELECT user_id, token, repo, branch, last_push, last_pull, last_error, updated_at FROM github_config WHERE user_id = ?`,
		userID,
	).Scan(&c.UserID, &c.Token, &c.Repo, &c.Branch, &lastPush, &lastPull, &c.LastError, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.LastPush = parseTimePtr(lastPush)
	c.LastPull = parseTimePtr(lastPull)
	c.UpdatedAt = parseTime(updated)
	return c, nil
}

func (s *Store) DeleteGitHub(userID string) error {
	_, err := s.db.Exec(`DELETE FROM github_config WHERE user_id = ?`, userID)
	return err
}

func (s *Store) UpdateGitHubStatus(userID string, push, pull bool, lastErr string) error {
	sets := []string{`last_error = ?`, `updated_at = ?`}
	args := []any{lastErr, now()}
	if push {
		sets = append([]string{`last_push = ?`}, sets...)
		args = append([]any{now()}, args...)
	}
	if pull {
		sets = append([]string{`last_pull = ?`}, sets...)
		args = append([]any{now()}, args...)
	}
	args = append(args, userID)
	_, err := s.db.Exec(`UPDATE github_config SET `+strings.Join(sets, ", ")+` WHERE user_id = ?`, args...)
	return err
}

func (s *Store) GetMCP(userID string) (*MCPPermissions, error) {
	p := &MCPPermissions{UserID: userID, Search: true, Read: true}
	var search, read, create, modify int
	err := s.db.QueryRow(
		`SELECT search, read, allow_create, allow_modify FROM mcp_permissions WHERE user_id = ?`, userID,
	).Scan(&search, &read, &create, &modify)
	if err == sql.ErrNoRows {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	p.Search = search == 1
	p.Read = read == 1
	p.Create = create == 1
	p.Modify = modify == 1
	return p, nil
}

func (s *Store) SetMCP(p MCPPermissions) error {
	_, err := s.db.Exec(`
INSERT INTO mcp_permissions (user_id, search, read, allow_create, allow_modify) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET search=excluded.search, read=excluded.read, allow_create=excluded.allow_create, allow_modify=excluded.allow_modify
`, p.UserID, boolToInt(p.Search), boolToInt(p.Read), boolToInt(p.Create), boolToInt(p.Modify))
	return err
}

func (s *Store) Stats(userID string) (Stats, error) {
	st := Stats{}
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(sync_count),0), COALESCE(SUM(files_synced),0), COUNT(*) FROM devices WHERE user_id = ?`, userID).
		Scan(&st.TotalSyncs, &st.FilesSynced, &st.DeviceCount)
	cutoff := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM devices WHERE user_id = ? AND last_seen_at >= ?`, userID, cutoff).Scan(&st.ActiveClients)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM conflicts WHERE user_id = ? AND resolved_at IS NULL`, userID).Scan(&st.ConflictCount)
	var last sql.NullString
	_ = s.db.QueryRow(`SELECT MAX(last_sync_at) FROM devices WHERE user_id = ?`, userID).Scan(&last)
	st.LastSync = parseTimePtr(last)
	return st, nil
}

func (s *Store) VaultDir(userID string) string {
	return filepath.Join(s.DataDir, "vaults", userID)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
