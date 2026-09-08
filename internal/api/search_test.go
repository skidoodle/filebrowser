package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestSearchStreamsThroughMiddleware(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 1<<20)
	ctx := t.Context()

	for _, p := range []string{"docs/black.png", "docs/blackish.txt", "docs/white.txt"} {
		if _, err := store.CreateFile(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	// Regression: the logging middleware's statusWriter used to hide
	// http.Flusher, making the streaming search 500 with
	// "streaming unsupported" behind the full handler chain.
	res := testGet(t, ts, ts.URL+"/api/search?q=black")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-ndjson") {
		t.Fatalf("content type = %q", ct)
	}
	body := mustRead(t, res.Body)
	if n := strings.Count(string(body), "\n"); n != 2 {
		t.Fatalf("expected 2 NDJSON lines for q=black, got %d (%q)", n, body)
	}
	if !strings.Contains(string(body), "docs/black.png") {
		t.Fatalf("missing match in %q", body)
	}
}

func TestSearchRespectsLimitAndType(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 1<<20)
	ctx := t.Context()

	for _, p := range []string{"d/a.png", "d/b.png", "d/c.png", "d/d.txt"} {
		if _, err := store.CreateFile(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	res := testGet(t, ts, ts.URL+"/api/search?q=&type=image&limit=2")
	defer func() { _ = res.Body.Close() }()
	body := string(mustRead(t, res.Body))
	if n := strings.Count(body, "\n"); n != 2 {
		t.Fatalf("limit=2 type=image: got %d lines (%q)", n, body)
	}
	if strings.Contains(body, "d/d.txt") {
		t.Fatalf("type filter leaked: %q", body)
	}
}
