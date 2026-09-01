package githubapp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type User struct {
	ID     int64    `json:"id"`
	Login  string   `json:"login"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Emails []string `json:"-"`
}

func ExchangeOAuth(clientID, clientSecret, code, redirectURI string) (string, error) {
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}
	req, err := http.NewRequest(http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Syncidian (+https://github.com/shangeethsivan/Syncidian)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		msg := out.Description
		if msg == "" {
			msg = out.Error
		}
		if msg == "" {
			msg = string(body)
		}
		return "", fmt.Errorf("GitHub OAuth: %s", msg)
	}
	return out.AccessToken, nil
}

func GetUser(token string) (*User, error) {
	body, err := do(http.MethodGet, "https://api.github.com/user", "Bearer "+token, nil)
	if err != nil {
		return nil, err
	}
	u := &User{}
	if err := json.Unmarshal(body, u); err != nil {
		return nil, err
	}
	if u.ID == 0 || u.Login == "" {
		return nil, fmt.Errorf("GitHub did not return a user")
	}
	verified := verifiedEmails(token)
	u.Emails = mergeEmails(u.Email, verified)
	if u.Email == "" && len(u.Emails) > 0 {
		u.Email = u.Emails[0]
	}
	return u, nil
}

func verifiedEmails(token string) []string {
	body, err := do(http.MethodGet, "https://api.github.com/user/emails", "Bearer "+token, nil)
	if err != nil {
		return nil
	}
	var list []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil
	}
	var primary, rest []string
	for _, e := range list {
		email := strings.TrimSpace(e.Email)
		if !e.Verified || email == "" {
			continue
		}
		if e.Primary {
			primary = append(primary, email)
		} else {
			rest = append(rest, email)
		}
	}
	return append(primary, rest...)
}

func mergeEmails(profile string, verified []string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(email string) {
		email = strings.TrimSpace(email)
		if email == "" {
			return
		}
		key := strings.ToLower(email)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, email)
	}
	add(profile)
	for _, e := range verified {
		add(e)
	}
	return out
}
