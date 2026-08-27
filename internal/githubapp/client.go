package githubapp

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const apiVersion = "2022-11-28"

// Manifest is posted to GitHub when a user creates a GitHub App from this instance.
type Manifest struct {
	Name                  string            `json:"name"`
	URL                   string            `json:"url"`
	HookAttributes        map[string]any    `json:"hook_attributes"`
	RedirectURL           string            `json:"redirect_url"`
	CallbackURLs          []string          `json:"callback_urls"`
	SetupURL              string            `json:"setup_url"`
	Description           string            `json:"description"`
	Public                bool              `json:"public"`
	DefaultEvents         []string          `json:"default_events"`
	DefaultPermissions    map[string]string `json:"default_permissions"`
	RequestOAuthOnInstall bool              `json:"request_oauth_on_install"`
}

// AppCredentials is returned after converting a GitHub App manifest code.
type AppCredentials struct {
	ID           int64  `json:"id"`
	Slug         string `json:"slug"`
	PEM          string `json:"pem"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	HTMLURL      string `json:"html_url"`
}

type Repo struct {
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
}

func NewManifest(base, name string) Manifest {
	base = strings.TrimRight(base, "/")
	urls := AppURLs(base)
	return Manifest{
		Name: name,
		URL:  urls.Homepage,
		HookAttributes: map[string]any{
			"url":    urls.Webhook,
			"active": true,
		},
		RedirectURL: urls.ManifestCallback,
		CallbackURLs: []string{
			urls.Callback,
		},
		SetupURL:      urls.Setup,
		Description:   "Syncidian vault backup. Contents: read and write. Single branch: main.",
		Public:        false,
		DefaultEvents: []string{"push"},
		DefaultPermissions: map[string]string{
			"contents":        "write",
			"metadata":        "read",
			"email_addresses": "read",
		},
		RequestOAuthOnInstall: true,
	}
}

// URLs are the GitHub App endpoints this Syncidian instance exposes.
type URLs struct {
	Homepage         string `json:"homepage"`
	Callback         string `json:"callback"`
	Setup            string `json:"setup"`
	Webhook          string `json:"webhook"`
	ManifestCallback string `json:"manifest_callback"`
}

func AppURLs(base string) URLs {
	base = strings.TrimRight(base, "/")
	return URLs{
		Homepage:         base,
		Callback:         base + "/api/v1/auth/github/callback",
		Setup:            base + "/api/v1/github/app/setup",
		Webhook:          base + "/api/v1/github/app/webhook",
		ManifestCallback: base + "/api/v1/github/app/callback",
	}
}

// NormalizeAppSlug accepts a bare slug or a pasted GitHub App URL and returns
// the path segment used in https://github.com/apps/<slug>/…
// Examples: "syncidian", "https://github.com/apps/syncidian",
// "github.com/apps/syncidian/installations/new" → "syncidian".
func NormalizeAppSlug(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	for _, prefix := range []string{
		"https://github.com/apps/",
		"http://github.com/apps/",
		"https://www.github.com/apps/",
		"http://www.github.com/apps/",
		"github.com/apps/",
		"www.github.com/apps/",
		"/apps/",
	} {
		if strings.HasPrefix(lower, prefix) {
			s = s[len(prefix):]
			lower = strings.ToLower(s)
			break
		}
	}
	// Also accept "https://github.com/apps" with no trailing path.
	for _, exact := range []string{
		"https://github.com/apps",
		"http://github.com/apps",
		"https://www.github.com/apps",
		"http://www.github.com/apps",
		"github.com/apps",
		"www.github.com/apps",
		"/apps",
	} {
		if lower == exact {
			return ""
		}
	}
	s = strings.Trim(s, "/")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '?'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}
	// Reject leftovers that still look like a URL scheme (bad paste).
	if strings.Contains(s, ":") {
		return ""
	}
	return strings.TrimSpace(s)
}

// InstallURL is the GitHub page where a user installs this App on a repository.
// Pass a non-empty state so GitHub returns it on the OAuth callback after
// "Install & Authorize" (Request user authorization during installation).
// Without that, Syncidian cannot correlate the redirect and never records the install.
func InstallURL(slug, state string) string {
	slug = NormalizeAppSlug(slug)
	if slug == "" {
		return ""
	}
	u := "https://github.com/apps/" + slug + "/installations/new"
	if state = strings.TrimSpace(state); state != "" {
		u += "?state=" + url.QueryEscape(state)
	}
	return u
}

func ConvertManifest(code string) (*AppCredentials, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("missing GitHub App manifest code")
	}
	resp, err := do(http.MethodPost, "https://api.github.com/app-manifests/"+code+"/conversions", "", nil)
	if err != nil {
		return nil, err
	}
	var creds AppCredentials
	if err := json.Unmarshal(resp, &creds); err != nil {
		return nil, err
	}
	if creds.ID == 0 || creds.PEM == "" {
		return nil, fmt.Errorf("GitHub App conversion returned incomplete credentials")
	}
	creds.Slug = NormalizeAppSlug(creds.Slug)
	if creds.Slug == "" {
		creds.Slug = NormalizeAppSlug(creds.HTMLURL)
	}
	return &creds, nil
}

func InstallationToken(appID int64, pemData []byte, installationID int64) (token string, expires time.Time, err error) {
	jwt, err := SignJWT(appID, pemData)
	if err != nil {
		return "", time.Time{}, err
	}
	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	body, err := do(http.MethodPost, url, "Bearer "+jwt, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	var out struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", time.Time{}, err
	}
	if out.Token == "" {
		return "", time.Time{}, fmt.Errorf("GitHub did not return an installation token")
	}
	exp, err := time.Parse(time.RFC3339, out.ExpiresAt)
	if err != nil {
		exp = time.Now().Add(50 * time.Minute)
	}
	return out.Token, exp, nil
}

func ListRepos(installToken string) ([]Repo, error) {
	body, err := do(http.MethodGet, "https://api.github.com/installation/repositories?per_page=100", "Bearer "+installToken, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Repositories []Repo `json:"repositories"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Repositories == nil {
		out.Repositories = []Repo{}
	}
	return out.Repositories, nil
}

