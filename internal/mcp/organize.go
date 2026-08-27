package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shangeethsivan/Syncidian/internal/store"
)

var (
	frontmatterRe = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n`)
	typeFieldRe   = regexp.MustCompile(`(?mi)^type:\s*["']?([^\n"']+)`)
	ideaFolderRe  = regexp.MustCompile(`(?i)(^|/)(ideas?|inbox|fleeting|braindump)(/|$)`)
)

func (s *Server) suggestNotePath(userID, topic, kind string) (any, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, fmt.Errorf("topic is required")
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "idea"
	}

	paths, _, _, err := s.noteIndex(userID)
	if err != nil {
		return nil, err
	}

	type candidate struct {
		Path  string `json:"path"`
		Score int    `json:"score"`
		Why   string `json:"why"`
	}
	var cands []candidate
	topicLower := strings.ToLower(topic)
	tokens := tokenize(topicLower)

	for _, p := range paths {
		score := 0
		var reasons []string
		base := strings.ToLower(noteStem(p))
		fullLower := strings.ToLower(p)

		if strings.Contains(base, topicLower) || strings.Contains(fullLower, topicLower) {
			score += 50
			reasons = append(reasons, "path matches topic")
		}
		for _, tok := range tokens {
			if len(tok) < 3 {
				continue
			}
			if strings.Contains(base, tok) {
				score += 15
			}
			if strings.Contains(fullLower, tok) {
				score += 5
			}
		}
		if ideaFolderRe.MatchString(p) {
			score += 20
			reasons = append(reasons, "in ideas/inbox folder")
		}
		if kind == "idea" && (strings.Contains(base, "idea") || strings.Contains(fullLower, "/ideas/")) {
			score += 10
		}

		body, err := s.readVaultText(userID, p)
		if err == nil {
			if fm := frontmatterRe.FindStringSubmatch(body); len(fm) > 1 {
				if m := typeFieldRe.FindStringSubmatch(fm[1]); len(m) > 1 {
					t := strings.ToLower(strings.TrimSpace(m[1]))
					if t == kind || (kind == "idea" && (t == "ideas" || t == "fleeting")) {
						score += 40
						reasons = append(reasons, "frontmatter type="+t)
					}
				}
			}
			lower := strings.ToLower(body)
			if strings.Contains(lower, topicLower) {
				score += 25
				reasons = append(reasons, "content mentions topic")
			}
		}

		if score > 0 {
			why := strings.Join(reasons, "; ")
			if why == "" {
				why = "keyword overlap"
			}
			cands = append(cands, candidate{Path: p, Score: score, Why: why})
		}
	}

	sort.Slice(cands, func(i, j int) bool {
		if cands[i].Score == cands[j].Score {
			return cands[i].Path < cands[j].Path
		}
		return cands[i].Score > cands[j].Score
	})
	if len(cands) > 10 {
		cands = cands[:10]
	}

	slug := slugify(topic)
	suggestedNew := path.Join(defaultFolderForKind(kind, paths), slug+".md")
	out := map[string]any{
		"topic":            topic,
		"kind":             kind,
		"suggested_new":    suggestedNew,
		"existing_matches": cands,
		"hint":             "Prefer appending to an existing match when score is high; otherwise create suggested_new in the same folder style as the vault.",
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	return textResult(string(raw)), nil
}

func defaultFolderForKind(kind string, paths []string) string {
	counts := map[string]int{}
	for _, p := range paths {
		dir := path.Dir(p)
		if dir == "." {
			continue
		}
		if ideaFolderRe.MatchString(dir) {
			counts[dir]++
		}
	}
	best := ""
	bestN := 0
	for d, n := range counts {
		if n > bestN || (n == bestN && d < best) {
			best, bestN = d, n
		}
	}
	if best != "" {
		return best
	}
	switch kind {
	case "idea", "ideas", "fleeting":
		return "Ideas"
	case "project":
		return "Projects"
	case "meeting":
		return "Meetings"
	default:
		return "Notes"
	}
}

func slugify(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "note"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	return fields
}

func (s *Server) findRelated(userID, query string, limit int) (any, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	paths, byStem, byPath, err := s.noteIndex(userID)
	if err != nil {
		return nil, err
	}

	// If query looks like a path, seed with its outgoing + backlinks.
	seedLinks := map[string]int{}
	qPath, ok := vaultRel(ensureMD(query))
	if ok {
		if body, err := s.readVaultText(userID, qPath); err == nil {
			for _, t := range extractWikiTargets(body) {
				resolved := resolveWikiLink(t, byStem, byPath)
				seedLinks[resolved] += 30
			}
		}
		// Approximate backlinks cheaply via index scan of stems
		want := strings.ToLower(noteStem(qPath))
		for _, p := range paths {
			if strings.EqualFold(p, qPath) {
				continue
			}
			b, err := s.loadNote(userID, p)
			if err != nil || !utf8.Valid(b) {
				continue
			}
			for _, t := range extractWikiTargets(string(b)) {
				if strings.ToLower(noteStem(t)) == want {
					seedLinks[p] += 30
					break
				}
			}
		}
	}

	qLower := strings.ToLower(query)
	tokens := tokenize(qLower)
	type hit struct {
		Path  string `json:"path"`
		Score int    `json:"score"`
	}
	var hits []hit
	for _, p := range paths {
		score := seedLinks[p]
		pl := strings.ToLower(p)
		if strings.Contains(pl, qLower) {
			score += 40
		}
		for _, tok := range tokens {
			if len(tok) >= 3 && strings.Contains(pl, tok) {
				score += 8
			}
		}
		b, err := s.loadNote(userID, p)
		if err == nil && utf8.Valid(b) {
			body := strings.ToLower(string(b))
			if strings.Contains(body, qLower) {
				score += 20
			}
			for _, tok := range tokens {
				if len(tok) >= 3 && strings.Contains(body, tok) {
					score += 3
				}
			}
		}
		if score > 0 {
			hits = append(hits, hit{Path: p, Score: score})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	raw, _ := json.MarshalIndent(map[string]any{"query": query, "related": hits}, "", "  ")
	return textResult(string(raw)), nil
}

func (s *Server) appendToNote(userID, notePath, content, heading string) (any, error) {
	notePath, ok := vaultRel(ensureMD(notePath))
	if !ok {
		return nil, fmt.Errorf("invalid path")
	}
	content = strings.TrimRight(content, " \t\r\n")
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	var body string
	exists := true
	b, err := s.loadNote(userID, notePath)
	if err != nil {
		if !errors.Is(err, ErrNoteNotFound) {
			return nil, noteErr(err)
		}
		exists = false
		body = ""
	} else {
		if !utf8.Valid(b) {
			return nil, fmt.Errorf("note is not valid UTF-8")
		}
		body = string(b)
	}

	chunk := content
	if heading != "" {
		h := strings.TrimSpace(heading)
		if !strings.HasPrefix(h, "#") {
			h = "## " + h
		}
		if updated, ok := insertUnderHeading(body, h, content); ok {
			return s.writeNote(userID, notePath, updated, true)
		}
		chunk = h + "\n\n" + content
	}

	if !exists {
		return s.writeNote(userID, notePath, chunk+"\n", false)
	}
	trimmed := strings.TrimRight(body, " \t\r\n")
	return s.writeNote(userID, notePath, trimmed+"\n\n"+chunk+"\n", true)
}

func insertUnderHeading(body, heading, content string) (string, bool) {
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimRight(line, "\r") == heading {
			start = i
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	headingLevel := 0
	for _, r := range heading {
		if r == '#' {
			headingLevel++
		} else {
			break
		}
	}
	for i := start + 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		if !strings.HasPrefix(line, "#") {
			continue
		}
		level := 0
		for _, r := range line {
			if r == '#' {
				level++
			} else {
				break
			}
		}
		if level > 0 && level <= headingLevel && (len(line) == level || line[level] == ' ') {
			end = i
			break
		}
	}
	insert := append([]string{}, lines[:end]...)
	if end > start+1 && strings.TrimSpace(lines[end-1]) != "" {
		insert = append(insert, "")
	}
	insert = append(insert, content, "")
	insert = append(insert, lines[end:]...)
	return strings.Join(insert, "\n"), true
}

func (s *Server) resolveExisting(userID, raw string) (string, error) {
	p, ok := vaultRel(raw)
	if !ok {
		return "", fmt.Errorf("invalid path")
	}
	if s.noteExists(userID, p) {
		return p, nil
	}
	if !basenameHasDot(p) {
		md, ok := vaultRel(ensureMD(p))
		if ok && s.noteExists(userID, md) {
			return md, nil
		}
	}
	return "", ErrNoteNotFound
}

func destForMove(from, rawTo string) (string, error) {
	to, ok := vaultRel(rawTo)
	if !ok {
		return "", fmt.Errorf("invalid to path")
	}
	if strings.HasSuffix(strings.ToLower(from), ".md") && !basenameHasDot(to) {
		to, ok = vaultRel(ensureMD(to))
		if !ok {
			return "", fmt.Errorf("invalid to path")
		}
	}
	return to, nil
}

func (s *Server) moveNote(userID, from, to string) (any, error) {
	resolved, err := s.resolveExisting(userID, from)
	if err != nil {
		return nil, noteErr(err)
	}
	from = resolved
	to, err = destForMove(from, to)
	if err != nil {
		return nil, err
	}
	if from == to {
		return textResult("Already at " + to), nil
	}
	b, err := s.loadNote(userID, from)
	if err != nil {
		return nil, noteErr(err)
	}
	if s.noteExists(userID, to) {
		return nil, fmt.Errorf("destination already exists")
	}
	if err := s.notes().Put(userID, to, b, false); err != nil {
		return nil, noteErr(err)
	}
	if err := s.notes().Delete(userID, from); err != nil {
		return nil, noteErr(err)
	}
	sum := sha256.Sum256(b)
	hash := hex.EncodeToString(sum[:])
	mtime := time.Now().UnixMilli()
	_ = s.Store.UpsertFile(store.FileMeta{UserID: userID, Path: from, Deleted: true, Mtime: mtime})
	if err := s.Store.UpsertFile(store.FileMeta{
		UserID: userID, Path: to, Hash: hash, Size: int64(len(b)), Mtime: mtime,
	}); err != nil {
		return nil, err
	}
	s.notify(userID, from, "", true, nil)
	s.notify(userID, to, hash, false, b)
	_ = s.Store.AddActivity(store.Activity{UserID: userID, Action: "mcp.move", Detail: from + " -> " + to})
	return textResult("Moved " + from + " → " + to), nil
}

func (s *Server) deleteNote(userID, notePath string) (any, error) {
	notePath, err := s.resolveExisting(userID, notePath)
	if err != nil {
		return nil, noteErr(err)
	}
	if err := s.notes().Delete(userID, notePath); err != nil {
		return nil, noteErr(err)
	}
	mtime := time.Now().UnixMilli()
	if err := s.Store.UpsertFile(store.FileMeta{UserID: userID, Path: notePath, Deleted: true, Mtime: mtime}); err != nil {
		return nil, err
	}
	s.notify(userID, notePath, "", true, nil)
	_ = s.Store.AddActivity(store.Activity{UserID: userID, Action: "mcp.delete", Detail: notePath})
	return textResult("Deleted " + notePath), nil
}

func (s *Server) listUnderPrefix(userID, fromPrefix string) ([]string, error) {
	all, err := s.notes().List(userID)
	if err != nil {
		return nil, noteErr(err)
	}
	var targets []string
	for _, p := range all {
		p, ok := vaultRel(p)
		if !ok {
			continue
		}
		if strings.HasPrefix(p, fromPrefix+"/") || p == fromPrefix {
			targets = append(targets, p)
		}
	}
	sort.Strings(targets)
	return targets, nil
}

func (s *Server) bulkMove(userID, fromPrefix, toPrefix string, paths []string) (any, error) {
	fromPrefix = strings.Trim(strings.ReplaceAll(fromPrefix, "\\", "/"), "/")
	toPrefix = strings.Trim(strings.ReplaceAll(toPrefix, "\\", "/"), "/")
	if fromPrefix == "" || toPrefix == "" {
		return nil, fmt.Errorf("from_prefix and to_prefix are required")
	}

	var targets []string
	if len(paths) > 0 {
		for _, p := range paths {
			resolved, err := s.resolveExisting(userID, p)
			if err != nil {
				return nil, fmt.Errorf("path %s: %w", p, noteErr(err))
			}
			if !strings.HasPrefix(resolved, fromPrefix+"/") && resolved != fromPrefix {
				return nil, fmt.Errorf("path %s is not under %s", resolved, fromPrefix)
			}
			targets = append(targets, resolved)
		}
	} else {
		var err error
		targets, err = s.listUnderPrefix(userID, fromPrefix)
		if err != nil {
			return nil, err
		}
	}
	if len(targets) == 0 {
		return textResult("No files to move."), nil
	}
	if len(targets) > 200 {
		return nil, fmt.Errorf("refusing to move more than 200 files at once (%d matched)", len(targets))
	}

	var moved []string
	var errs []string
	for _, from := range targets {
		rel := strings.TrimPrefix(from, fromPrefix)
		rel = strings.TrimPrefix(rel, "/")
		to := path.Join(toPrefix, rel)
		if _, err := s.moveNote(userID, from, to); err != nil {
			errs = append(errs, from+": "+err.Error())
			continue
		}
		moved = append(moved, from+" → "+to)
	}
	s.notifyBatch(userID)
	out := map[string]any{"moved": moved, "errors": errs, "count": len(moved)}
	raw, _ := json.MarshalIndent(out, "", "  ")
	return textResult(string(raw)), nil
}

func (s *Server) bulkAddLinks(userID, linkTo string, paths []string) (any, error) {
	linkTo, ok := vaultRel(ensureMD(linkTo))
	if !ok {
		return nil, fmt.Errorf("invalid link_to path")
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("paths is required")
	}
	if len(paths) > 100 {
		return nil, fmt.Errorf("refusing more than 100 paths")
	}
	var updated []string
	var errs []string
	for _, p := range paths {
		if _, err := s.addBacklink(userID, p, linkTo, ""); err != nil {
			errs = append(errs, p+": "+err.Error())
			continue
		}
		updated = append(updated, p)
	}
	raw, _ := json.MarshalIndent(map[string]any{"updated": updated, "errors": errs, "link_to": linkTo}, "", "  ")
	return textResult(string(raw)), nil
}
