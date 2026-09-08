package guard

import (
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// Config controls the guard chain.
type Config struct {
	// TrustedProxies are CIDRs whose X-Forwarded-For headers are honored.
	TrustedProxies []string
	// RequestRate is the per-IP API request budget per second.
	RequestRate int
	// DownloadRate is the per-IP raw-download budget (bytes per second).
	DownloadRate int64
	// Difficulty is the proof-of-work leading-zero count; 0 disables.
	Difficulty int
	// Disabled turns the whole chain off (for development).
	Disabled bool
	// Secret overrides the capability signing key (hashed). When empty the
	// key is persisted in CacheDir so tokens survive restarts.
	Secret string
	// CacheDir holds the persisted capability key when Secret is empty.
	CacheDir string
}

// Guard orchestrates the abuse-protection middleware chain.
type Guard struct {
	cfg    Config
	req    *limiter
	dl     *limiter
	banned *bans
	Tokens *Tokens
	log    logger
}

// logger keeps guard decoupled from slog.
type logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

// New builds the guard and starts its background janitors.
func New(cfg Config, log logger) (*Guard, error) {
	key, err := ResolveKey(cfg.Secret, cfg.CacheDir)
	if err != nil {
		return nil, err
	}
	g := &Guard{
		cfg:    cfg,
		req:    newLimiter(float64(cfg.RequestRate), float64(cfg.RequestRate)*2),
		dl:     newLimiter(float64(cfg.DownloadRate), float64(cfg.DownloadRate)),
		banned: newBans(),
		Tokens: NewTokensWithKey(key),
		log:    log,
	}
	g.req.startJanitor(5*time.Minute, 15*time.Minute)
	g.dl.startJanitor(5*time.Minute, 15*time.Minute)
	g.banned.startJanitor(time.Minute)
	return g, nil
}

// honeypotPaths are decoys no legitimate file browser ever serves; hitting
// one is instant-ban material because only scanners look for them.
var honeypotPaths = map[string]bool{
	"/wp-login.php": true, "/wp-admin": true, "/xmlrpc.php": true,
	"/.env": true, "/.git/config": true, "/.git/head": true,
	"/.aws/credentials": true, "/phpmyadmin": true, "/admin/config.php": true,
	"/cgi-bin/": true, "/.ds_store": true, "/config.json.bak": true,
	"/actuator/health": true, "/.svn/entries": true,
}

// suspiciousUA matches common scripted clients and bots.
var suspiciousUA = []string{
	"curl/", "wget", "python-requests", "python-urllib", "scrapy",
	"go-http-client", "java/", "apache-httpclient", "libwww", "httpclient",
	"bot", "crawler", "spider", "scanner", "headless",
}

// ipKey returns the rate-limit/ban key for a request.
func (g *Guard) ipKey(r *http.Request) string {
	trusted := make([]netip.Prefix, 0, len(g.cfg.TrustedProxies))
	for _, c := range g.cfg.TrustedProxies {
		if p, err := netip.ParsePrefix(c); err == nil {
			trusted = append(trusted, p.Masked())
		}
	}
	return ClientIP(r, trusted).String()
}

// Middleware is the outer chain: honeypot → ban → rate limit.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	if g.cfg.Disabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := g.ipKey(r)

		if honeypot(r.URL.Path) {
			g.banned.Ban(key, 24*time.Hour)
			g.log.Warn("honeypot hit, banning", "ip", key, "path", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		if banned, remaining := g.banned.Banned(key); banned {
			w.Header().Set("Retry-After", remaining.Round(time.Second).String())
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		if !g.req.allow(key, 1) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		if suspiciousUserAgent(r.UserAgent()) {
			g.banned.Strike(key)
		}

		next.ServeHTTP(w, r)
	})
}

// RequireCapability wraps mutating handlers: they need a valid
// X-Capability header minted by the SPA.
func (g *Guard) RequireCapability(next http.Handler) http.Handler {
	if g.cfg.Disabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Capability")
		if !g.Tokens.Verify(token) {
			w.Header().Set("X-Capability", "challenge")
			http.Error(w, "capability required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AllowDownload charges n bytes against the IP's download budget. Large
// payloads consume the budget as it refills rather than being rejected for
// exceeding the burst, so big files download reliably at the configured rate.
func (g *Guard) AllowDownload(r *http.Request, n int64) bool {
	if g.cfg.Disabled || n <= 0 {
		return true
	}
	key := g.ipKey(r)
	const floor = 64 << 10 // reject only when the bucket is nearly drained
	if !g.dl.allowPartial(key, float64(n), floor) {
		g.banned.Strike(key)
		return false
	}
	return true
}

func honeypot(p string) bool {
	lp := strings.ToLower(p)
	if honeypotPaths[lp] {
		return true
	}
	for prefix := range honeypotPaths {
		if strings.HasSuffix(prefix, "/") && strings.HasPrefix(lp, prefix) {
			return true
		}
	}
	return false
}

func suspiciousUserAgent(ua string) bool {
	if ua == "" {
		return true
	}
	l := strings.ToLower(ua)
	for _, s := range suspiciousUA {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// SuspiciousUserAgent exports the bot heuristic for the capability flow.
func SuspiciousUserAgent(ua string) bool { return suspiciousUserAgent(ua) }

// PowDifficulty returns the configured proof-of-work difficulty.
func (g *Guard) PowDifficulty() int { return g.cfg.Difficulty }
