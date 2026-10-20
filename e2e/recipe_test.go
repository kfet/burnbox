package e2e

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"github.com/kfet/burnbox/internal/contract"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// recipePy extracts the recipient Python program from recipe.js by
// concatenating the JS string literals of `var py = ...;`. Testing this
// (not a copy) guarantees the shipped recipe is what gets verified.
func recipePy(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("../internal/ui/assets/recipe.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	i := strings.Index(s, "var py =")
	if i < 0 {
		t.Fatal("var py not found in recipe.js")
	}
	s = s[i+len("var py ="):]
	s = s[:strings.Index(s, "\";\n")+1]
	var b strings.Builder
	for _, part := range strings.Split(s, "\" +\n") {
		part = strings.TrimSpace(part)
		if !strings.HasSuffix(part, "\"") {
			part += "\""
		}
		u, err := strconv.Unquote(part)
		if err != nil {
			t.Fatalf("unquote %q: %v", part, err)
		}
		b.WriteString(u)
	}
	py := b.String()
	if strings.Contains(py, "'") {
		t.Fatal("recipe Python must not contain single quotes (shell-quoted)")
	}
	for _, bad := range []string{"openssl", "subprocess"} {
		if strings.Contains(py, bad) {
			t.Fatalf("recipe Python must not use %s", bad)
		}
	}
	return py
}

func needPython(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
}

func runPy(t *testing.T, py string, stdin string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("python3", append([]string{"-c", py}, args...)...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("python failed: %v\nstderr: %s", err, errb.String())
	}
	return out.Bytes()
}

// encryptV1IV is encryptV1 with a caller-chosen IV (for counter-carry cases).
func encryptV1IV(t *testing.T, pt, iv []byte) (blob, key string) {
	t.Helper()
	master := make([]byte, 32)
	if _, err := rand.Read(master); err != nil {
		t.Fatal(err)
	}
	ek := hmacSHA256(master, []byte(contract.LabelEnc))
	mk := hmacSHA256(master, []byte(contract.LabelMAC))
	block, _ := aes.NewCipher(ek)
	ct := make([]byte, len(pt))
	cipher.NewCTR(block, iv).XORKeyStream(ct, pt)
	tag := hmacSHA256(mk, append(append([]byte{}, iv...), ct...))
	return b64u(append(append(append([]byte{}, iv...), ct...), tag...)), b64u(master)
}

// TestRecipeDecryptsGo proves the pure-Python recipe decrypts payloads
// produced by Go's crypto/aes CTR for awkward sizes and counter carries.
func TestRecipeDecryptsGo(t *testing.T) {
	needPython(t)
	py := recipePy(t)
	ff := bytes.Repeat([]byte{0xff}, 16)
	low := append(bytes.Repeat([]byte{0x11}, 8), 0, 0, 0, 0, 0, 0, 0xff, 0x80) // low bytes carry after 128 blocks
	rnd := make([]byte, 16)
	_, _ = rand.Read(rnd)
	cases := []struct {
		name string
		n    int
		iv   []byte
	}{
		{"empty", 0, rnd},
		{"one byte", 1, rnd},
		{"one block", 16, rnd},
		{"non-aligned multi", 16*5 + 7, rnd},
		{"wrap 2^128", 16*3 + 3, ff},
		{">256 blocks carry", 16*300 + 9, low},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pt := make([]byte, c.n)
			_, _ = rand.Read(pt)
			blob, key := encryptV1IV(t, pt, c.iv)
			if got := runPy(t, py, blob, key); !bytes.Equal(got, pt) {
				t.Fatalf("mismatch: got %d bytes, want %d", len(got), len(pt))
			}
		})
	}
}

// TestRecipeAESKnownAnswer checks the recipe's AES-256 block cipher against
// FIPS-197 Appendix C.3. With CTR, decrypting a zero block at counter P
// yields E(K, P); only the KDF line is swapped to inject K; the MAC path runs for real.
func TestRecipeAESKnownAnswer(t *testing.T) {
	needPython(t)
	py := recipePy(t)
	const (
		k  = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
		p  = "00112233445566778899aabbccddeeff"
		ct = "8ea2b7ca516745bfeafc49904b496089"
	)
	kdf := `H(m,b"` + contract.LabelEnc + `")`
	if !strings.Contains(py, kdf) {
		t.Fatal("recipe KDF line changed; update KAT harness")
	}
	py = strings.Replace(py, kdf, `bytes.fromhex("`+k+`")`, 1)
	master := []byte("kat-master")
	iv := append(mustHex(p), make([]byte, 16)...)
	tag := hmacSHA256(hmacSHA256(master, []byte(contract.LabelMAC)), iv)
	blob := b64u(append(iv, tag...))
	if got := hex.EncodeToString(runPy(t, py, blob, b64u(master))); got != ct {
		t.Fatalf("AES-256 KAT: got %s want %s", got, ct)
	}
}

func mustHex(s string) []byte { b, _ := hex.DecodeString(s); return b }

// TestRecipeTamperedUnderO: the MAC check must not be an assert — under
// python3 -O asserts vanish. Tampered ciphertext must exit non-zero with
// nothing on stdout.
func TestRecipeTamperedUnderO(t *testing.T) {
	needPython(t)
	py := recipePy(t)
	blob, key := encryptV1IV(t, []byte("top secret plaintext"), make([]byte, 16))
	raw, err := base64.RawURLEncoding.DecodeString(blob)
	if err != nil {
		t.Fatal(err)
	}
	raw[16] ^= 1 // flip a ciphertext bit
	cmd := exec.Command("python3", "-O", "-c", py, key)
	cmd.Stdin = strings.NewReader(b64u(raw))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err == nil {
		t.Fatalf("tampered blob decrypted under -O: %q", out.String())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout not empty: %q", out.String())
	}
	if !strings.Contains(errb.String(), "bad MAC") {
		t.Fatalf("stderr = %q, want bad MAC", errb.String())
	}
}
