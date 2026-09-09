package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/skidoodle/filebrowser/internal/auth"
	"github.com/skidoodle/filebrowser/internal/authz"
	"github.com/skidoodle/filebrowser/internal/config"
	"github.com/skidoodle/filebrowser/internal/guard"
	"github.com/skidoodle/filebrowser/internal/storage"
	"github.com/skidoodle/filebrowser/internal/store"
)

// Server wires configuration, storage and the HTTP mux together.
type Server struct {
	cfg      *config.Config
	store    storage.Storage
	appStore *store.Store
	log      *slog.Logger
	web      fs.FS
	version  string
	commit   string
	guard    *guard.Guard
	tokens   *guard.Tokens
	auth     *auth.Manager
	authz    *authz.Engine
	mux      *http.ServeMux
	tus      *tusRegistry
}

// Options configures a new Server.
type Options struct {
	Config   *config.Config
	Store    storage.Storage
	AppStore *store.Store
	Log      *slog.Logger
	Web      fs.FS
	Version  string
	Commit   string
	Guard    *guard.Guard
	Auth     *auth.Manager
	Authz    *authz.Engine
}

// New builds the server and its routing table.
func New(o Options) (*Server, error) {
	var tokens *guard.Tokens
	if o.Guard != nil && o.Guard.Tokens != nil {
		tokens = o.Guard.Tokens
	} else {
		var err error
		tokens, err = guard.NewTokens()
		if err != nil {
			return nil, err
		}
	}
	appSt := o.AppStore
	if appSt == nil && o.Auth != nil {
		appSt = o.Auth.Store()
	}
	s := &Server{
		cfg:      o.Config,
		store:    o.Store,
		appStore: appSt,
		log:      o.Log,
		web:      o.Web,
		version:  o.Version,
		commit:   o.Commit,
		guard:    o.Guard,
		tokens:   tokens,
		auth:     o.Auth,
		authz:    o.Authz,
		mux:      http.NewServeMux(),
		tus:      newTusRegistry(o.Store, o.Log, o.Config.MaxUpload, 3*time.Hour),
	}
	if err := s.routes(); err != nil {
		return nil, err
	}
	return s, nil
}

