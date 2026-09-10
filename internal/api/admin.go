package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/skidoodle/filebrowser/internal/authz"
	"github.com/skidoodle/filebrowser/internal/store"
)

// maxDeleteBatch bounds how many paths a single delete request may carry.
const maxDeleteBatch = 1000

// handleDelete removes files or directories (recursively). Admin-only.
// Body: {"paths": ["a", "b", ...]}.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := decodeJSONBody(r, &body, 64<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Paths) == 0 || len(body.Paths) > maxDeleteBatch {
		apiError(w, http.StatusBadRequest, "paths must contain 1 to 1000 entries")
		return
	}
	deleted := make([]string, 0, len(body.Paths))
	for _, p := range body.Paths {
		if !s.canWrite(w, r, p) {
			return
		}
		// RemoveAll-style deletes hide typos; report missing paths as 404.
		if _, err := s.store.Stat(r.Context(), p); err != nil {
			if len(deleted) > 0 {
				writeJSON(w, http.StatusMultiStatus, map[string]any{"deleted": deleted, "failed": p})
				return
			}
			respondErr(w, err)
			return
		}
		if err := s.store.Remove(r.Context(), p); err != nil {
			if len(deleted) > 0 {
				// Partial success is reported as such; the frontend can retry
				// the remainder.
				writeJSON(w, http.StatusMultiStatus, map[string]any{"deleted": deleted, "failed": p})
				return
			}
			respondErr(w, err)
			return
		}
		// Deleted directories take their private marks with them; failures
		// are cosmetic (the folder is gone either way).
		if err := s.authz.DeletePrivateBelow(p); err != nil {
			s.log.Warn("delete: clearing private marks failed", "path", p, "err", err)
		}
		deleted = append(deleted, p)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMove renames or relocates a single entry. Admin-only.
// Body: {"from": "a", "to": "b"}.
func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.From == "" || body.To == "" {
		apiError(w, http.StatusBadRequest, "from and to are required")
		return
	}
	if rejectReservedRoot(w, body.To) {
		return
	}
	if !s.canWrite(w, r, body.From) || !s.canWrite(w, r, body.To) {
		return
	}
	info, err := s.store.Move(r.Context(), body.From, body.To)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrExist):
			apiError(w, http.StatusConflict, "destination already exists")
		case errors.Is(err, os.ErrInvalid):
			apiError(w, http.StatusBadRequest, "invalid move")
		default:
			respondErr(w, err)
		}
		return
	}
	// A moved private folder keeps its privacy under the new name.
	if err := s.authz.RenamePrivate(body.From, body.To); err != nil {
		s.log.Warn("move: rewriting private marks failed", "from", body.From, "to", body.To, "err", err)
	}
	writeJSON(w, http.StatusOK, info)
}

// handleSave streams the request body over an existing or new file.
// Admin/writer or valid short-lived edit token. Query: path.
func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		apiError(w, http.StatusBadRequest, "missing path")
		return
	}
	if rejectReservedRoot(w, path) {
		return
	}
	if !sameOrigin(r) {
		apiError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}

	r, ok := s.authorizeSave(w, r, path)
	if !ok {
		return
	}

	if r.Body == nil {
		apiError(w, http.StatusBadRequest, "missing body")
		return
	}
	maxUp := s.maxUpload()
	limited := http.MaxBytesReader(w, r.Body, maxUp)
	info, err := s.store.Write(r.Context(), path, limited, maxUp)
	if err != nil {
		if tooLarge, ok := errors.AsType[*http.MaxBytesError](err); ok {
			_ = tooLarge
			apiError(w, http.StatusRequestEntityTooLarge, "content exceeds maximum size")
			return
		}
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// authorizeSave authorizes a write to path: either a valid edit token
// minted for that path (anonymous, public policy) or an authenticated
// writer session. Returns the request, possibly with the session
// identity stashed in its context, and whether the write may proceed.
func (s *Server) authorizeSave(w http.ResponseWriter, r *http.Request, path string) (*http.Request, bool) {
	editToken := r.Header.Get("X-Edit-Token")
	if editToken != "" && s.accessPolicy() == policyPublic && s.tokens != nil && s.tokens.VerifyForPath(editToken, path) {
		if s.authz != nil && !s.cfg.Insecure && s.authz.CanRead(authz.Actor{}, path) != nil {
			apiError(w, http.StatusNotFound, "not found")
			return r, false
		}
		return r, true
	}
	id, ok := s.authenticate(w, r)
	if !ok {
		return r, false
	}
	r = withIdentity(r, id)
	return r, s.canWrite(w, r, path)
}

// handleGetPolicy returns the active access policy ("public", "readonly", "private"). Admin-only.
func (s *Server) handleGetPolicy(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"access_policy": s.accessPolicy(),
	})
}

