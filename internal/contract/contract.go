// Package contract is the single source of truth for the names and
// labels of burnbox's frozen v1 client-side crypto contract (see
// AGENTS.md). It holds strings only — the Go server never encrypts.
//
// e2e/contract_test.go asserts that the user-facing pages, the README,
// the browser client, the terminal recipe and the Go test encryptors all
// agree with these values, so changing the cipher or a label without
// updating the page text fails the build.
package contract

const (
	// Cipher is the symmetric cipher (256-bit key, full 128-bit counter).
	Cipher = "AES-256-CTR"
	// MAC is the authenticator, also used as the subkey KDF.
	MAC = "HMAC-SHA256"
	// Construction is how Cipher and MAC are composed.
	Construction = "encrypt-then-MAC"
	// LabelEnc derives the AES key: ek = HMAC-SHA256(master, LabelEnc).
	LabelEnc = "burnbox/v1/enc"
	// LabelMAC derives the MAC key: mk = HMAC-SHA256(master, LabelMAC).
	LabelMAC = "burnbox/v1/mac"
)
