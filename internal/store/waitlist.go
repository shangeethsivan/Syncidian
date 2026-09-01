package store

import (
	"fmt"
	"strings"
)

func NormalizeWaitlistEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func ValidWaitlistEmail(email string) bool {
	email = NormalizeWaitlistEmail(email)
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	at := strings.LastIndexByte(email, '@')
	if at < 1 || at == len(email)-1 {
		return false
	}
	local, domain := email[:at], email[at+1:]
	if strings.ContainsAny(local, " \t\r\n@") || strings.ContainsAny(domain, " \t\r\n@") {
		return false
	}
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	return true
}

// AddWaitlistEmail stores an encrypted email. created is false when the address
// is already on the list. The unique key is an HMAC of the normalized address,
// so the plaintext never has to sit in a unique index.
func (s *Store) AddWaitlistEmail(email string) (created bool, err error) {
	email = NormalizeWaitlistEmail(email)
	if !ValidWaitlistEmail(email) {
		return false, fmt.Errorf("invalid email")
	}
	hash := s.crypt.MAC(email)
	enc := s.seal(email)
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO waitlist (id, email_hash, email_enc, created_at) VALUES (?, ?, ?, ?)`,
		NewID(), hash, enc, now(),
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Store) ListWaitlist() ([]WaitlistEntry, error) {
	rows, err := s.db.Query(`SELECT id, email_enc, created_at FROM waitlist ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WaitlistEntry
	for rows.Next() {
		var e WaitlistEntry
		var enc, created string
		if err := rows.Scan(&e.ID, &enc, &created); err != nil {
			return nil, err
		}
		email, err := s.openSecret(enc)
		if err != nil {
			return nil, err
		}
		e.Email = email
		e.CreatedAt = parseTime(created)
		out = append(out, e)
	}
	if out == nil {
		out = []WaitlistEntry{}
	}
	return out, rows.Err()
}

func (s *Store) WaitlistCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM waitlist`).Scan(&n)
	return n, err
}