// handleSetPolicy updates the active access policy. Admin-only.
// Body: {"access_policy": "public" | "readonly" | "private"}.
func (s *Server) handleSetPolicy(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil {
		apiError(w, http.StatusInternalServerError, "store unavailable")
		return
	}
	var body struct {
		AccessPolicy string `json:"access_policy"`
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	policy := strings.ToLower(strings.TrimSpace(body.AccessPolicy))
	switch policy {
	case policyPublic, policyReadonly, policyPrivate:
		// valid
	default:
		apiError(w, http.StatusBadRequest, "invalid access policy: must be public, readonly, or private")
		return
	}
	if err := s.appStore.SetAccessPolicy(policy); err != nil {
		respondErr(w, err)
		return
	}
	s.log.Info("access policy updated", "access_policy", policy)
	writeJSON(w, http.StatusOK, map[string]string{
		"access_policy": policy,
	})
}

// SystemDynamicSettings holds runtime configurable system knobs.
type SystemDynamicSettings struct {
	MaxUpload      int64  `json:"max_upload"`
	MaxTextSize    int64  `json:"max_text_size"`
	Guard          bool   `json:"guard"`
	RequestRate    int    `json:"request_rate"`
	DownloadRate   int64  `json:"download_rate"`
	PowDifficulty  int    `json:"pow_difficulty"`
	TrustedProxies string `json:"trusted_proxies"`
}

// SystemInfo holds read-only environment and host runtime details.
type SystemInfo struct {
	Root          string `json:"root"`
	Database      string `json:"database"`
	CacheDir      string `json:"cache_dir"`
	BaseURL       string `json:"base_url"`
	Address       string `json:"address"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	GoVersion     string `json:"go_version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// SystemSettingsResponse bundles dynamic settings and read-only host specs.
type SystemSettingsResponse struct {
	Dynamic SystemDynamicSettings `json:"dynamic"`
	Info    SystemInfo            `json:"info"`
}

// UpdateSystemSettingsRequest represents partial or full modifications to dynamic settings.
type UpdateSystemSettingsRequest struct {
	MaxUpload      *int64  `json:"max_upload"`
	MaxTextSize    *int64  `json:"max_text_size"`
	Guard          *bool   `json:"guard"`
	RequestRate    *int    `json:"request_rate"`
	DownloadRate   *int64  `json:"download_rate"`
	PowDifficulty  *int    `json:"pow_difficulty"`
	TrustedProxies *string `json:"trusted_proxies"`
}

// handleGetSystemSettings returns dynamic settings and read-only host information. Admin-only.
func (s *Server) handleGetSystemSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.getSystemSettingsResponse())
}

func (s *Server) getSystemSettingsResponse() SystemSettingsResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var (
		maxUpload      int64 = 10 << 30
		maxTextSize    int64 = 10 << 20
		guardEnabled         = true
		requestRate          = 60
		downloadRate   int64 = 200 << 20
		powDiff              = 4
		trustedProxies string
		root           string
		dbPath         string
		cacheDir       string
		baseURL        string
		address        string
	)

	if s.cfg != nil {
		maxUpload = s.cfg.MaxUpload
		maxTextSize = s.cfg.MaxTextSize
		guardEnabled = s.cfg.Guard
		requestRate = s.cfg.RequestRate
		downloadRate = s.cfg.DownloadRate
		powDiff = s.cfg.PowDifficulty
		proxies := make([]string, 0, len(s.cfg.TrustedProxies))
		for _, p := range s.cfg.TrustedProxies {
			proxies = append(proxies, p.String())
		}
		trustedProxies = strings.Join(proxies, ", ")
		root = s.cfg.Root
		dbPath = s.cfg.Database
		cacheDir = s.cfg.CacheDir
		baseURL = s.cfg.BaseURL
		address = s.cfg.Address
	}

	uptime := int64(0)
	if !s.startTime.IsZero() {
		uptime = int64(time.Since(s.startTime).Seconds())
	}

	return SystemSettingsResponse{
		Dynamic: SystemDynamicSettings{
			MaxUpload:      maxUpload,
			MaxTextSize:    maxTextSize,
			Guard:          guardEnabled,
			RequestRate:    requestRate,
			DownloadRate:   downloadRate,
			PowDifficulty:  powDiff,
			TrustedProxies: trustedProxies,
		},
		Info: SystemInfo{
			Root:          root,
			Database:      dbPath,
			CacheDir:      cacheDir,
			BaseURL:       baseURL,
			Address:       address,
			Version:       s.version,
			Commit:        s.commit,
			OS:            runtime.GOOS,
			Arch:          runtime.GOARCH,
			GoVersion:     runtime.Version(),
			UptimeSeconds: uptime,
		},
	}
}

