package store

import "testing"

func TestCreateUserAndToken(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	u, err := st.CreateUser("ada", "hash", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("ada", "hash", false); err == nil {
		t.Fatal("expected duplicate username to fail")
	}
	tok, err := st.CreateToken(u.ID, "obsidian", "sk_sync_test", "sk_sync_test…")
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetTokenByHash(HashToken("sk_sync_test"))
	if err != nil || got == nil || got.ID != tok.ID {
		t.Fatalf("lookup: %+v %v", got, err)
	}
	n, err := st.UserCount()
	if err != nil || n != 1 {
		t.Fatalf("count %d %v", n, err)
	}
}

func TestGitHubConfigIsPerUser(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ada, err := st.CreateUser("ada", "hash", true)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := st.CreateUser("bob", "hash", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetGitHub(GitHubConfig{UserID: bob.ID, Token: "ghp_bob", Repo: "bob/vault", Branch: "main"}); err != nil {
		t.Fatal(err)
	}

	adaGH, err := st.GetGitHub(ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	if adaGH != nil {
		t.Fatalf("admin must not inherit another user's GitHub config: %+v", adaGH)
	}
	bobGH, err := st.GetGitHub(bob.ID)
	if err != nil || bobGH == nil || bobGH.Repo != "bob/vault" {
		t.Fatalf("bob github: %+v %v", bobGH, err)
	}
}
