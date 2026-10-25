package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestPricingRoute(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	res, body := getBody(t, hs.URL+"/pricing", "text/html")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /pricing: %d %s", res.StatusCode, body)
	}
	html := string(body)
	if !strings.Contains(html, "$2") {
		t.Fatal("GET /pricing must contain $2")
	}
	if strings.Contains(html, "$20") {
		t.Fatal("GET /pricing must not contain $20")
	}
	if !strings.Contains(html, "14-day trial") {
		t.Fatal("GET /pricing must mention the 14-day trial")
	}
	if !strings.Contains(html, "Self-host is free") && !strings.Contains(html, "self-host stays free") && !strings.Contains(html, "Self-host stays free") {
		t.Fatal("GET /pricing must say self-host is free")
	}

	res, body = getBody(t, hs.URL+"/", "text/html")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /: %d", res.StatusCode)
	}
	home := string(body)
	if !strings.Contains(home, `id="pricing"`) || !strings.Contains(home, `href="/pricing"`) {
		t.Fatal("homepage must include #pricing and a link to /pricing")
	}

	res, body = getBody(t, hs.URL+"/sitemap.xml", "")
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "/pricing") {
		t.Fatalf("sitemap must list /pricing: %d %s", res.StatusCode, body)
	}
}
