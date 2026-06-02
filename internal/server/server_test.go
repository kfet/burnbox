package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kfet/burnbox/internal/store"
	"github.com/kfet/burnbox/internal/ui"
)

func newTestServer(t *testing.T, opts store.Options) (*httptest.Server, *store.Store) {
	t.Helper()
	st := store.New(opts)
	t.Cleanup(st.Close)
	ts := httptest.NewServer(New(st, "v0.0.0-test"))
	t.Cleanup(ts.Close)
	return ts, st
}

func put(t *testing.T, base, body, query string) (*http.Response, map[string]string) {
	t.Helper()
	url := base + "/s"
	if query != "" {
		url += "?" + query
	}
	resp, err := http.Post(url, "application/octet-stream", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	var m map[string]string
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = json.Unmarshal(b, &m)
	return resp, m
}

func TestHealth(t *testing.T) {
	ts, _ := newTestServer(t, store.Options{})
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(b) != "ok" {
		t.Fatalf("health = %d %q", resp.StatusCode, b)
	}
}

func TestVersion(t *testing.T) {
	ts, _ := newTestServer(t, store.Options{})
	resp, err := http.Get(ts.URL + "/version")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.StatusCode != 200 || m["version"] != "v0.0.0-test" {
		t.Fatalf("version = %d %q", resp.StatusCode, m["version"])
	}
}

func TestPutGetBurn(t *testing.T) {
	ts, _ := newTestServer(t, store.Options{})
	resp, m := put(t, ts.URL, "deadbeefblob", "ttl=3600")
	if resp.StatusCode != 200 {
		t.Fatalf("PUT status %d", resp.StatusCode)
	}
	id := m["id"]
	if id == "" {
		t.Fatal("no id returned")
	}

	// First GET returns the blob as octet-stream and burns it.
	g, err := http.Get(ts.URL + "/s/" + id)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(g.Body)
	g.Body.Close()
	if g.StatusCode != 200 {
		t.Fatalf("GET status %d", g.StatusCode)
	}
	if ct := g.Header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("content-type %q", ct)
	}
	if string(body) != "deadbeefblob" {
		t.Fatalf("blob mismatch: %q", body)
	}

	// Second GET is a 404 (burned).
	g2, err := http.Get(ts.URL + "/s/" + id)
	if err != nil {
		t.Fatal(err)
	}
	if g2.StatusCode != 404 {
		t.Fatalf("second GET = %d, want 404", g2.StatusCode)
	}
	g2.Body.Close()
}

func TestPutEmptyBody(t *testing.T) {
	ts, _ := newTestServer(t, store.Options{})
	resp, m := put(t, ts.URL, "", "")
	if resp.StatusCode != 400 {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
	if m["error"] == "" {
		t.Fatal("want error message")
	}
}

func TestPutTooLarge(t *testing.T) {
	ts, _ := newTestServer(t, store.Options{MaxSize: 8})
	resp, _ := put(t, ts.URL, "this is definitely more than eight bytes", "")
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", resp.StatusCode)
	}
}

func TestPutTTLClampingDefault(t *testing.T) {
	// Bad / missing ttl falls through to the store default; just assert
	// the request still succeeds across the parse branches.
	ts, _ := newTestServer(t, store.Options{})
	for _, q := range []string{"", "ttl=abc", "ttl=-5", "ttl=0", "ttl=60"} {
		resp, m := put(t, ts.URL, "x", q)
		if resp.StatusCode != 200 || m["id"] == "" {
			t.Fatalf("q=%q status=%d", q, resp.StatusCode)
		}
	}
}

func TestGetNotFound(t *testing.T) {
	ts, _ := newTestServer(t, store.Options{})
	g, err := http.Get(ts.URL + "/s/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Body.Close()
	if g.StatusCode != 404 {
		t.Fatalf("status %d", g.StatusCode)
	}
}

func TestPages(t *testing.T) {
	ts, _ := newTestServer(t, store.Options{})
	cases := []struct {
		path, wantCT, wantSub string
	}{
		{"/", "text/html; charset=utf-8", "burn"},
		{"/burnbox.js", "text/javascript; charset=utf-8", "AES-CTR"},
		{"/recipe.js", "text/javascript; charset=utf-8", "openssl"},
		{"/r/someid", "text/html; charset=utf-8", "recipe"},
	}
	for _, c := range cases {
		resp, err := http.Get(ts.URL + c.path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s status %d", c.path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != c.wantCT {
			t.Fatalf("%s ct %q", c.path, ct)
		}
		if !bytes.Contains(b, []byte(c.wantSub)) {
			t.Fatalf("%s body missing %q", c.path, c.wantSub)
		}
		if csp := resp.Header.Get("Content-Security-Policy"); csp == "" {
			t.Fatalf("%s missing Content-Security-Policy", c.path)
		}
		if resp.Header.Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("%s missing Referrer-Policy", c.path)
		}
	}
}

func TestUnknownPath404(t *testing.T) {
	ts, _ := newTestServer(t, store.Options{})
	resp, err := http.Get(ts.URL + "/nope/whatever")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}

func TestPutStoreError(t *testing.T) {
	// Force store.Put to fail via a randID error so handlePut's 500
	// branch is exercised.
	st := store.New(store.Options{RandID: func() (string, error) {
		return "", io.ErrUnexpectedEOF
	}})
	t.Cleanup(st.Close)
	ts := httptest.NewServer(New(st, "v0.0.0-test"))
	t.Cleanup(ts.Close)
	resp, m := put(t, ts.URL, "x", "")
	if resp.StatusCode != 500 {
		t.Fatalf("status %d, want 500", resp.StatusCode)
	}
	if m["error"] == "" {
		t.Fatal("want error body")
	}
}

func TestParseTTL(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"abc", 0},
		{"-1", 0},
		{"0", 0},
		{"90", 90 * time.Second},
	}
	for _, c := range cases {
		if got := parseTTL(c.in); got != c.want {
			t.Fatalf("parseTTL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestPutBodyReadError(t *testing.T) {
	st := store.New(store.Options{})
	t.Cleanup(st.Close)
	srv := New(st, "v0.0.0-test")
	req := httptest.NewRequest("POST", "/s", errReader{})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "read failed") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

// TestCSPMatchesInlineBootstrap recomputes the sha256 of the inline
// <script> in index.html and asserts the CSP pins exactly that hash, so
// the bootstrap and the policy can never silently drift apart.
func TestCSPMatchesInlineBootstrap(t *testing.T) {
	html := string(ui.Index)
	const open = "<script>"
	i := strings.Index(html, open)
	if i < 0 {
		t.Fatal("no inline <script> in index.html")
	}
	j := strings.Index(html[i:], "</script>")
	if j < 0 {
		t.Fatal("unterminated inline <script>")
	}
	body := html[i+len(open) : i+j]
	sum := sha256.Sum256([]byte(body))
	want := "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
	if !strings.Contains(contentSecurityPolicy, want) {
		t.Fatalf("CSP missing inline-script hash %q\nCSP: %s", want, contentSecurityPolicy)
	}
}
