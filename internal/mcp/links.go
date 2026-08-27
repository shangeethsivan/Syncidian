package mcp

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Obsidian wikilink: [[Note]], [[Note|alias]], [[Note#heading]], [[Note#heading|alias]], [[path/Note]]
var wikiLinkRe = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|[^\]]*)?\]\]`)

type graphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type graphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func extractWikiTargets(content string) []string {
	matches := wikiLinkRe.FindAllStringSubmatch(content, -1)
	seen := map[string]struct{}{}
	var out []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		target := strings.TrimSpace(m[1])
		target = strings.ReplaceAll(target, "\\", "/")
		target = strings.Trim(target, "/")
		if target == "" {
			continue
		}
		key := strings.ToLower(target)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, target)
	}
	return out
}

// resolveWikiLink maps a wikilink target to a vault path using an index of note stems and full paths.
func resolveWikiLink(target string, byStem map[string][]string, byPath map[string]string) string {
	target = strings.TrimSpace(strings.ReplaceAll(target, "\\", "/"))
	target = strings.Trim(target, "/")
	if target == "" {
		return ""
	}
	withMD := ensureMD(target)
	if p, ok := byPath[strings.ToLower(withMD)]; ok {
		return p
	}
	if p, ok := byPath[strings.ToLower(target)]; ok {
		return p
	}
	stem := noteStem(target)
	if !strings.Contains(target, "/") {
		cands := byStem[strings.ToLower(stem)]
		if len(cands) == 1 {
			return cands[0]
		}
		if len(cands) > 1 {
			// Prefer shallowest / exact basename match
			sort.Strings(cands)
			return cands[0]
		}
	}
	return withMD // unresolved — still useful as a dangling edge target
}

func (s *Server) noteIndex(userID string) (paths []string, byStem map[string][]string, byPath map[string]string, err error) {
	listed, err := s.notes().List(userID)
	if err != nil {
		return nil, nil, nil, noteErr(err)
	}
	byStem = map[string][]string{}
	byPath = map[string]string{}
	for _, p := range mdPaths(listed, "") {
		paths = append(paths, p)
		byPath[strings.ToLower(p)] = p
		stem := strings.ToLower(noteStem(p))
		byStem[stem] = append(byStem[stem], p)
	}
	sort.Strings(paths)
	return paths, byStem, byPath, nil
}

func (s *Server) readVaultText(userID, rel string) (string, error) {
	rel, ok := vaultRel(rel)
	if !ok {
		return "", fmt.Errorf("invalid path")
	}
	b, err := s.loadNote(userID, rel)
	if err != nil {
		return "", noteErr(err)
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("note is not valid UTF-8")
	}
	return string(b), nil
}

func (s *Server) outgoingLinks(userID, notePath string) (any, error) {
	notePath, ok := vaultRel(ensureMD(notePath))
	if !ok {
		return nil, fmt.Errorf("invalid path")
	}
	body, err := s.readVaultText(userID, notePath)
	if err != nil {
		return nil, err
	}
	_, byStem, byPath, err := s.noteIndex(userID)
	if err != nil {
		return nil, err
	}
	targets := extractWikiTargets(body)
	type link struct {
		Target string `json:"target"`
		Path   string `json:"path"`
		Exists bool   `json:"exists"`
	}
	var links []link
	for _, t := range targets {
		resolved := resolveWikiLink(t, byStem, byPath)
		_, exists := byPath[strings.ToLower(resolved)]
		links = append(links, link{Target: t, Path: resolved, Exists: exists})
	}
	if links == nil {
		links = []link{}
	}
	b, _ := json.MarshalIndent(map[string]any{"path": notePath, "links": links}, "", "  ")
	return textResult(string(b)), nil
}

func (s *Server) backlinks(userID, notePath string) (any, error) {
	notePath, ok := vaultRel(ensureMD(notePath))
	if !ok {
		return nil, fmt.Errorf("invalid path")
	}
	paths, byStem, byPath, err := s.noteIndex(userID)
	if err != nil {
		return nil, err
	}
	wantStem := strings.ToLower(noteStem(notePath))
	wantPath := strings.ToLower(notePath)
	var hits []string
	for _, p := range paths {
		if strings.EqualFold(p, notePath) {
			continue
		}
		b, err := s.loadNote(userID, p)
		if err != nil || !utf8.Valid(b) {
			continue
		}
		for _, t := range extractWikiTargets(string(b)) {
			resolved := resolveWikiLink(t, byStem, byPath)
			if strings.ToLower(resolved) == wantPath || strings.ToLower(noteStem(t)) == wantStem {
				hits = append(hits, p)
				break
			}
		}
	}
	sort.Strings(hits)
	if len(hits) == 0 {
		return textResult("No backlinks to " + notePath), nil
	}
	out := map[string]any{"path": notePath, "backlinks": hits}
	raw, _ := json.MarshalIndent(out, "", "  ")
	return textResult(string(raw)), nil
}

func (s *Server) buildGraph(userID string, prefix string, format string) (any, error) {
	paths, byStem, byPath, err := s.noteIndex(userID)
	if err != nil {
		return nil, err
	}
	if prefix != "" {
		prefix = strings.Trim(strings.ReplaceAll(prefix, "\\", "/"), "/")
		var filtered []string
		for _, p := range paths {
			if strings.HasPrefix(p, prefix+"/") || p == prefix || strings.HasPrefix(p, prefix) {
				filtered = append(filtered, p)
			}
		}
		paths = filtered
	}
	nodeSet := map[string]struct{}{}
	var edges []graphEdge
	edgeSeen := map[string]struct{}{}

	for _, p := range paths {
		nodeSet[p] = struct{}{}
		b, err := s.loadNote(userID, p)
		if err != nil || !utf8.Valid(b) {
			continue
		}
		for _, t := range extractWikiTargets(string(b)) {
			to := resolveWikiLink(t, byStem, byPath)
			nodeSet[to] = struct{}{}
			key := p + "->" + to
			if _, ok := edgeSeen[key]; ok {
				continue
			}
			edgeSeen[key] = struct{}{}
			edges = append(edges, graphEdge{From: p, To: to})
		}
	}

	var nodes []graphNode
	for id := range nodeSet {
		nodes = append(nodes, graphNode{ID: id, Label: noteStem(id)})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "json"
	}

	mermaid := renderMermaid(nodes, edges)
	payload := map[string]any{
		"nodes":   nodes,
		"edges":   edges,
		"mermaid": mermaid,
		"stats": map[string]int{
			"nodes": len(nodes),
			"edges": len(edges),
		},
	}
	if format == "mermaid" {
		return textResult(mermaid), nil
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return textResult(string(raw)), nil
}

func renderMermaid(nodes []graphNode, edges []graphEdge) string {
	var b strings.Builder
	b.WriteString("graph LR\n")
	idMap := map[string]string{}
	for i, n := range nodes {
		aid := fmt.Sprintf("n%d", i)
		idMap[n.ID] = aid
		label := strings.ReplaceAll(n.Label, `"`, "'")
		b.WriteString(fmt.Sprintf("  %s[\"%s\"]\n", aid, label))
	}
	for _, e := range edges {
		from, ok1 := idMap[e.From]
		to, ok2 := idMap[e.To]
		if !ok1 || !ok2 {
			continue
		}
		b.WriteString(fmt.Sprintf("  %s --> %s\n", from, to))
	}
	return b.String()
}

