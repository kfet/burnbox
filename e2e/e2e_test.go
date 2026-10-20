// Package e2e holds burnbox's end-to-end smoke test. It proves the core
// promise: a blob produced by the frozen v1 client crypto can be
// decrypted on a bare box using only curl + python3 (stdlib) only,
// and that reading burns the secret.
//
// Run via `make e2e` (sets E2E=1). Skipped otherwise.
package e2e

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"github.com/kfet/burnbox/internal/contract"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/kfet/burnbox/internal/server"
	"github.com/kfet/burnbox/internal/store"
)

func b64u(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func hmacSHA256(key, msg []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(msg)
	return m.Sum(nil)
}

// encryptV1 performs the frozen client-side v1 contract and returns the
// stored blob (base64url) and the master key (base64url).
func encryptV1(t *testing.T, plaintext string) (blob, key string) {
	t.Helper()
	master := make([]byte, 32)
	iv := make([]byte, 16)
	if _, err := rand.Read(master); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(iv); err != nil {
		t.Fatal(err)
	}
	ek := hmacSHA256(master, []byte(contract.LabelEnc))
	mk := hmacSHA256(master, []byte(contract.LabelMAC))

	block, err := aes.NewCipher(ek)
	if err != nil {
		t.Fatal(err)
	}
	ct := make([]byte, len(plaintext))
	cipher.NewCTR(block, iv).XORKeyStream(ct, []byte(plaintext))
	tag := hmacSHA256(mk, append(append([]byte{}, iv...), ct...))

	raw := append(append(append([]byte{}, iv...), ct...), tag...)
	return b64u(raw), b64u(master)
}

func TestE2E(t *testing.T) {
	if os.Getenv("E2E") == "" {
		t.Skip("set E2E=1 to run (make e2e)")
	}

	st := store.New(store.Options{})
	defer st.Close()
	ts := httptest.NewServer(server.New(st, "v0.0.0-test"))
	defer ts.Close()

	const secret = "correct horse battery staple\nline two\twith tab"
	blob, key := encryptV1(t, secret)

	// POST the opaque blob.
	resp, err := http.Post(ts.URL+"/s?ttl=3600", "application/octet-stream", strings.NewReader(blob))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("POST /s = %d: %s", resp.StatusCode, body)
	}
	id := extractID(t, string(body))

	// Pages serve and contain their markers.
	assertContains(t, httpGet(t, ts.URL+"/"), "burnbox")
	assertContains(t, httpGet(t, ts.URL+"/burnbox.js"), "AES-CTR")
	assertContains(t, httpGet(t, ts.URL+"/r/"+id), "recipe")

	// THE point: decrypt via the real bare-OS recipient path.
	got := decryptViaShell(t, ts.URL+"/s/"+id, key)
	if got != secret {
		t.Fatalf("shell decrypt mismatch:\n got %q\nwant %q", got, secret)
	}

	// Burned: second GET is 404.
	r2, err := http.Get(ts.URL + "/s/" + id)
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.StatusCode != 404 {
		t.Fatalf("second GET = %d, want 404 (burned)", r2.StatusCode)
	}
}

// decryptViaShell runs the exact bare-OS recipient pipeline, byte-for-byte
// the form the recipe page generates (the Python is
// extracted from recipe.js itself, so there is a single source of truth):
//
//	curl -s URL | python3 -c '<stdlib hmac verify + pure-Python AES-256-CTR>' KEY
//
// The key is passed as a python argv — NOT via the environment — so this
// test fails if anyone reintroduces an env-var dependency (e.g. the old
// `KEY=... curl ... | python3` form, where KEY never reached python).
// It skips (not fails) if curl/python3 are unavailable.
func decryptViaShell(t *testing.T, url, key string) string {
	t.Helper()
	for _, bin := range []string{"bash", "curl", "python3"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH — skipping bare-OS recipient check", bin)
		}
	}
	py := recipePy(t)

	// Key as argv ($3), and deliberately NOT in the environment, to mirror
	// the real recipe and guard against the env-prefix footgun.
	cmd := exec.Command("bash", "-c", `curl -s "$1" | python3 -c "$2" "$3"`, "_", url, py, key)
	cmd.Env = append(os.Environ(), "KEY=") // prove python does not read KEY
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("shell pipeline failed: %v\nstderr: %s", err, errb.String())
	}
	return out.String()
}

func httpGet(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s = %d", url, resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("missing %q", needle)
	}
}

func extractID(t *testing.T, jsonBody string) string {
	t.Helper()
	// minimal parse: {"id":"..."}
	const k = `"id":"`
	i := strings.Index(jsonBody, k)
	if i < 0 {
		t.Fatalf("no id in %q", jsonBody)
	}
	rest := jsonBody[i+len(k):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		t.Fatalf("malformed id in %q", jsonBody)
	}
	return rest[:j]
}
