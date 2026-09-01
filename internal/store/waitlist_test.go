package store

import (
	"strings"
	"testing"
)

func TestWaitlistEncryptsEmailAtRest(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	created, err := st.AddWaitlistEmail("  Ada@Syncidian.com ")
	if err != nil || !created {
		t.Fatalf("add: created=%v err=%v", created, err)
	}
	again, err := st.AddWaitlistEmail("ada@syncidian.com")
	if err != nil || again {
		t.Fatalf("duplicate should be ignored: created=%v err=%v", again, err)
	}

	var stored string
	if err := st.db.QueryRow(`SELECT email_enc FROM waitlist`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, encPrefix) {
		t.Fatalf("email should be encrypted at rest, got %q", stored)
	}
	if strings.Contains(strings.ToLower(stored), "ada@syncidian.com") {
		t.Fatal("plaintext email leaked into sqlite")
	}

	list, err := st.ListWaitlist()
	if err != nil || len(list) != 1 || list[0].Email != "ada@syncidian.com" {
		t.Fatalf("list: %+v %v", list, err)
	}
	n, err := st.WaitlistCount()
	if err != nil || n != 1 {
		t.Fatalf("count %d %v", n, err)
	}
}

func TestWaitlistRejectsInvalidEmail(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, email := range []string{"", "not-an-email", "a@", "@b.com", "a@b", "a b@c.com"} {
		if _, err := st.AddWaitlistEmail(email); err == nil {
			t.Fatalf("expected invalid email %q", email)
		}
	}
}