func (s *Server) addBacklink(userID, fromPath, toPath, alias string) (any, error) {
	fromPath, ok := vaultRel(ensureMD(fromPath))
	if !ok {
		return nil, fmt.Errorf("invalid from path")
	}
	toPath, ok = vaultRel(ensureMD(toPath))
	if !ok {
		return nil, fmt.Errorf("invalid to path")
	}
	body, err := s.readVaultText(userID, fromPath)
	if err != nil {
		return nil, err
	}
	linkTarget := strings.TrimSuffix(toPath, ".md")
	if path.Dir(fromPath) == path.Dir(toPath) || path.Dir(toPath) == "." {
		// Prefer short stem when same folder or target at root
		if path.Dir(fromPath) == path.Dir(toPath) {
			linkTarget = noteStem(toPath)
		}
	}
	link := "[[" + linkTarget
	if alias != "" {
		link += "|" + alias
	}
	link += "]]"

	// Skip if an equivalent link already exists
	for _, t := range extractWikiTargets(body) {
		if strings.EqualFold(noteStem(t), noteStem(toPath)) || strings.EqualFold(ensureMD(t), toPath) {
			return textResult("Backlink already present in " + fromPath), nil
		}
	}

	trimmed := strings.TrimRight(body, " \t\r\n")
	var newBody string
	if trimmed == "" {
		newBody = link + "\n"
	} else {
		newBody = trimmed + "\n\n" + link + "\n"
	}
	return s.writeNote(userID, fromPath, newBody, true)
}
