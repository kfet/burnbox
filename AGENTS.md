# AGENTS.md

Project brief for AI agents working on `burnbox`.

## What this is

`burnbox` is a self-hosted, **server-blind, one-time secret** service.
You paste a secret in your browser, it is encrypted client-side, and
you get a single-use URL. The recipient opens it once and the
ciphertext is destroyed (burn-after-reading). The server **never sees
the plaintext or the key**.

It is a Yopass / PrivateBin / OneTimeSecret in spirit, but built to
satisfy three constraints *simultaneously* — which no existing service
does:

1. **Zero install on the recipient.** Decrypt with only tools that ship
   on a bare macOS / Linux box: `curl`, `python3` (stdlib), `openssl`.
   No `gpg`, no `node`, no `age`, no `pip install`.
2. **True end-to-end.** The server stores opaque ciphertext only. The
   key lives in the URL fragment (`#...`) and is never transmitted.
3. **Web-hosted.** A single static page does the in-browser crypto; a
   tiny stdlib Go server stores and burns blobs.

The unlock that makes (1) and (2) coexist is the cipher choice:
**AES-256-CTR + HMAC-SHA256 (Encrypt-then-MAC)** instead of AES-GCM.
EtM is an authenticated-encryption construction of equivalent strength,
but unlike GCM both primitives are exposed by the `openssl` CLI and
Python's `hmac`/`hashlib` stdlib — so the recipient one-liner needs no
third-party crypto library. Every "fancy" competitor picked GCM and
thereby locked terminal users out.

## Scope (what's in v0.1)

- **Single binary**, subcommands: `serve`, `version`.
- **HTTP API**:
  - `POST /s` — body is an opaque ciphertext blob (≤ max size). Returns
    JSON `{"id": "..."}`. Optional `?ttl=` (seconds, clamped).
  - `GET /s/{id}` — atomically returns the blob **and deletes it**
    (burn). 404 (JSON `{"error":"not found or already viewed"}`) if
    absent/expired/already burned. `Content-Type: application/octet-stream`.
  - `GET /` — the static single-page app (encrypt + in-browser decrypt).
  - `GET /r/{id}` — a human page that prints the copy-paste terminal
    recipient recipe (curl|python3|openssl) for that id, with the
    fragment filled in client-side from `location.hash`.
  - `GET /healthz` — `200 ok`.
- **Storage**: in-memory map with per-entry TTL + a janitor goroutine.
  No DB, no disk persistence in v0.1 (blobs are ephemeral by nature —
  a restart losing un-read secrets is acceptable and arguably correct).
- **Crypto is entirely client-side.** The Go server contains **no
  encryption code** and MUST NOT — that is the whole point. It is a
  blind blob store. Keep it that way.

## Out of scope (v0.1)

- Disk-backed / clustered storage. Single process, in-memory. (A
  `Store` interface keeps the door open for v0.2 Redis/bbolt.)
- Passphrase second-factor. The fragment key is the only secret in
  v0.1. (Design note below for how it slots in later.)
- Accounts, quotas, abuse mitigation beyond a body-size cap and a
  per-IP rate limit. PoW / captcha is a later conversation.
- File uploads. v0.1 is text secrets; the blob cap is small (256 KiB).

## Crypto contract (frozen — both ends must agree byte-for-byte)

This is a wire contract shared by three independent implementations
(browser WebCrypto, the Go test harness, the recipient one-liner).
Changing it is a breaking change and a version bump of the `v1` tag.

- **Master key**: 32 random bytes. Transported as URL-safe base64
  **without padding** in the URL fragment. Never sent to the server.
- **Subkey derivation** (HMAC, so it is trivially reproducible in shell
  *and* WebCrypto):
  - `ek = HMAC-SHA256(key=master, msg="burnbox/v1/enc")`  → 32-byte AES key
  - `mk = HMAC-SHA256(key=master, msg="burnbox/v1/mac")`  → 32-byte MAC key
- **IV**: 16 random bytes (AES-CTR counter block).
- **Encrypt**: `ct = AES-256-CTR(ek, iv, plaintext)`.
- **Authenticate (Encrypt-then-MAC)**: `tag = HMAC-SHA256(mk, iv || ct)`.
  Verify with a constant-time compare *before* decrypting on read.
- **Blob (what the server stores)**: `base64url_nopad(iv || ct || tag)`.
  - Layout: first 16 bytes IV, last 32 bytes tag, middle is ciphertext.
  - The server treats this as an opaque string; only the client parses it.

> **Pinned interop detail (do not change):** WebCrypto `AES-CTR` MUST use
> `length: 128` so the *entire* 128-bit block is the counter — this is
> what `openssl enc -aes-256-ctr -iv <16B>` does (full-block big-endian
> increment). Any smaller `length` makes the browser and the shell path
> diverge silently. Versioning is carried by the KDF labels
> (`burnbox/v1/...`), so no version byte is stored on the wire; a future
> `v2` simply changes the labels and old keys can't validate new blobs.
> The server returns the raw blob as `application/octet-stream` (never
> JSON-wrapped) so the recipient pipe stays a clean `curl | python3`.

### Reference recipient one-liner (must keep working)

