//go:build ignore

// Encrypts stdin with the v1 contract; prints blob then key. Used by live-verify.sh.
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

func h(k, m []byte) []byte { x := hmac.New(sha256.New, k); x.Write(m); return x.Sum(nil) }
func main() {
	pt, _ := io.ReadAll(os.Stdin)
	m := make([]byte, 32)
	iv := make([]byte, 16)
	rand.Read(m)
	rand.Read(iv)
	b, _ := aes.NewCipher(h(m, []byte("burnbox/v1/enc")))
	ct := make([]byte, len(pt))
	cipher.NewCTR(b, iv).XORKeyStream(ct, pt)
	t := h(h(m, []byte("burnbox/v1/mac")), append(append([]byte{}, iv...), ct...))
	e := base64.RawURLEncoding
	fmt.Println(e.EncodeToString(append(append(append([]byte{}, iv...), ct...), t...)))
	fmt.Println(e.EncodeToString(m))
}
