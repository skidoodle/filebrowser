package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/skidoodle/filebrowser/internal/api"
	"github.com/skidoodle/filebrowser/internal/auth"
	"github.com/skidoodle/filebrowser/internal/authz"
	"github.com/skidoodle/filebrowser/internal/config"
	"github.com/skidoodle/filebrowser/internal/guard"
	"github.com/skidoodle/filebrowser/internal/storage/local"
	"github.com/skidoodle/filebrowser/internal/store"
)

// Build metadata injected at release time via -ldflags "-X ...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// errNoEmbed signals dev builds where the SPA runs on the Vite dev server.
var errNoEmbed = errors.New("dev build: frontend is served by the Vite dev server (`just dev-frontend`)")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// newAuthManager wires the auth manager on top of the account store, or
// returns nil in insecure mode where authentication is disabled entirely.
func newAuthManager(cfg *config.Config, st *store.Store, log *slog.Logger) (*auth.Manager, error) {
	if cfg.Insecure {
		log.Warn("INSECURE MODE: every visitor has full write access; never enable this on a public host")
		return nil, nil
	}
	seed := cfg.AdminPassword
	if seed != "" {
		log.Info("seeding admin password from FILEBROWSER_ADMIN_PASSWORD")
	}
	dir := filepath.Join(cfg.Root, ".filebrowser")
	mgr, err := auth.New(dir, cfg.Secret, seed, st, log)
	if err != nil {
		return nil, err
	}
	if !mgr.Initialized() {
		log.Info("no accounts yet; the first visit can create one via onboarding")
	}
	return mgr, nil
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	level := slog.LevelInfo
	if cfg.Dev {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	fsStore, err := local.New(cfg.Root)
	if err != nil {
		return err
	}

	// Account + private-folder persistence lives in SQLite inside the
	// reserved namespace.
	st, err := store.Open(filepath.Join(cfg.Root, ".filebrowser", "users.db"))
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	// The permission engine caches the private-folder set and invalidates
	// on every change.
	eng, err := authz.New(st)
	if err != nil {
		return err
	}

	mgr, err := newAuthManager(cfg, st, log)
	if err != nil {
		return err
	}

	var grd *guard.Guard
	if cfg.Guard {
		proxies := make([]string, 0, len(cfg.TrustedProxies))
		for _, p := range cfg.TrustedProxies {
			proxies = append(proxies, p.String())
		}
		grd, err = guard.New(guard.Config{
			TrustedProxies: proxies,
			RequestRate:    cfg.RequestRate,
			DownloadRate:   cfg.DownloadRate,
			Difficulty:     cfg.PowDifficulty,
			Secret:         cfg.Secret,
			CacheDir:       cfg.CacheDir,
		}, log)
		if err != nil {
			return err
		}
	} else {
		log.Info("guard disabled by configuration")
	}

	web, err := webFS()
	if err != nil && !errors.Is(err, errNoEmbed) {
		return err
	}

	srv, err := api.New(api.Options{
		Config:  cfg,
		Store:   fsStore,
		Log:     log,
		Web:     web,
		Version: version,
		Commit:  commit,
		Guard:   grd,
		Auth:    mgr,
		Authz:   eng,
	})
	if err != nil {
		return err
	}

	log.Info("starting filebrowser", "version", version, "commit", commit, "built", date, "root", cfg.Root, "address", cfg.Address)
	return srv.Run(ctx)
}
