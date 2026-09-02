package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shangeethsivan/Syncidian/internal/config"
	"github.com/shangeethsivan/Syncidian/internal/store"
)

func TestAuthLimiterBlocksAfterMax(t *testing.T) {
	l := newAuthLimiter(authLimits{PasswordMax: 3, PasswordWindow: time.Minute, MaxKeys: 32})
	key := "password:ip:1.2.3.4"
	for i := 0; i < 3; i++ {
		if _, ok := l.blocked(key, 3, time.Minute); ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		l.add(key, time.Minute)
	}
	retry, ok := l.blocked(key, 3, time.Minute)
	if !ok {
		t.Fatal("expected lockout after 3 failures")
	}
	if retry <= 0 || retry > time.Minute {
		t.Fatalf("retry %s", retry)
	}
	l.clear(key)
	if _, ok := l.blocked(key, 3, time.Minute); ok {
		t.Fatal("clear should lift lockout")
	}
}

func TestAuthLimiterExpires(t *testing.T) {
	l := newAuthLimiter(authLimits{PasswordMax: 2, PasswordWindow: 20 * time.Millisecond, MaxKeys: 8})
	key := "k"
	l.add(key, 20*time.Millisecond)
	l.add(key, 20*time.Millisecond)
	if _, ok := l.blocked(key, 2, 20*time.Millisecond); !ok {
		t.Fatal("should be blocked")
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := l.blocked(key, 2, 20*time.Millisecond); ok {
		t.Fatal("window should have expired")
	}
}

func TestClientIPPrefersForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", " 203.0.113.9, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Fatalf("clientIP=%q", got)
	}
}

func TestLoginRateLimit(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "http://localhost"}, st, nil)
	srv.authLimit = newAuthLimiter(authLimits{
		PasswordMax:    3,
		PasswordWindow: time.Minute,
		SignupMax:      20,
		SignupWindow:   time.Minute,
		SetupMax:       20,
		SetupWindow:    time.Minute,
	})
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()

	admin := setupAdmin(t, hs)
	_ = createAndLoginUser(t, hs, admin, "bob")

	for i := 0; i < 3; i++ {
		res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/login", map[string]string{
			"username": "bob", "password": "wrong-password",
		}, nil, "")
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("fail %d: want 401, got %d %v", i+1, res.StatusCode, m)
		}
	}
	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/login", map[string]string{
		"username": "bob", "password": "wrong-password",
	}, nil, "")
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d %v", res.StatusCode, m)
	}
	if res.Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/login", map[string]string{
		"username": "bob", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("correct password still locked: %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/mcp/login", map[string]string{
		"username": "bob", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("mcp login should share the password budget: %d %v", res.StatusCode, m)
	}
}

func TestSetupRateLimit(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{Addr: ":0", DataDir: dir, PublicURL: "http://localhost"}, st, nil)
	srv.authLimit = newAuthLimiter(authLimits{
		PasswordMax:    50,
		PasswordWindow: time.Minute,
		SignupMax:      2,
		SignupWindow:   time.Minute,
		SetupMax:       2,
		SetupWindow:    time.Minute,
	})
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()

	res, m := doJSON(t, http.MethodPost, hs.URL+"/api/v1/setup", map[string]string{
		"username": "ada", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("first setup: %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/setup", map[string]string{
		"username": "eve", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("second setup: want 409, got %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/setup", map[string]string{
		"username": "mallory", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("third setup: want 429, got %d %v", res.StatusCode, m)
	}

	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/signup", map[string]any{
		"username": "cara", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("first signup: %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/signup", map[string]any{
		"username": "dan", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("second signup: %d %v", res.StatusCode, m)
	}
	res, m = doJSON(t, http.MethodPost, hs.URL+"/api/v1/auth/signup", map[string]any{
		"username": "erin", "password": "password1",
	}, nil, "")
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("third signup: want 429, got %d %v", res.StatusCode, m)
	}
}
