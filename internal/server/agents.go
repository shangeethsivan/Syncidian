package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/shangeethsivan/Syncidian/internal/web"
)

const mcpProtocolVersion = "2024-11-05"
const mcpServerVersion = "0.2.0"

const aiBots = "GPTBot, ChatGPT-User, OAI-SearchBot, ClaudeBot, Claude-Web, Claude-User, Google-Extended, Google-CloudVertexBot, Applebot-Extended, PerplexityBot, Bytespider, Amazonbot, CCBot, meta-externalagent, FacebookBot"

func (s *Server) publicOrigin(r *http.Request) string {
	if u := strings.TrimRight(s.Cfg.PublicURL, "/"); u != "" {
		return u
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	return scheme + "://" + host
}

func wantsMarkdown(accept string) bool {
	if accept == "" {
		return false
	}
	type qval struct {
		name string
		q    float64
		ok   bool
	}
	html, md := qval{name: "text/html"}, qval{name: "text/markdown"}
	for _, part := range strings.Split(accept, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		media, params, _ := strings.Cut(part, ";")
		media = strings.TrimSpace(media)
		q := 1.0
		for _, p := range strings.Split(params, ";") {
			p = strings.TrimSpace(p)
			if k, v, ok := strings.Cut(p, "="); ok && strings.TrimSpace(k) == "q" {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = f
				}
			}
		}
		switch media {
		case "text/markdown", "text/x-markdown":
			md.ok, md.q = true, q
		case "text/html", "application/xhtml+xml":
			html.ok, html.q = true, q
		}
	}
	if !md.ok {
		return false
	}
	if !html.ok {
		return true
	}
	return md.q >= html.q
}

func (s *Server) discoveryLinkHeader(origin string) string {
	return strings.Join([]string{
		fmt.Sprintf(`<%s/.well-known/api-catalog>; rel="api-catalog"; type="application/linkset+json"`, origin),
		fmt.Sprintf(`<%s/.well-known/oauth-protected-resource>; rel="oauth-protected-resource"`, origin),
		fmt.Sprintf(`<%s/.well-known/mcp/server-card.json>; rel="describedby"; type="application/json"`, origin),
		fmt.Sprintf(`<%s/.well-known/ai-catalog.json>; rel="ai-catalog"; type="application/json"`, origin),
		fmt.Sprintf(`<%s/sitemap.xml>; rel="sitemap"`, origin),
		fmt.Sprintf(`<%s/auth.md>; rel="service-doc"; type="text/markdown"`, origin),
	}, ", ")
}

func allowAgents(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Cache-Control", "public, max-age=300")
}

func writePublic(w http.ResponseWriter, status int, contentType string, body []byte) {
	allowAgents(w)
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	origin := s.publicOrigin(r)
	var b strings.Builder
	b.WriteString("# https://www.rfc-editor.org/rfc/rfc9309.html\n")
	b.WriteString("# Content Signals: https://contentsignals.org\n")
	fmt.Fprintf(&b, "Sitemap: %s/sitemap.xml\n", origin)
	fmt.Fprintf(&b, "Agentmap: %s/.well-known/ai-catalog.json\n\n", origin)
	b.WriteString("User-agent: *\nAllow: /\n")
	b.WriteString("Content-Signal: search=yes, ai-train=yes, ai-input=yes\n\n")
	for _, bot := range strings.Split(aiBots, ", ") {
		fmt.Fprintf(&b, "User-agent: %s\nAllow: /\nContent-Signal: search=yes, ai-train=yes, ai-input=yes\n\n", bot)
	}
	writePublic(w, http.StatusOK, "text/plain; charset=utf-8", []byte(b.String()))
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	origin := s.publicOrigin(r)
	paths := []string{"/", "/admin", "/auth.md", "/openapi.json", "/.well-known/mcp/server-card.json", "/.well-known/api-catalog", "/assets/obsidian.zip"}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "  <url><loc>%s%s</loc></url>\n", origin, p)
	}
	b.WriteString("</urlset>\n")
	writePublic(w, http.StatusOK, "application/xml; charset=utf-8", []byte(b.String()))
}

func (s *Server) handleAuthMD(w http.ResponseWriter, r *http.Request) {
	raw, err := web.FS.ReadFile("static/agents/auth.md")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "auth.md missing")
		return
	}
	body := strings.ReplaceAll(string(raw), "{origin}", s.publicOrigin(r))
	writePublic(w, http.StatusOK, "text/markdown; charset=utf-8", []byte(body))
}

