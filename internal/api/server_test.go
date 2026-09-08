package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/skidoodle/filebrowser/internal/config"
)

func TestSPAHandlerServesApplicationRoutes(t *testing.T) {
	t.Parallel()

	web := fstest.MapFS{
		"index.html": {Data: []byte("app shell")},
	}
	handler := (&Server{web: web}).spaHandler()

	routes := []string{
		"/",
		"/sample%20image",
		"/view/sample%20image/sample.webp",
		"/settings/users",
		"/login",
		"/setup",
		"/new/folder/sample%20image",
		"/new/file/sample%20image",
	}
	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, route, nil)
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
			}
			if body := res.Body.String(); body != "app shell" {
				t.Fatalf("body = %q, want app shell", body)
			}
			if csp := res.Header().Get("Content-Security-Policy"); csp == "" {
				t.Fatal("missing Content-Security-Policy header")
			}
		})
	}
}

func TestSPAHandlerConfiguresBaseURL(t *testing.T) {
	t.Parallel()

	web := fstest.MapFS{
		"index.html": {
			Data: []byte(`<meta name="filebrowser-base" content=""><script src="/assets/app.js"></script>`),
		},
		"assets/app.js": {Data: []byte("app javascript")},
	}
	s := &Server{
		cfg: &config.Config{BaseURL: "/share"},
		web: web,
	}
	mux := http.NewServeMux()
	mux.Handle("/", s.spaHandler())
	handler := http.StripPrefix(s.cfg.BaseURL, mux)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/share/view/docs/readme.txt", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("route status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, `name="filebrowser-base" content="/share"`) {
		t.Fatalf("index is missing base URL metadata: %q", body)
	}
	if !strings.Contains(body, `src="/share/assets/app.js"`) {
		t.Fatalf("index asset URL is missing base URL: %q", body)
	}

	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/share/assets/app.js", nil)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if body := res.Body.String(); body != "app javascript" {
		t.Fatalf("asset body = %q, want app javascript", body)
	}
}