func validateSystemSettings(body UpdateSystemSettingsRequest) ([]netip.Prefix, error) {
	if err := validateNumbers(body); err != nil {
		return nil, err
	}
	return parseProxyCIDRs(body.TrustedProxies)
}

func validateNumbers(body UpdateSystemSettingsRequest) error {
	if body.MaxUpload != nil && *body.MaxUpload <= 0 {
		return errors.New("max_upload must be greater than 0")
	}
	if body.MaxTextSize != nil && *body.MaxTextSize <= 0 {
		return errors.New("max_text_size must be greater than 0")
	}
	if body.RequestRate != nil && *body.RequestRate < 1 {
		return errors.New("request_rate must be at least 1")
	}
	if body.DownloadRate != nil && *body.DownloadRate <= 0 {
		return errors.New("download_rate must be greater than 0")
	}
	if body.PowDifficulty != nil && (*body.PowDifficulty < 0 || *body.PowDifficulty > 16) {
		return errors.New("pow_difficulty must be between 0 and 16")
	}
	return nil
}

func parseProxyCIDRs(raw *string) ([]netip.Prefix, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	var prefixes []netip.Prefix
	for cidr := range strings.SplitSeq(trimmed, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy CIDR %q", cidr)
		}
		prefixes = append(prefixes, p.Masked())
	}
	return prefixes, nil
}

func persistSystemSettings(st *store.Store, body UpdateSystemSettingsRequest, prefixes []netip.Prefix) error {
	pairs := []struct {
		key, val string
		set      bool
	}{
		{"max_upload", strconv.FormatInt(deref(body.MaxUpload, 0), 10), body.MaxUpload != nil},
		{"max_text_size", strconv.FormatInt(deref(body.MaxTextSize, 0), 10), body.MaxTextSize != nil},
		{"guard", strconv.FormatBool(deref(body.Guard, false)), body.Guard != nil},
		{"request_rate", strconv.Itoa(deref(body.RequestRate, 0)), body.RequestRate != nil},
		{"download_rate", strconv.FormatInt(deref(body.DownloadRate, 0), 10), body.DownloadRate != nil},
		{"pow_difficulty", strconv.Itoa(deref(body.PowDifficulty, 0)), body.PowDifficulty != nil},
	}
	for _, p := range pairs {
		if p.set {
			if err := st.SetMeta(p.key, p.val); err != nil {
				return err
			}
		}
	}
	if body.TrustedProxies != nil {
		proxyStrings := make([]string, 0, len(prefixes))
		for _, p := range prefixes {
			proxyStrings = append(proxyStrings, p.String())
		}
		if err := st.SetMeta("trusted_proxies", strings.Join(proxyStrings, ", ")); err != nil {
			return err
		}
	}
	return nil
}

func deref[T any](ptr *T, def T) T {
	if ptr == nil {
		return def
	}
	return *ptr
}

func (s *Server) applySystemSettings(body UpdateSystemSettingsRequest, prefixes []netip.Prefix) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg == nil {
		return
	}
	if body.MaxUpload != nil {
		s.cfg.MaxUpload = *body.MaxUpload
		if s.tus != nil {
			s.tus.setMaxSize(*body.MaxUpload)
		}
	}
	if body.MaxTextSize != nil {
		s.cfg.MaxTextSize = *body.MaxTextSize
	}
	if body.Guard != nil {
		s.cfg.Guard = *body.Guard
	}
	if body.RequestRate != nil {
		s.cfg.RequestRate = *body.RequestRate
	}
	if body.DownloadRate != nil {
		s.cfg.DownloadRate = *body.DownloadRate
	}
	if body.PowDifficulty != nil {
		s.cfg.PowDifficulty = *body.PowDifficulty
	}
	if body.TrustedProxies != nil {
		s.cfg.TrustedProxies = prefixes
	}
	if s.guard != nil {
		proxyList := make([]string, 0, len(s.cfg.TrustedProxies))
		for _, p := range s.cfg.TrustedProxies {
			proxyList = append(proxyList, p.String())
		}
		s.guard.UpdateConfig(!s.cfg.Guard, s.cfg.RequestRate, s.cfg.DownloadRate, s.cfg.PowDifficulty, proxyList)
	}
}

// handleUpdateSystemSettings persists dynamic settings updates and updates active runtime. Admin-only.
func (s *Server) handleUpdateSystemSettings(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil {
		apiError(w, http.StatusInternalServerError, "store unavailable")
		return
	}

	var body UpdateSystemSettingsRequest
	if err := decodeJSONBody(r, &body, 64<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prefixes, err := validateSystemSettings(body)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := persistSystemSettings(s.appStore, body, prefixes); err != nil {
		respondErr(w, err)
		return
	}

	s.applySystemSettings(body, prefixes)
	s.log.Info("system settings updated")
	writeJSON(w, http.StatusOK, s.getSystemSettingsResponse())
}
