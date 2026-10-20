package e2e

import (
	"crypto/aes"
	"os"
	"strings"
	"testing"

	"github.com/kfet/burnbox/internal/contract"
)

// Drift guard: the human-facing description of the crypto (pages, README)
// and every implementation (browser JS, terminal recipe, Go encryptors)
// must agree with internal/contract. Changing the cipher, MAC or a KDF
// label anywhere without updating the rest fails here.

// impl maps each contract name to the exact token each implementation uses
// for it. A new Cipher/MAC value has no entry, which fails the test until
// someone deliberately updates every implementation and this table.
var impl = map[string]struct{ js, py []string }{
	"AES-256-CTR": {
		js: []string{`name: "AES-CTR", counter: iv, length: 128`, `new Uint8Array(32)`},
		py: []string{`range(15)`, `.to_bytes(17,"big")[1:]`}, // 14 rounds = AES-256; 128-bit counter
	},
	"HMAC-SHA256": {
		js: []string{`{ name: "HMAC", hash: "SHA-256" }`},
		py: []string{`from hmac import*`, `digest(k,x,"sha256")`, `compare_digest(`},
	},
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestContractPagesDescribeCrypto(t *testing.T) {
	want := []string{contract.Cipher, contract.MAC, contract.LabelEnc, contract.LabelMAC}
	for _, p := range []string{
		"../internal/ui/assets/index.html",
		"../internal/ui/assets/recipe.html",
		"../README.md",
		"../AGENTS.md",
	} {
		s := readFile(t, p)
		for _, w := range want {
			if !strings.Contains(s, w) {
				t.Errorf("%s does not mention %q", p, w)
			}
		}
		if !strings.Contains(strings.ToLower(s), strings.ToLower(contract.Construction)) {
			t.Errorf("%s does not mention %q", p, contract.Construction)
		}
	}
	// The two pages carry the same exact sentence, outside any element the
	// ?ai variant rewrites.
	for _, p := range []string{"../internal/ui/assets/index.html", "../internal/ui/assets/recipe.html"} {
		s := readFile(t, p)
		sentence := "<strong>" + contract.Cipher + " + " + contract.MAC + "</strong> (" + contract.Construction + ")"
		if !strings.Contains(s, sentence) || !strings.Contains(s, "never sent to the server") {
			t.Errorf("%s missing crypto sentence %q", p, sentence)
		}
	}
	if strings.Contains(readFile(t, "../internal/ui/assets/recipe.js"), `getElementById("crypto")`) {
		t.Error("recipe.js must not rewrite the crypto sentence")
	}
}

func TestContractImplementationsAgree(t *testing.T) {
	js := readFile(t, "../internal/ui/assets/burnbox.js")
	py := recipePy(t)
	for _, name := range []string{contract.Cipher, contract.MAC} {
		m, ok := impl[name]
		if !ok {
			t.Fatalf("contract %q has no implementation mapping — update burnbox.js, recipe.js, the Go encryptors and this table", name)
		}
		for _, tok := range m.js {
			if !strings.Contains(js, tok) {
				t.Errorf("burnbox.js missing %q for %s", tok, name)
			}
		}
		for _, tok := range m.py {
			if !strings.Contains(py, tok) {
				t.Errorf("recipe Python missing %q for %s", tok, name)
			}
		}
	}
	for _, l := range []string{contract.LabelEnc, contract.LabelMAC} {
		if !strings.Contains(js, `enc.encode("`+l+`")`) {
			t.Errorf("burnbox.js does not derive with label %q", l)
		}
		if !strings.Contains(py, `b"`+l+`"`) {
			t.Errorf("recipe Python does not derive with label %q", l)
		}
		if !strings.Contains(readFile(t, "../skills/deploy/enc.go"), `"`+l+`"`) {
			t.Errorf("skills/deploy/enc.go does not use label %q", l)
		}
	}
	// No stray labels of another version anywhere in the clients.
	for _, s := range []string{js, py} {
		rest := strings.NewReplacer(contract.LabelEnc, "", contract.LabelMAC, "").Replace(s)
		if strings.Contains(rest, "burnbox/v") {
			t.Error("client contains a KDF label other than the contract's")
		}
	}
	// The Go encryptors use HMAC-SHA256 subkeys (32 bytes) -> AES-256.
	if k := hmacSHA256([]byte("m"), []byte(contract.LabelEnc)); len(k) != 32 {
		t.Fatalf("ek len %d", len(k))
	} else if _, err := aes.NewCipher(k); err != nil {
		t.Fatal(err)
	}
}
