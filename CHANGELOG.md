# Changelog

All notable changes to burnbox are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and the project aims to
follow semantic versioning once it reaches 1.0.

## [Unreleased]

### Changed
- **Recipient recipe is ~30% smaller** (Python 1340 → 948 bytes) and still
  readable: S-box computed from powers of 3 (GF(2^8) inverse + affine),
  flat key schedule, `hmac.digest`, `input()`. Same wire contract;
  explicit constant-time `sys.exit("bad MAC")` kept; max-size secret
  decrypts in ~1.5 s on macOS python3.9.

### Changed
- `skills/deploy` now documents the real production target (`rn`, systemd
  user unit, Caddy at burnbox.kfet.io) and ships `live-verify.sh` for the
  post-deploy round-trip + burn check.

## [0.1.10] - 2026-10-19

### Fixed
- **Recipe MAC check survives `python3 -O`.** The HMAC verification was an
  `assert`, which `-O` strips — a tampered blob would then decrypt to
  garbage. It is now an explicit constant-time check that `sys.exit("bad MAC")`s
  non-zero before any output. New test covers tampered ciphertext under `-O`.

## [0.1.9] - 2026-10-19

### Changed
- **Recipient recipe no longer needs `openssl`.** The terminal decrypt is
  now `curl` + `python3` stdlib only: the `openssl enc -aes-256-ctr`
  subprocess is replaced by a compact pure-Python AES-256 + CTR loop
  (full 128-bit big-endian counter, identical output). Runs on stock
  macOS `/usr/bin/python3` (3.9) and Python ≥ 3.8. Wire format and server
  are unchanged; old links keep working.
- Tests now extract the recipe Python from `recipe.js` and verify it
  against Go-encrypted payloads (empty, non-block-aligned, >256-block
  counter carry, 2^128 wrap) and the FIPS-197 AES-256 known-answer vector.

## [0.1.8] - 2026-07-02

### Added
- Terminal recipe page now links to an **AI-agent variant** at
  `/r/{id}?ai#key`. It renders the same `curl | python3 | openssl`
  decrypt (wrapped in `( umask 077; … )`) prefixed with `#` prompt
  comments instructing the agent: do not print/echo/`cat` the secret (it
  can land in logs/transcripts), do not web-fetch or "preview" the URL
  (it burns on the first read), and pipe the output to a temporary file
  it deletes when done. Purely client-side (the key stays in the URL
  fragment); the server stays blind.

## [0.1.7] - 2026-06-18

### Fixed
- `GET /s/{id}` now responds with `Content-Type: text/plain; charset=utf-8`
  instead of `application/octet-stream`. The stored blob is base64url ASCII
  text, so `text/plain` is the accurate type; link-aware surfaces (chat
  apps, previews) can now classify it correctly instead of treating it as
  an unknown binary. Burn-after-read semantics are unchanged.

## [0.1.6] - 2026-06-07

### Added
- iOS home-screen icon: a 180×180 `apple-touch-icon.png` (full-bleed
  square — iOS rounds corners itself) embedded via `embed.FS` and served
  at `/apple-touch-icon.png`, linked from the index and recipe pages
  along with an `apple-mobile-web-app-title` of "burnbox". "Add to Home
  Screen" on iOS now picks up the flame automatically. The PNG is
  regenerated from the committed `apple-touch-icon.src.svg` via
  `rsvg-convert` (see `internal/ui/ui.go`).

## [0.1.5] - 2026-06-06

### Added
- Built-in SVG favicon (a flame in the brand accent) embedded via
  `embed.FS` and served at `/favicon.svg`; linked from the index and
  recipe pages with prefix-safe relative hrefs. Permitted by the
  existing `img-src 'self'` CSP — no policy change needed.

## [0.1.4] - 2026-06-06

### Fixed
- Terminal recipe command was broken: it used the `KEY='…' curl … | python3`
  env-prefix form, where the assignment scopes only to `curl` and never
  reaches the piped `python3`, so decryption failed with `bad MAC`. The
  recipe now passes the key as a python `argv` (`… | python3 -c '…' '<key>'`),
  which is robust regardless of shell and drops the `os` import.