```bash
KEY='<fragment>' curl -s https://HOST/s/<id> | python3 -c '
import sys,os,base64,hmac,hashlib,subprocess
def b64u(s): return base64.urlsafe_b64decode(s + "="*(-len(s)%4))
blob = b64u(sys.stdin.read().strip())
iv, ct, tag = blob[:16], blob[16:-32], blob[-32:]
mk = b64u(os.environ["KEY"])
ek  = hmac.new(mk, b"burnbox/v1/enc", hashlib.sha256).digest()
mac = hmac.new(mk, b"burnbox/v1/mac", hashlib.sha256).digest()
if not hmac.compare_digest(tag, hmac.new(mac, iv+ct, hashlib.sha256).digest()):
    sys.exit("bad MAC — wrong key or corrupted")
sys.stdout.buffer.write(subprocess.run(
    ["openssl","enc","-aes-256-ctr","-d","-K",ek.hex(),"-iv",iv.hex()],
    input=ct, capture_output=True, check=True).stdout)'
```

The `/r/{id}` page renders exactly this, with `HOST` and `<id>` baked in
and `KEY` pulled from the fragment by a 3-line script so the user copies
a ready-to-run command. The fragment never reaches the server because
browsers do not send it.

## Threat model (state honestly in README)

- **Server compromise does not reveal secrets**: it holds only
  ciphertext + the MAC; the key is never present. ✓
- **The classic browser-E2E caveat applies**: a malicious/compromised
  server could serve tampered JS that exfiltrates the fragment. This is
  unsolvable for *browser-delivered* crypto. The terminal recipe is the
  mitigation — it can be pinned/audited once and run without loading any
  server JS. Document this prominently; it is a feature, not an excuse.
- **Burn is best-effort against races**: the read path deletes under a
  lock and returns the blob only to the first caller; concurrent reads
  get exactly one winner (see store contract).

## Constraints

- **Stdlib-only on the server.** Zero third-party Go dependencies. No
  `go.sum`. **Any proposed dependency requires an aside-advisor
  escalation first** and must clear a very high bar.
- **No crypto in the Go server.** If you find yourself importing
  `crypto/aes` in `internal/server` or `internal/store`, stop — you
  have broken the design. (Crypto *is* allowed in `e2e/` and tests, to
  verify the contract.)
- **Go 1.22+.**
- **No global state.** Constructor returns `*Server`; handlers are
  methods on it. No `init()` registries.
- **Tests hit real HTTP** via `httptest.NewServer`. No mocking the
  transport.

## Repo layout

```
cmd/burnbox/main.go         # entry point: serve, version (covignored — wiring)
doc.go / doc_test.go        # package doc + Version/Commit/BuildDate vars
internal/store/             # in-memory TTL blob store, atomic burn
internal/server/            # HTTP handlers (blind blob store + page serving)
internal/ui/                # //go:embed static SPA + recipe template
e2e/                        # make e2e: full round-trip incl. openssl recipient
VERSION CHANGELOG.md README.md Makefile .covignore .github/workflows/
```

## Workflow

- `make all` = build + cross-compile matrix + gofmt + vet + race/shuffle
  tests with a **100% coverage gate** (via `covgate`, excluding
  `.covignore`) + the e2e smoke. Must pass before any commit.
  **Do not weaken the gate** — add a justified `.covignore` line or
  write the test.
- Every user-visible change gets a `## [Unreleased]` CHANGELOG entry.
- Keep README + this file in sync when the crypto contract or layout
  changes.

## Advisor cadence (mandatory)

Use `aside` with `escalate=true` (aside-advisor skill) at minimum:

1. **Before freezing the crypto contract** — confirm EtM construction,
   subkey derivation, IV handling, and that WebCrypto + openssl + python
   can all reproduce it. (This is the self-handoff review.)
2. **Before the store concurrency model** — confirm the atomic-burn
   semantics under concurrent reads.
3. **Before declaring v0.1 done** — sanity-check the whole shape and the
   threat-model writeup.

Plus the standard triggers: stuck, changing approach, about to call it
done.

## E2E harness (continuous verification)

`make e2e` must, in one process:

1. Start `burnbox serve` on a random port (or use `httptest`).
2. In Go, perform the **client** crypto for a known plaintext using the
   frozen contract, `POST /s` the blob, get an id.
3. **Shell out to the real recipient one-liner** (`python3` + `openssl`)
   against `GET /s/{id}` and assert it prints the original plaintext.
   This proves the bare-OS path actually works — the core promise.
4. Assert the second `GET /s/{id}` is a 404 (burned).
5. Assert `GET /` and `GET /r/{id}` serve HTML and contain the recipe.

Fast (sub-30s). Skips the shell step with a clear log line if `python3`
or `openssl` is unavailable, but CI has both.

## Reference repos

For repo conventions (Makefile, CI, CHANGELOG, doc.go, release flow):

- `github.com/kfet/harborrs` — closest scaffolding (this brief mirrors it)
- `github.com/kfet/pinexec` — minimal stdlib-only build/test setup
- `github.com/kfet/covgate` — drives the 100% coverage gate

Clone into `/tmp` for reference; do not vendor.
