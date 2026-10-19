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
   on a bare macOS / Linux box: `curl` and `python3` (stdlib only,
   3.8+ — stock macOS `/usr/bin/python3` is 3.9). No `openssl`, no
   `gpg`, no `node`, no `age`, no `pip install`.
2. **True end-to-end.** The server stores opaque ciphertext only. The
   key lives in the URL fragment (`#...`) and is never transmitted.
3. **Web-hosted.** A single static page does the in-browser crypto; a
   tiny stdlib Go server stores and burns blobs.

The unlock that makes (1) and (2) coexist is the cipher choice:
**AES-256-CTR + HMAC-SHA256 (Encrypt-then-MAC)** instead of AES-GCM.
EtM is an authenticated-encryption construction of equivalent strength,
but unlike GCM it is easy to implement with Python's stdlib alone:
`hmac`/`hashlib` for the MAC plus a compact pure-Python AES-256 block
function driving a CTR loop — so the recipient one-liner needs no
third-party crypto library and no `openssl`. Every "fancy" competitor
picked GCM and thereby locked terminal users out.

## NON-NEGOTIABLE product invariants (do not propose weakening these)

These are the *reason burnbox exists*. Any change, refactor, or
"improvement" that erodes one of them is wrong by definition — push
back on the request, do not implement it.

1. **Burn-after-reading is THE product.** `GET /s/{id}` MUST atomically
   return the blob **and destroy it** on the *first* successful read
   (`store.GetDel`). The secret is gone forever after one fetch. Do NOT:
   - make reads non-destructive,
   - gate the burn behind an opt-in (header / method / query param /
     confirmation page) so "passive" fetches don't burn,
   - add a "preview" / "peek" path that returns the blob without burning,
   - defer or delay the burn.
   If a link unfurler / chat preview / scanner fetches the URL, it
   **burns the secret — that is correct and intended behaviour**, not a
   bug. The mitigation is operational (don't paste raw `/s/` links into
   surfaces that auto-fetch; share the `/r/{id}#key` recipe or the copied
   command instead), never a code change that softens the burn.

2. **Server stays blind.** No encryption/decryption code in the Go
   server, ever. It stores and returns opaque ciphertext only. The key
   lives in the URL fragment and is never transmitted.

3. **Zero-install recipient.** Decrypt with only `curl`, stdlib
   stdlib `python3`. No new client-side dependency may sneak in.

Rationale for #1, recorded so it is never re-litigated: a one-time
secret that can be read twice is not a one-time secret. The entire value
over a pastebin is that exposure is bounded to exactly one read. "Make it
survive being previewed/unfurled" defeats the whole purpose.

## Scope (what's in v0.1)

- **Single binary**, subcommands: `serve`, `version`.
- **HTTP API**:
  - `POST /s` — body is an opaque ciphertext blob (≤ max size). Returns
    JSON `{"id": "..."}`. Optional `?ttl=` (seconds, clamped).
  - `GET /s/{id}` — atomically returns the blob **and deletes it**
    (burn). 404 (JSON `{"error":"not found or already viewed"}`) if
    absent/expired/already burned. `Content-Type: text/plain; charset=utf-8`.
  - `GET /` — the static single-page app (encrypt + in-browser decrypt).
  - `GET /r/{id}` — a human page that prints the copy-paste terminal
    recipient recipe (curl|python3) for that id, with the
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
> what the recipe's pure-Python CTR loop does (full-block big-endian
> increment mod 2^128, same as `openssl enc -aes-256-ctr`). Any smaller
> `length` makes the browser and the shell path diverge silently. Versioning is carried by the KDF labels
> (`burnbox/v1/...`), so no version byte is stored on the wire; a future
> `v2` simply changes the labels and old keys can't validate new blobs.
> The server returns the raw blob as `text/plain; charset=utf-8` (the
> blob is base64url ASCII, never JSON-wrapped) so the recipient pipe stays
> a clean `curl | python3`.

### Reference recipient one-liner (must keep working)

```bash
curl -s https://HOST/s/<id> | python3 -c '
import sys,base64,hmac,hashlib
def u(s): return base64.urlsafe_b64decode(s+"="*(-len(s)%4))
b=u(sys.stdin.read().strip()); iv,ct,tag=b[:16],b[16:-32],b[-32:]
m=u(sys.argv[1])
ek=hmac.new(m,b"burnbox/v1/enc",hashlib.sha256).digest()
mk=hmac.new(m,b"burnbox/v1/mac",hashlib.sha256).digest()
if not hmac.compare_digest(tag,hmac.new(mk,iv+ct,hashlib.sha256).digest()): sys.exit("bad MAC")
X=lambda a:(a<<1^(27 if a&128 else 0))&255
S=[99]*256;p=q=1
while 1:
 p=p^X(p);q^=q<<1;q^=q<<2;q^=q<<4;q&=255;q^=9 if q&128 else 0
 S[p]=99^q^(q<<1|q>>7)&255^(q<<2|q>>6)&255^(q<<3|q>>5)&255^(q<<4|q>>4)&255
 if p==1: break
w=[list(ek[i:i+4]) for i in range(0,32,4)];r=1
for i in range(8,60):
 t=w[-1]
 if i%8==0: t=[S[x] for x in t[1:]+t[:1]];t[0]^=r;r=X(r)
 elif i%8==4: t=[S[x] for x in t]
 w.append([x^y for x,y in zip(w[i-8],t)])
K=[sum(w[j:j+4],[]) for j in range(0,60,4)]
def M(a):
 t=a[0]^a[1]^a[2]^a[3];return [a[i]^t^X(a[i]^a[(i+1)%4]) for i in range(4)]
def E(s):
 s=[x^y for x,y in zip(s,K[0])]
 for r in range(1,15):
  s=[S[s[(i+4*i)%16]] for i in range(16)]
  if r<14: s=M(s[0:4])+M(s[4:8])+M(s[8:12])+M(s[12:16])
  s=[x^y for x,y in zip(s,K[r])]
 return s
n=int.from_bytes(iv,"big");o=bytearray()
for i in range(0,len(ct),16): o+=bytes(x^y for x,y in zip(ct[i:i+16],E(((n+i//16)%2**128).to_bytes(16,"big"))))
sys.stdout.buffer.write(bytes(o))' '<fragment>'
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
- **Go 1.24+.**
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
e2e/                        # make e2e: full round-trip incl. pure-python recipient
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

## E2E harness (continuous verification)

`make e2e` must, in one process:

1. Start `burnbox serve` on a random port (or use `httptest`).
2. In Go, perform the **client** crypto for a known plaintext using the
   frozen contract, `POST /s` the blob, get an id.
3. **Shell out to the real recipient one-liner** (`curl` + `python3`)
   against `GET /s/{id}` and assert it prints the original plaintext.
   This proves the bare-OS path actually works — the core promise.
4. Assert the second `GET /s/{id}` is a 404 (burned).
5. Assert `GET /` and `GET /r/{id}` serve HTML and contain the recipe.

Fast (sub-30s). Skips the shell step with a clear log line if `python3`
is unavailable, but CI has it. The recipe Python is extracted from
   `recipe.js` itself and also checked against Go-encrypted payloads
   (odd sizes, counter carry/wrap) and a FIPS-197 AES-256 vector.
