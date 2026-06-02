// Package burnbox is the root package for the burnbox server — a
// server-blind, one-time secret sharing service.
//
// The server is a blind blob store: it holds opaque ciphertext and
// nothing else. All encryption happens client-side (browser WebCrypto
// or a bare-OS curl|python3|openssl one-liner); the key travels only in
// the URL fragment and never reaches the server. See AGENTS.md for the
// full design brief and the frozen crypto contract.
//
// Subpackages:
//
//   - internal/store  — in-memory TTL blob store with atomic burn
//   - internal/server — HTTP handlers (blind store + page serving)
//   - internal/ui     — embedded single-page app and recipient recipe
package burnbox

// Version is the current build version, sourced from the VERSION file at
// release time. Kept as a var (not const) so release builds can override
// it via -ldflags -X github.com/kfet/burnbox.Version=...
var (
	Version   = "0.1.1"
	Commit    = "unknown" // git short SHA, set via -ldflags at release time
	BuildDate = "unknown" // ISO-8601 UTC, set via -ldflags at release time
)
