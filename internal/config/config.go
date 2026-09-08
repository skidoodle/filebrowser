package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const prefix = "FILEBROWSER_"

// Config holds the full runtime configuration of the server.
type Config struct {
	// Root is the filesystem directory served to guests.
	Root string
	// Address is the listen address, e.g. "127.0.0.1:8080".
	Address string
	// BaseURL is an optional subpath the app is served under, e.g. "/files".
	BaseURL string
	// MaxUpload is the maximum size in bytes a single uploaded file may have.
	MaxUpload int64
	// CacheDir stores disposable data such as image thumbnails.
	CacheDir string
	// TrustedProxies are CIDRs of reverse proxies whose X-Forwarded-For
	// headers are honored when determining client IPs.
	TrustedProxies []netip.Prefix
	// MaxTextSize is the largest file (bytes) still detected/served as text.
	MaxTextSize int64
	// Guard toggles the abuse-protection middleware chain.
	Guard bool
	// RequestRate is the per-IP API request budget per second.
	RequestRate int
	// DownloadRate is the per-IP raw-download budget in bytes per second.
	DownloadRate int64
	// PowDifficulty is the number of leading hex zeros required in a
	// proof-of-work solution (0 disables challenges).
	PowDifficulty int
	// Secret overrides the capability signing key. When empty, a key is
	// persisted in CacheDir so capability tokens survive restarts.
	Secret string
	// Insecure disables admin authentication entirely: every visitor has
	// full write access. Intended for local deployments only.
	Insecure bool
	// AdminPassword seeds (or resets) the admin account password. When
	// empty the persisted password hash is left untouched.
	AdminPassword string
	// Dev enables verbose logging and relaxed defaults for development.
	Dev bool
}

// FromEnv builds a Config from FILEBROWSER_* environment variables.
func FromEnv() (*Config, error) {
	cfg := &Config{
		Root:          getenv("ROOT", "./data"),
		Address:       getenv("ADDRESS", "0.0.0.0:8080"),
		BaseURL:       getenv("BASEURL", ""),
		MaxUpload:     10 << 30,
		CacheDir:      getenv("CACHEDIR", filepath.Join(os.TempDir(), "filebrowser-cache")),
		MaxTextSize:   10 << 20,
		Guard:         getbool("GUARD", true),
		RequestRate:   60,
		DownloadRate:  200 << 20, // 200 MiB/s per IP
		PowDifficulty: 4,
		Secret:        getenv("SECRET", ""),
		Insecure:      getbool("INSECURE", false),
		AdminPassword: getenv("ADMIN_PASSWORD", ""),
		Dev:           getbool("DEBUG", false),
	}

	var err error
	if err = cfg.parseSizes(); err != nil {
		return nil, err
	}
	if err = cfg.parseGuardLimits(); err != nil {
		return nil, err
	}
	if err = cfg.parseTrustedProxies(); err != nil {
		return nil, err
	}

	cfg.normalizeBaseURL()

	return cfg, nil
}

// parseSizes reads byte-sized settings.
func (c *Config) parseSizes() error {
	if v, ok := lookup("MAXUPLOAD"); ok {
		n, err := parseBytes(v)
		if err != nil {
			return fmt.Errorf("config: FILEBROWSER_MAXUPLOAD: %w", err)
		}

		c.MaxUpload = n
	}
	if v, ok := lookup("MAXTEXTSIZE"); ok {
		n, err := parseBytes(v)
		if err != nil {
			return fmt.Errorf("config: FILEBROWSER_MAXTEXTSIZE: %w", err)
		}

		c.MaxTextSize = n
	}

	return nil
}

// parseGuardLimits reads the abuse-protection knobs.
func (c *Config) parseGuardLimits() error {
	if v, ok := lookup("REQUESTRATE"); ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return fmt.Errorf("config: FILEBROWSER_REQUESTRATE: invalid %q", v)
		}

		c.RequestRate = n
	}
	if v, ok := lookup("DOWNLOADRATE"); ok {
		n, err := parseBytes(v)
		if err != nil {
			return fmt.Errorf("config: FILEBROWSER_DOWNLOADRATE: %w", err)
		}

		c.DownloadRate = n
	}
	if v, ok := lookup("POWDIFFICULTY"); ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 16 {
			return fmt.Errorf("config: FILEBROWSER_POWDIFFICULTY: invalid %q", v)
		}

		c.PowDifficulty = n
	}

	return nil
}

// parseTrustedProxies reads the proxy CIDR list.
func (c *Config) parseTrustedProxies() error {
	for cidr := range strings.SplitSeq(getenv("TRUSTEDPROXIES", ""), ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			return fmt.Errorf("config: FILEBROWSER_TRUSTEDPROXIES: %w", err)
		}
		c.TrustedProxies = append(c.TrustedProxies, p.Masked())
	}
	return nil
}

// normalizeBaseURL ensures the base URL is a clean subpath.
func (c *Config) normalizeBaseURL() {
	if c.BaseURL != "" && !strings.HasPrefix(c.BaseURL, "/") {
		c.BaseURL = "/" + c.BaseURL
	}
	c.BaseURL = strings.TrimSuffix(c.BaseURL, "/")
}

func lookup(key string) (string, bool) {
	v, ok := os.LookupEnv(prefix + key)
	if !ok || strings.TrimSpace(v) == "" {
		return "", false
	}
	return strings.TrimSpace(v), true
}

func getenv(key, def string) string {
	if v, ok := lookup(key); ok {
		return v
	}
	return def
}

func getbool(key string, def bool) bool {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// parseBytes parses byte sizes with optional binary SI suffixes
// ("512KiB", "10MiB", "1GiB", "2TiB", or plain byte counts).
func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty value")
	}
	lower := strings.ToLower(s)
	units := []struct {
		suffix string
		mult   int64
	}{
		{"kib", 1 << 10},
		{"mib", 1 << 20},
		{"gib", 1 << 30},
		{"tib", 1 << 40},
		{"kb", 1 << 10},
		{"mb", 1 << 20},
		{"gb", 1 << 30},
		{"tb", 1 << 40},
	}
	for _, u := range units {
		if rest, found := strings.CutSuffix(lower, u.suffix); found {
			n, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size %q", s)
			}
			return int64(n * float64(u.mult)), nil
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return n, nil
}
