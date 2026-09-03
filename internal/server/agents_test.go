package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/shangeethsivan/Syncidian/internal/web"
)

func getBody(t *testing.T, url string, accept string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return res, raw
}

func TestAgentDiscovery(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	res, body := getBody(t, hs.URL+"/robots.txt", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("robots.txt %d %s", res.StatusCode, body)
	}
	txt := string(body)
	for _, needle := range []string{"User-agent: *", "User-agent: GPTBot", "Content-Signal:", "Sitemap:", "Agentmap:", "Disallow: /admin", "Disallow: /api/v1/setup", "Disallow: /api/v1/users"} {
		if !strings.Contains(txt, needle) {
			t.Fatalf("robots.txt missing %q:\n%s", needle, txt)
		}
	}

	res, body = getBody(t, hs.URL+"/sitemap.xml", "")
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "/auth.md") {
		t.Fatalf("sitemap %d %s", res.StatusCode, body)
	}
	if strings.Contains(string(body), "/admin") {
		t.Fatal("sitemap must not advertise the operator page")
	}

	res, body = getBody(t, hs.URL+"/", "text/markdown")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("markdown / %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/markdown") {
		t.Fatalf("markdown content-type %q", ct)
	}
	if !strings.Contains(string(body), "MCP JSON-RPC") {
		t.Fatalf("landing markdown: %s", body)
	}
	if res.Header.Get("Link") == "" {
		t.Fatal("markdown response missing Link")
	}

	res, body = getBody(t, hs.URL+"/", "text/html")
	if res.StatusCode != http.StatusOK || !strings.Contains(res.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("html / %d %s %s", res.StatusCode, res.Header.Get("Content-Type"), body[:min(80, len(body))])
	}
	if !strings.Contains(res.Header.Get("Link"), `rel="api-catalog"`) {
		t.Fatalf("html Link %q", res.Header.Get("Link"))
	}

	checks := []struct {
		path string
		want string
	}{
		{"/auth.md", "Bearer token"},
		{"/.well-known/api-catalog", "linkset"},
		{"/.well-known/oauth-protected-resource", "bearer_methods_supported"},
		{"/.well-known/oauth-authorization-server", "token_endpoint"},
		{"/.well-known/openid-configuration", "issuer"},
		{"/.well-known/mcp/server-card.json", "streamable-http"},
		{"/.well-known/mcp/server-card.json", "/assets/syncidian.png"},
		{"/.well-known/mcp.json", "syncidian"},
		{"/.well-known/agent-skills/index.json", "syncidian-mcp"},
		{"/.well-known/skills/index.json", "$schema"},
		{"/.well-known/ai-catalog.json", "specVersion"},
		{"/openapi.json", "openapi"},
		{"/.well-known/agent-skills/syncidian-mcp/SKILL.md", "name: syncidian-mcp"},
	}
	for _, c := range checks {
		res, body = getBody(t, hs.URL+c.path, "")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s status %d %s", c.path, res.StatusCode, body)
		}
		if !strings.Contains(string(body), c.want) {
			t.Fatalf("%s missing %q: %s", c.path, c.want, body)
		}
		if res.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("%s missing CORS *", c.path)
		}
	}

	res, body = getBody(t, hs.URL+"/.well-known/agent-skills/index.json", "")
	var idx struct {
		Skills []struct {
			SHA256 string `json:"sha256"`
			Digest string `json:"digest"`
			Type   string `json:"type"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(body, &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx.Skills) != 1 || idx.Skills[0].Type != "skill-md" {
		t.Fatalf("skills index: %+v", idx)
	}
	raw, err := web.FS.ReadFile("static/agents/syncidian-mcp/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	want := hex.EncodeToString(sum[:])
	if idx.Skills[0].SHA256 != want || idx.Skills[0].Digest != "sha256:"+want {
		t.Fatalf("digest mismatch got %s want %s", idx.Skills[0].Digest, want)
	}
}

func TestWantsMarkdown(t *testing.T) {
	if !wantsMarkdown("text/markdown") {
		t.Fatal("plain markdown")
	}
	if wantsMarkdown("text/html,application/xhtml+xml") {
		t.Fatal("browser html")
	}
	if !wantsMarkdown("text/html;q=0.8, text/markdown;q=1") {
		t.Fatal("markdown preferred")
	}
	if wantsMarkdown("") {
		t.Fatal("empty")
	}
}