- The e2e test masked this: it injected `KEY` into the whole shell
  environment rather than running the real generated form. It now executes
  the exact pipeline with the key as argv and `KEY` deliberately absent from
  the environment, so this class of regression fails the test.

## [0.1.3] - 2026-06-02

### Added
- Copy-command button on the terminal recipe page now confirms success: it
  flashes "Copied ✓" (green) or a hint on failure.
- CI workflows updated to run on Node 24 actions (`checkout@v6`,
  `setup-go@v6`), clearing the Node 20 deprecation warnings.

## [0.1.2] - 2026-06-02

### Added
- Copy-link button now confirms the result: it shows "Copied ✓" (green)
  on success, or a "Press ⌘/Ctrl-C to copy" hint if the clipboard API is
  unavailable, instead of silently doing nothing.
- Automated release pipeline (`.github/workflows/release.yml`): on a
  `v*` tag, build per-arch binary tarballs, publish them as release
  assets, and push a rendered Homebrew formula to
  `github.com/kfet/homebrew-tap`. Install with
  `brew install kfet/tap/burnbox`.
- `make release-local` target, `packaging/homebrew/burnbox.rb.tmpl`, and
  `scripts/render-formula.sh` supporting the pipeline and local release
  verification.

## [0.1.1] - 2026-06-02

### Added
- `GET /version` endpoint reporting the running build version as JSON,
  useful for deploy/health checks.
- Frontend footer now shows the running version and links to the project
  source (the former "about" link pointed at the page itself and did
  nothing).

## [0.1.0] - 2026-06-01

### Added
- Initial implementation of burnbox: a server-blind, one-time secret
  sharing service.
- Frozen **v1 crypto contract**: AES-256-CTR + HMAC-SHA256
  (Encrypt-then-MAC), HMAC-derived subkeys, key carried only in the URL
  fragment. Reproducible byte-for-byte across browser WebCrypto, the Go
  test harness, and the bare-OS `curl|python3|openssl` recipient path.
- `internal/store`: in-memory TTL blob store with atomic burn-on-read
  (`GetDel`) and a background janitor.
- `internal/server`: stdlib HTTP surface — `POST /s`, `GET /s/{id}`
  (burn), `GET /` (SPA), `GET /r/{id}` (terminal recipe page),
  `GET /burnbox.js`, `GET /recipe.js`, `GET /healthz`. No cryptography on
  the server side by design.
- Defence-in-depth response headers on all served pages/scripts: a
  strict `Content-Security-Policy` (`script-src 'self'`, no inline JS),
  `Referrer-Policy: no-referrer`, and `X-Content-Type-Options: nosniff`,
  to harden the browser-delivered crypto against fragment exfiltration.
- `internal/ui`: embedded WebCrypto single-page app and recipe page.
- `cmd/burnbox`: `serve` and `version` subcommands.
- Path-relative frontend URLs so burnbox can be reverse-proxied under an
  arbitrary path prefix (e.g. Tailscale `serve --set-path=/secret`, which
  strips the prefix). Assets, fetches, and generated share/recipe links
  all resolve against the document base instead of the host root.
- Deployment artefacts: `packaging/launchd/` macOS LaunchAgent template
  and `packaging/systemd/burnbox.service` user unit, plus a `deploy`
  skill documenting the test-then-prod flow behind Tailscale Funnel.
  burnbox is stateless, so deploy is just ship-binary + supervise +
  funnel — no data dir or credentials.
- `make all` pipeline: build + cross-compile matrix + gofmt + vet +
  frontend lint + race/shuffle tests with a 100% coverage gate + an e2e
  smoke that decrypts via the real bare-OS recipient pipeline.

### Fixed
- Trailing-slash robustness when mounted under a stripped path prefix:
  opening the app without the trailing slash (e.g. `/secret`) made the
  relative assets resolve one level too high (404, blank page). index.html
  now carries a tiny inline bootstrap that redirects to the slash form;
  it is allowed under the strict CSP via a pinned sha256 hash, which a
  unit test keeps in sync with the script.

