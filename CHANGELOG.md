# Changelog

All notable changes to burnbox are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and the project aims to
follow semantic versioning once it reaches 1.0.

## [Unreleased]

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