func (s *Server) handleLandingMarkdown(w http.ResponseWriter, r *http.Request) {
	raw, err := web.FS.ReadFile("static/agents/landing.md")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "landing markdown missing")
		return
	}
	words := len(strings.Fields(string(raw)))
	allowAgents(w)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("X-Markdown-Tokens", strconv.Itoa(words))
	w.Header().Set("Vary", "Accept")
	w.Header().Set("Link", s.discoveryLinkHeader(s.publicOrigin(r)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (s *Server) handleAPICatalog(w http.ResponseWriter, r *http.Request) {
	origin := s.publicOrigin(r)
	writeJSONCORS(w, map[string]any{
		"linkset": []map[string]any{
			{
				"anchor": origin + "/api/v1",
				"service-desc": []map[string]any{
					{"href": origin + "/openapi.json", "type": "application/vnd.oai.openapi+json"},
				},
				"service-doc": []map[string]any{
					{"href": origin + "/auth.md", "type": "text/markdown"},
					{"href": origin + "/", "type": "text/html"},
				},
				"status": []map[string]any{
					{"href": origin + "/health", "type": "application/json"},
				},
			},
			{
				"anchor": origin + "/mcp",
				"service-desc": []map[string]any{
					{"href": origin + "/.well-known/mcp/server-card.json", "type": "application/json"},
				},
				"service-doc": []map[string]any{
					{"href": origin + "/auth.md", "type": "text/markdown"},
				},
				"status": []map[string]any{
					{"href": origin + "/health", "type": "application/json"},
				},
			},
		},
	}, "application/linkset+json")
}

func (s *Server) handleOAuthProtectedResource(w http.ResponseWriter, r *http.Request) {
	origin := s.publicOrigin(r)
	writeJSONCORS(w, map[string]any{
		"resource":                 origin,
		"authorization_servers":    []string{origin},
		"bearer_methods_supported": []string{"header"},
		"resource_documentation":   origin + "/auth.md",
		"scopes_supported":         []string{"mcp", "sync"},
	}, "application/json")
}

func (s *Server) handleOAuthAS(w http.ResponseWriter, r *http.Request) {
	origin := s.publicOrigin(r)
	writeJSONCORS(w, map[string]any{
		"issuer":                                origin,
		"authorization_endpoint":                origin + "/api/v1/auth/github/start",
		"token_endpoint":                        origin + "/api/v1/mcp/login",
		"grant_types_supported":                 []string{"password"},
		"response_types_supported":              []string{"code"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"mcp", "sync"},
		"service_documentation":                 origin + "/auth.md",
		"agent_auth": map[string]any{
			"register_uri": origin + "/auth.md",
			"supported_identity_types": []string{
				"bearer_token",
			},
			"credential_types": []string{"sk_sync"},
		},
	}, "application/json")
}

func (s *Server) mcpServerCard(origin string) map[string]any {
	endpoint := origin + "/mcp"
	return map[string]any{
		"$schema":         "https://static.modelcontextprotocol.io/schemas/mcp-server-card/v1.json",
		"version":         "1.0",
		"protocolVersion": mcpProtocolVersion,
		"name":            "io.github.shangeethsivan/syncidian",
		"title":           "Syncidian",
		"description":     "Self-hosted Obsidian sync and MCP bridge. Search and read vault notes; optional GitHub-backed writes.",
		"websiteUrl":      origin,
		"repository": map[string]any{
			"url":    "https://github.com/shangeethsivan/Syncidian",
			"source": "github",
		},
		"serverInfo": map[string]any{
			"name":    "syncidian",
			"title":   "Syncidian",
			"version": mcpServerVersion,
		},
		"transport": map[string]any{
			"type":     "streamable-http",
			"endpoint": endpoint,
		},
		"remotes": []map[string]any{
			{"type": "streamable-http", "url": endpoint},
		},
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"authentication": map[string]any{
			"required": true,
			"schemes":  []string{"bearer"},
		},
		"documentationUrl": origin + "/auth.md",
	}
}

func (s *Server) handleMCPServerCard(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w, s.mcpServerCard(s.publicOrigin(r)), "application/json")
}

func (s *Server) handleMCPServerCards(w http.ResponseWriter, r *http.Request) {
	card := s.mcpServerCard(s.publicOrigin(r))
	writeJSONCORS(w, map[string]any{"servers": []any{card}}, "application/json")
}

func (s *Server) handleAgentSkillsIndex(w http.ResponseWriter, r *http.Request) {
	origin := s.publicOrigin(r)
	raw, err := web.FS.ReadFile("static/agents/syncidian-mcp/SKILL.md")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "skill missing")
		return
	}
	sum := sha256.Sum256(raw)
	hexSum := hex.EncodeToString(sum[:])
	skillURL := origin + "/.well-known/agent-skills/syncidian-mcp/SKILL.md"
	writeJSONCORS(w, map[string]any{
		"$schema": "https://schemas.agentskills.io/discovery/0.2.0/schema.json",
		"skills": []map[string]any{
			{
				"name":        "syncidian-mcp",
				"type":        "skill-md",
				"description": "Connect to this Syncidian instance’s MCP server to search, read, and optionally write Obsidian notes.",
				"url":         skillURL,
				"digest":      "sha256:" + hexSum,
				"sha256":      hexSum,
			},
		},
	}, "application/json")
}

