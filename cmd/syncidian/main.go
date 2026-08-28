package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/shangeethsivan/Syncidian/internal/config"
	"github.com/shangeethsivan/Syncidian/internal/server"
	"github.com/shangeethsivan/Syncidian/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const version = "0.1.3"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			if err := runServe(log); err != nil {
				log.Error("serve", "err", err)
				os.Exit(1)
			}
			return
		case "user":
			if err := runUser(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "token":
			if err := runToken(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "version", "-v", "--version":
			fmt.Println("syncidian", version)
			return
		case "help", "-h", "--help":
			printHelp()
			return
		}
	}
	if err := runServe(log); err != nil {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`Syncidian — self-hosted Obsidian sync, GitHub backup, and MCP.

Usage:
  syncidian serve              Start the HTTP server (default)
  syncidian user create USER   Create a dashboard user
  syncidian token create USER  Create an Obsidian access token
  syncidian version

Environment:
  SYNCIDIAN_ADDR                 Listen address (default :8080, or $PORT)
  SYNCIDIAN_DATA                 Data directory (default ./data, or $RAILWAY_VOLUME_MOUNT_PATH).
                                 Persist this path (Docker/Railway volume at /data) or every
                                 deploy wipes users, GitHub App credentials, and vault files.
  SYNCIDIAN_DATA_KEY             Optional 32-byte hex/base64 key to encrypt GitHub secrets at rest.
  SYNCIDIAN_PUBLIC_URL           Public URL shown in the dashboard
  SYNCIDIAN_BOOTSTRAP_USER       Create this admin on first boot
  SYNCIDIAN_BOOTSTRAP_PASSWORD   Password for the bootstrap admin
  SYNCIDIAN_ADMIN_PATH           Unlisted operator path (default /admin). Not linked from the public site.
  SYNCIDIAN_GITHUB_APP_ID        GitHub App ID (optional; operator page can create the app)
  SYNCIDIAN_GITHUB_APP_SLUG      GitHub App slug
  SYNCIDIAN_GITHUB_CLIENT_ID     GitHub App OAuth client ID
  SYNCIDIAN_GITHUB_CLIENT_SECRET GitHub App OAuth client secret
  SYNCIDIAN_GITHUB_APP_PRIVATE_KEY  PEM; use \n for newlines
`)
}

func runServe(log *slog.Logger) error {
	cfg := config.FromEnv()
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.DataDir, "data", cfg.DataDir, "data directory")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	srv := server.New(cfg, st, log)
	srv.BootstrapFromEnv()
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	p := config.PersistenceStatus(cfg.DataDir)
	if !p.OK {
		log.Error("data directory is not persistent; users, GitHub App credentials, and vault files will be lost on the next deploy",
			"data", cfg.DataDir, "hint", p.Hint)
	}
	log.Info("syncidian listening", "addr", cfg.Addr, "data", cfg.DataDir, "url", cfg.PublicURL, "persistence", p.Kind)
	return httpSrv.ListenAndServe()
}

func openStore() (*store.Store, error) {
	cfg := config.FromEnv()
	return store.Open(cfg.DataDir)
}

func runUser(args []string) error {
	if len(args) < 2 || args[0] != "create" {
		return fmt.Errorf("usage: syncidian user create <username> [password]")
	}
	username := args[1]
	password := ""
	if len(args) > 2 {
		password = args[2]
	}
	if password == "" {
		password = os.Getenv("SYNCIDIAN_BOOTSTRAP_PASSWORD")
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters (pass as argument or SYNCIDIAN_BOOTSTRAP_PASSWORD)")
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	n, _ := st.UserCount()
	u, err := st.CreateUser(username, string(hash), n == 0)
	if err != nil {
		return err
	}
	fmt.Printf("created user %s (%s) admin=%v\n", u.Username, u.ID, u.IsAdmin)
	return nil
}

func runToken(args []string) error {
	if len(args) < 2 || args[0] != "create" {
		return fmt.Errorf("usage: syncidian token create <username> [name]")
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	u, err := st.GetUserByUsername(args[1])
	if err != nil || u == nil {
		return fmt.Errorf("user not found")
	}
	if u.IsAdmin {
		return fmt.Errorf("admins manage users and cannot hold vault access tokens")
	}
	name := "Obsidian"
	if len(args) > 2 {
		name = strings.Join(args[2:], " ")
	}
	raw, prefix, err := newToken()
	if err != nil {
		return err
	}
	t, err := st.CreateToken(u.ID, name, raw, prefix)
	if err != nil {
		return err
	}
	fmt.Printf("token %s for %s:\n%s\n\nStore this in the Obsidian plugin. It will not be shown again.\n", t.Name, u.Username, raw)
	return nil
}

func newToken() (raw, prefix string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = "sk_sync_" + hex.EncodeToString(b)
	prefix = raw[:16] + "…"
	return raw, prefix, nil
}
