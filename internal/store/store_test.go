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
	if err := st.SetGitHub(GitHubConfig{
		UserID: bob.ID, Token: "ghs_cached", Repo: "bob/second", Branch: "develop",
		AppID: 42, AppSlug: "syncidian-bob", AppPEM: "pem", InstallationID: 99,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetGitHub(bob.ID)
	if err != nil || got == nil || got.Repo != "bob/second" || got.AppID != 42 || got.InstallationID != 99 {
		t.Fatalf("bob should have one replaced repo: %+v %v", got, err)
	}
	if got.Branch != "main" {
		t.Fatalf("store should default branch to main, got %q", got.Branch)
	}
	if !got.Configured() {
		t.Fatalf("app install should count as configured: %+v", got)
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

func TestGitHubIdentityAndInstanceApp(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	u, err := st.CreateGitHubUser("octocat", "octocat@example.com", 4242)
	if err != nil {
		t.Fatal(err)
	}
	if u.GitHubID != 4242 || u.Email != "octocat@example.com" || u.PasswordHash != "" {
		t.Fatalf("github user: %+v", u)
	}
	got, err := st.GetUserByGitHubID(4242)
	if err != nil || got == nil || got.ID != u.ID {
		t.Fatalf("lookup by github id: %+v %v", got, err)
	}
	byEmail, err := st.GetUserByEmail("octocat@example.com")
	if err != nil || byEmail == nil || byEmail.ID != u.ID {
		t.Fatalf("lookup by email: %+v %v", byEmail, err)
	}

	if err := st.SetInstanceGitHubApp(GitHubApp{
		AppID: 7, Slug: "syncidian", PEM: "pem", ClientID: "iv1", ClientSecret: "sec",
	}); err != nil {
		t.Fatal(err)
	}
	app, err := st.GetInstanceGitHubApp()
	if err != nil || app == nil || !app.Configured() || app.Slug != "syncidian" {
		t.Fatalf("instance app: %+v %v", app, err)
	}
}