// routes registers every endpoint on the mux.
func (s *Server) routes() error {
	// Static web assets (the Vite-built React SPA). When nil, assets are
	// served by the Vite dev server during development.
	s.mux.HandleFunc("GET /api/health", s.handleHealth)

	// Public reads (gated by authz inside the handlers).
	s.mux.HandleFunc("GET /api/me", s.handleMe)
	s.mux.HandleFunc("GET /api/list", s.handleList)
	s.mux.HandleFunc("GET /api/meta", s.handleMeta)
	s.mux.HandleFunc("GET /api/raw", s.handleRaw)
	s.mux.HandleFunc("GET /api/download", s.handleDownload)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
	s.mux.HandleFunc("GET /api/usage", s.handleUsage)
	s.mux.HandleFunc("GET /api/thumb", s.handleThumb)
	s.mux.HandleFunc("GET /api/big", s.handleBig)
	s.mux.HandleFunc("GET /api/subtitle", s.handleSubtitle)
	s.mux.HandleFunc("/api/capability", s.handleCapability)

	// Auth operations.
	s.mux.HandleFunc("POST /api/auth/setup", s.handleAuthSetup)
	s.mux.HandleFunc("POST /api/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	s.mux.Handle("POST /api/auth/password", s.write(http.HandlerFunc(s.handleAuthPassword)))

	// Mutations require a capability token minted by the SPA.
	// Creation endpoints allow anonymous visitors when access policy is public.
	s.mux.Handle("POST /api/dir", s.writeCreation(s.capability(http.HandlerFunc(s.handleCreateDir))))
	s.mux.Handle("POST /api/file", s.writeCreation(s.capability(http.HandlerFunc(s.handleCreateFile))))
	s.mux.Handle("POST /api/tus", s.writeCreation(s.capability(http.HandlerFunc(s.handleTusCreate))))
	s.mux.Handle("PATCH /api/tus/{id}", s.capability(http.HandlerFunc(s.handleTusPatch)))
	s.mux.Handle("DELETE /api/tus/{id}", s.capability(http.HandlerFunc(s.handleTusDelete)))

	// Mutating file operations: authenticated writers with permission on
	// the target (admin anywhere, scoped users inside their scope), or valid edit token.
	s.mux.Handle("POST /api/delete", s.write(http.HandlerFunc(s.handleDelete)))
	s.mux.Handle("POST /api/move", s.write(http.HandlerFunc(s.handleMove)))
	s.mux.Handle("PUT /api/raw", s.capability(http.HandlerFunc(s.handleSave)))
	s.mux.HandleFunc("HEAD /api/tus/{id}", s.handleTusHead)
	s.mux.HandleFunc("OPTIONS /api/tus", s.handleTusOptions)
	s.mux.HandleFunc("OPTIONS /api/tus/{id}", s.handleTusOptions)

	// Mark directories private.
	s.mux.Handle("POST /api/private", s.write(http.HandlerFunc(s.handleSetPrivate)))
	s.mux.Handle("DELETE /api/private", s.write(http.HandlerFunc(s.handleUnsetPrivate)))

	// Account self-service.
	s.mux.Handle("GET /api/users", s.admin(http.HandlerFunc(s.handleListUsers)))
	s.mux.Handle("POST /api/users", s.admin(http.HandlerFunc(s.handleCreateUser)))
	s.mux.Handle("PATCH /api/users/{id}", s.admin(http.HandlerFunc(s.handleUpdateUser)))
	s.mux.Handle("DELETE /api/users/{id}", s.admin(http.HandlerFunc(s.handleDeleteUser)))

	// Access policy management.
	s.mux.Handle("GET /api/settings/policy", s.admin(http.HandlerFunc(s.handleGetPolicy)))
	s.mux.Handle("PUT /api/settings/policy", s.admin(http.HandlerFunc(s.handleSetPolicy)))

	if s.web != nil {
		s.mux.Handle("/", s.spaHandler())
	}
	return nil
}

// capability wraps a mutation handler with the guard's capability check.
func (s *Server) capability(next http.Handler) http.Handler {
	if s.guard == nil {
		return next
	}
	return s.guard.RequireCapability(next)
}

// Handler returns the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	h := s.recoverMiddleware(s.logMiddleware(s.secureHeadersMiddleware(s.mux)))
	if s.guard != nil {
		h = s.guard.Middleware(h)
	}
	if s.cfg.BaseURL != "" {
		h = http.StripPrefix(s.cfg.BaseURL, h)
	}
	return h
}

// Run starts the HTTP server and blocks until ctx is cancelled,
// then drains in-flight connections gracefully.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Address,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.cfg.Address)
	if err != nil {
		return fmt.Errorf("api: listen: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("listening", "address", ln.Addr().String())
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			s.log.Error("graceful shutdown failed", "err", err)
			return fmt.Errorf("api: shutdown: %w", ctx.Err())
		}
		s.log.Info("shutdown complete")
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("api: serve: %w", err)
	}
}

// spaCSP locks the embedded app down: everything same-origin, styles may be
// inline (pdf.js/video.js inject them), objects and framing are banned.
const spaCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; media-src 'self' blob:; font-src 'self' data:; " +
	"connect-src 'self'; worker-src 'self' blob:; object-src 'none'; " +
	"frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// spaHandler serves the embedded SPA: real files straight from disk,
// anything else falls back to index.html for client-side routing.
func (s *Server) spaHandler() http.Handler {
	fileServer := http.FileServerFS(s.web)
	index, indexErr := fs.ReadFile(s.web, "index.html")
	if indexErr == nil {
		index = s.configureSPAIndex(index)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", spaCSP)
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}

		serveIndex := p == "index.html"
		if !serveIndex {
			_, err := fs.Stat(s.web, p)
			serveIndex = err != nil
		}
		if serveIndex && indexErr == nil {
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
			return
		}
		if serveIndex {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

// configureSPAIndex injects the external mount path into frontend URLs.
func (s *Server) configureSPAIndex(index []byte) []byte {
	if s.cfg == nil || s.cfg.BaseURL == "" {
		return index
	}

	baseURL := html.EscapeString(s.cfg.BaseURL)
	index = bytes.Replace(
		index,
		[]byte(`name="filebrowser-base" content=""`),
		[]byte(`name="filebrowser-base" content="`+baseURL+`"`),
		1,
	)
	return bytes.ReplaceAll(index, []byte(`"/assets/`), []byte(`"`+baseURL+`/assets/`))
}