// Installation is a GitHub App installation on an account.
type Installation struct {
	ID      int64 `json:"id"`
	AppID   int64 `json:"app_id"`
	Account struct {
		Login string `json:"login"`
	} `json:"account"`
}

// GetInstallation returns an installation that belongs to this App (JWT auth).
// Used to confirm an installation_id from a callback before binding it to a user.
func GetInstallation(appID int64, pemData []byte, installationID int64) (*Installation, error) {
	jwt, err := SignJWT(appID, pemData)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.github.com/app/installations/%d", installationID)
	body, err := do(http.MethodGet, url, "Bearer "+jwt, nil)
	if err != nil {
		return nil, err
	}
	var inst Installation
	if err := json.Unmarshal(body, &inst); err != nil {
		return nil, err
	}
	if inst.ID == 0 {
		return nil, fmt.Errorf("GitHub installation not found")
	}
	return &inst, nil
}

// ListUserInstallations lists App installations the user can access (user OAuth token).
func ListUserInstallations(userToken string) ([]Installation, error) {
	body, err := do(http.MethodGet, "https://api.github.com/user/installations?per_page=100", "Bearer "+userToken, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Installations []Installation `json:"installations"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Installations == nil {
		out.Installations = []Installation{}
	}
	return out.Installations, nil
}

// FindAppInstallation returns the installation for appID from a user token, if any.
func FindAppInstallation(userToken string, appID int64) (int64, error) {
	list, err := ListUserInstallations(userToken)
	if err != nil {
		return 0, err
	}
	var match int64
	for _, inst := range list {
		if inst.AppID == appID {
			if match != 0 {
				// Multiple installs of this app (e.g. personal + org); caller should use installation_id.
				return 0, nil
			}
			match = inst.ID
		}
	}
	return match, nil
}

// SignJWT builds a GitHub App RS256 JWT. iss is the numeric App ID.
func SignJWT(appID int64, pemData []byte) (string, error) {
	key, err := parseRSAKey(pemData)
	if err != nil {
		return "", err
	}
	now := time.Now().Add(-60 * time.Second)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"iat":%d,"exp":%d,"iss":%d}`, now.Unix(), now.Add(9*time.Minute).Unix(), appID,
	)))
	signing := header + "." + payload
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseRSAKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("invalid GitHub App private key PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub App private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("GitHub App private key is not RSA")
	}
	return key, nil
}

func do(method, url, auth string, payload any) ([]byte, error) {
	var rdr io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", "Syncidian (+https://github.com/shangeethsivan/Syncidian)")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		var parsed struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &parsed)
		if parsed.Message != "" {
			msg = parsed.Message
		}
		return nil, fmt.Errorf("GitHub API %s: %s", resp.Status, msg)
	}
	return body, nil
}
