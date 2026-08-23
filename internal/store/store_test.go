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

func TestOneGitHubRepoPerUser(t *testing.T) {
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

	if err := st.SetGitHub(GitHubConfig{UserID: bob.ID, Token: "tok-a", Repo: "bob/first", Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetGitHub(GitHubConfig{UserID: bob.ID, Token: "tok-b", Repo: "bob/second", Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetGitHub(bob.ID)
	if err != nil || got == nil || got.Repo != "bob/second" || got.Token != "tok-b" {
		t.Fatalf("bob should have one replaced repo: %+v %v", got, err)
	}
	if other, err := st.GetGitHub(ada.ID); err != nil || other != nil {
		t.Fatalf("admin must not inherit a repo: %+v %v", other, err)
	}

	public, err := st.ListUsersPublic()
	if err != nil || len(public) != 2 {
		t.Fatalf("public users %d %v", len(public), err)
	}
	for _, u := range public {
		if u.PasswordHash != "" {
			t.Fatalf("ListUsersPublic leaked password hash for %s", u.Username)
		}
	}
}
