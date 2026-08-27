package store

import "time"

type User struct {
	ID           string
	Username     string
	PasswordHash string
	IsAdmin      bool
	Email        string
	GitHubID     int64
	CreatedAt    time.Time
}

type Token struct {
	ID         string
	UserID     string
	TokenHash  string
	Prefix     string
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Device struct {
	ID            string
	UserID        string
	Name          string
	Platform      string
	PluginVersion string
	LastSeenAt    time.Time
	LastSyncAt    *time.Time
	SyncCount     int
	FilesSynced   int
	CreatedAt     time.Time
}

type FileMeta struct {
	UserID    string
	Path      string
	Hash      string
	Size      int64
	Mtime     int64
	Deleted   bool
	UpdatedAt time.Time
}

type Conflict struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	Path          string     `json:"path"`
	LocalHash     string     `json:"local_hash"`
	RemoteHash    string     `json:"remote_hash"`
	LocalContent  []byte     `json:"-"`
	RemoteContent []byte     `json:"-"`
	DeviceID      string     `json:"device_id"`
	CreatedAt     time.Time  `json:"created_at"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	Resolution    string     `json:"resolution"`
}

type Activity struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	DeviceID  string    `json:"device_id"`
	Action    string    `json:"action"`
	Path      string    `json:"path"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

type GitHubConfig struct {
	UserID              string
	Token               string // cached GitHub App installation token, never a user PAT
	Repo                string
	Branch              string
	AppID               int64
	AppSlug             string
	AppPEM              string
	ClientID            string
	ClientSecret        string
	InstallationID      int64
	InstallTokenExpires time.Time
	LastPush            *time.Time
	LastPull            *time.Time
	LastError           string
	UpdatedAt           time.Time
}

func (c *GitHubConfig) HasApp() bool {
	return c != nil && c.AppID != 0 && c.AppPEM != ""
}

func (c *GitHubConfig) Configured() bool {
	return c.HasApp() && c.InstallationID != 0 && c.Repo != ""
}

type GitHubApp struct {
	AppID        int64
	Slug         string
	PEM          string
	ClientID     string
	ClientSecret string
	UpdatedAt    time.Time
}

func (a *GitHubApp) Configured() bool {
	return a != nil && a.AppID != 0 && a.PEM != "" && a.ClientID != "" && a.ClientSecret != ""
}

type MCPPermissions struct {
	UserID string `json:"user_id"`
	Search bool   `json:"search"`
	Read   bool   `json:"read"`
	Create bool   `json:"create"`
	Modify bool   `json:"modify"`
}

type MCPEvent struct {
	UserID      string
	Name        string
	Version     string
	UserAgent   string
	TokenID     string
	TokenName   string
	TokenPrefix string
	Method      string
	Tool        string
}

type MCPClient struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	ClientKey       string    `json:"client_key"`
	Name            string    `json:"name"`
	Version         string    `json:"version"`
	UserAgent       string    `json:"user_agent"`
	TokenName       string    `json:"token_name"`
	TokenPrefix     string    `json:"token_prefix"`
	FirstSeenAt     time.Time `json:"first_seen_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	InitializeCount int       `json:"initialize_count"`
	CallCount       int       `json:"call_count"`
	LastTool        string    `json:"last_tool"`
	Status          string    `json:"status"`
}

type MCPToolStat struct {
	Tool         string     `json:"tool"`
	CallCount    int        `json:"call_count"`
	LastCalledAt *time.Time `json:"last_called_at,omitempty"`
}

type MCPUsage struct {
	ClientCount   int           `json:"client_count"`
	ActiveClients int           `json:"active_clients"`
	TotalCalls    int           `json:"total_calls"`
	Calls24h      int           `json:"calls_24h"`
	Calls7d       int           `json:"calls_7d"`
	LastCallAt    *time.Time    `json:"last_call_at,omitempty"`
	Clients       []MCPClient   `json:"clients"`
	Tools         []MCPToolStat `json:"tools"`
}

type Stats struct {
	TotalSyncs     int        `json:"total_syncs"`
	FilesSynced    int        `json:"files_synced"`
	ActiveClients  int        `json:"active_clients"`
	DeviceCount    int        `json:"device_count"`
	ConflictCount  int        `json:"conflict_count"`
	LastSync       *time.Time `json:"last_sync"`
	MCPClientCount int        `json:"mcp_client_count"`
	MCPActive      int        `json:"mcp_active_clients"`
	MCPTotalCalls  int        `json:"mcp_total_calls"`
	MCPCalls7d     int        `json:"mcp_calls_7d"`
	LastMCPCall    *time.Time `json:"last_mcp_call"`
}