func (s *Server) handleAgentSkillMD(w http.ResponseWriter, r *http.Request) {
	raw, err := web.FS.ReadFile("static/agents/syncidian-mcp/SKILL.md")
	if err != nil {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	writePublic(w, http.StatusOK, "text/markdown; charset=utf-8", raw)
}

func (s *Server) handleAICatalog(w http.ResponseWriter, r *http.Request) {
	origin := s.publicOrigin(r)
	host := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}
	writeJSONCORS(w, map[string]any{
		"specVersion": "0.1",
		"host": map[string]any{
			"name": "Syncidian",
			"url":  origin,
		},
		"entries": []map[string]any{
			{
				"id":          "urn:air:" + host + ":mcp:syncidian",
				"displayName": "Syncidian MCP",
				"type":        "application/json",
				"url":         origin + "/.well-known/mcp/server-card.json",
				"representativeQueries": []string{
					"search my Obsidian notes",
					"read a note from my vault",
					"show backlinks for a note",
					"how do I authenticate to Syncidian MCP",
				},
			},
			{
				"id":          "urn:air:" + host + ":docs:auth",
				"displayName": "Syncidian agent auth",
				"type":        "text/markdown",
				"url":         origin + "/auth.md",
				"representativeQueries": []string{
					"how do I get a Syncidian access token",
					"MCP bearer token for Obsidian vault",
				},
			},
		},
	}, "application/json")
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	origin := s.publicOrigin(r)
	writeJSONCORS(w, map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Syncidian",
			"version":     mcpServerVersion,
			"description": "Obsidian sync coordination and MCP. Vault APIs require a Bearer sk_sync_ token or a dashboard session.",
		},
		"servers": []map[string]any{{"url": origin}},
		"security": []map[string]any{
			{"bearerAuth": []any{}},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "sk_sync"},
			},
		},
		"paths": map[string]any{
			"/health": map[string]any{
				"get": map[string]any{
					"summary":   "Liveness",
					"security":  []any{},
					"responses": map[string]any{"200": map[string]any{"description": "OK"}},
				},
			},
			"/mcp": map[string]any{
				"post": map[string]any{
					"summary":   "MCP JSON-RPC",
					"responses": map[string]any{"200": map[string]any{"description": "JSON-RPC response"}},
				},
			},
			"/api/v1/mcp/login": map[string]any{
				"post": map[string]any{
					"summary":   "Exchange vault username/password for a Bearer token",
					"security":  []any{},
					"responses": map[string]any{"200": map[string]any{"description": "Token (shown once)"}},
				},
			},
			"/api/v1/sync/plan": map[string]any{
				"post": map[string]any{"summary": "Plan vault sync", "responses": map[string]any{"200": map[string]any{"description": "Plan"}}},
			},
			"/api/v1/sync/manifest": map[string]any{
				"get": map[string]any{"summary": "Vault file manifest", "responses": map[string]any{"200": map[string]any{"description": "Manifest"}}},
			},
		},
	}, "application/vnd.oai.openapi+json")
}

func writeJSONCORS(w http.ResponseWriter, v any, contentType string) {
	allowAgents(w)
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
