package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/shangeethsivan/Syncidian/internal/config"
	"github.com/shangeethsivan/Syncidian/internal/server"
	"github.com/shangeethsivan/Syncidian/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const version = "0.2.0"

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
  SYNCIDIAN_ADMIN_HOST           Private operator hostname (e.g. admin.syncidian.com). When set,
                                 operator UI and admin APIs are served only on that Host.
  SYNCIDIAN_ADMIN_LISTEN_IP      Unicast IP to bind instead of 0.0.0.0 (Tailscale 100.x). Also
                                 reads TAILSCALE_IP. Binds 127.0.0.1 (tunnels) plus this address.
  SYNCIDIAN_GITHUB_APP_ID        GitHub App ID (optional; operator page can create the app)
  SYNCIDIAN_GITHUB_APP_SLUG      GitHub App slug
  SYNCIDIAN_GITHUB_CLIENT_ID     GitHub App OAuth client ID
  SYNCIDIAN_GITHUB_CLIENT_SECRET GitHub App OAuth client secret
  SYNCIDIAN_GITHUB_APP_PRIVATE_KEY  PEM; use \n for newlines
  SYNCIDIAN_GITHUB_ALLOWED_EMAIL Comma-separated GitHub emails allowed to sign in.
                                 Empty = any GitHub user. Production: unset this.
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
	p := config.PersistenceStatus(cfg.DataDir)
	if !p.OK {
		log.Error("data directory is not persistent; users, GitHub App credentials, and vault files will be lost on the next deploy",
			"data", cfg.DataDir, "hint", p.Hint)
	}
	addrs := cfg.ListenAddrs()
	log.Info("syncidian listening", "addr", strings.Join(addrs, ","), "data", cfg.DataDir, "url", cfg.PublicURL, "admin_host", cfg.AdminHost, "persistence", p.Kind)
	return serveHTTP(log, addrs, srv.Handler())
}

func serveHTTP(log *slog.Logger, addrs []string, handler http.Handler) error {
	if len(addrs) == 0 {
		return errors.New("no listen addresses")
	}
	errc := make(chan error, len(addrs))
	for _, addr := range addrs {
		addr := addr
		go func() {
			errc <- listenAndServe(log, addr, handler)
		}()
	}
	return <-errc
}

func listenAndServe(log *slog.Logger, addr string, handler http.Handler) error {
	ln, err := listenWithRetry(log, addr)
	if err != nil {
		return err
	}
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return httpSrv.Serve(ln)
}

func listenWithRetry(log *slog.Logger, addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, nil
	}
	host, _, splitErr := net.SplitHostPort(addr)
	if splitErr != nil || host == "" || host == "127.0.0.1" || host == "::1" {
		return nil, err
	}
	// Tailscale userspace/CGNAT: the 100.x address appears a few seconds after tailscaled.
	const attempts = 15
	log.Info("waiting for listen address (Tailscale IP may not be up yet)", "addr", addr, "err", err)
	for i := 1; i < attempts; i++ {
		time.Sleep(2 * time.Second)
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			log.Info("listen address ready", "addr", addr)
			return ln, nil
		}
	}
	return nil, err
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
