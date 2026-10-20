# burnbox

Server-blind, one-time secret sharing. Paste a secret, get a single-use
link, share it. The recipient opens it **once** and the ciphertext is
destroyed. The server never sees the plaintext or the key.

It is the only such service that satisfies three constraints at once:

1. **Zero install on the recipient.** Decrypt with only what ships on a
   bare macOS / Linux box — `curl` and `python3` (stdlib only). No `openssl`, no
   `gpg`, no `node`, no `age`, no `pip install`.
2. **True end-to-end.** The server stores opaque ciphertext only. The
   key lives in the URL fragment (`#…`) and is never transmitted.
3. **Web-hosted.** A single static page does the in-browser crypto; a
   tiny stdlib Go server stores and burns blobs.

The trick that makes (1) and (2) coexist is the cipher choice:
**AES-256-CTR + HMAC-SHA256 (Encrypt-then-MAC)** instead of AES-GCM. EtM
is an authenticated-encryption construction of equivalent strength, but
unlike GCM it is trivial to do with Python's stdlib alone: `hmac`/`hashlib`
for the MAC and a ~25-line pure-Python AES-256 block function driving the
CTR loop — so the recipient needs no crypto library and no `openssl`.
Yopass, PrivateBin, and friends all picked GCM and thereby locked
terminal users out.

## Quick start

```sh
go build -o burnbox ./cmd/burnbox
./burnbox serve            # listens on :8080
open http://localhost:8080
```

Type a secret → get a link like `http://host/#<id>.<key>`. Open it once;
it's gone.

## CLI

```
burnbox serve [flags]    # run the server
burnbox version          # print version, commit, build date
```

`serve` flags:

| Flag         | Default   | Meaning                                  |
|--------------|-----------|------------------------------------------|
| `-listen`    | `:8080`   | address to listen on                     |
| `-max-size`  | `262144`  | maximum ciphertext blob size, in bytes (256 KiB) |
| `-max-ttl`   | `168h`    | ceiling for requested TTLs (7 days)      |
| `-min-ttl`   | `1h`      | floor / default TTL                      |

A blob's lifetime is set per-secret via `POST /s?ttl=<seconds>`, clamped
into `[min-ttl, max-ttl]`; an absent or invalid `ttl` defaults to
`min-ttl`. The server is stateless and holds everything in memory — a
restart drops any un-read secrets (acceptable by design).

### HTTP API

| Method & path  | Behaviour                                                        |
|----------------|------------------------------------------------------------------|
| `POST /s`      | store an opaque ciphertext blob (body, ≤ `max-size`); returns `{"id":"…"}`. Optional `?ttl=<seconds>`. |
| `GET /s/{id}`  | atomically return the blob **and burn it**; `404` JSON if absent/expired/already viewed. `Content-Type: text/plain; charset=utf-8`. |
| `GET /`        | the static single-page app (encrypt + in-browser decrypt).       |
| `GET /r/{id}`  | human page rendering the copy-paste terminal recipe for that id. |
| `GET /healthz` | `200 ok`.                                                        |


## Decrypting in a terminal (zero install)

Every link has a companion recipe page at `/r/<id>#<key>` that prints a
ready-to-paste command. It looks like this:

```sh
curl -s https://host/s/<id> | python3 -c '
import sys,base64
from hmac import*
u=lambda s:base64.urlsafe_b64decode(s+"==")
b=u(input());m=u(sys.argv[1]);H=lambda k,x:digest(k,x,"sha256")
if not compare_digest(b[-32:],H(H(m,b"burnbox/v1/mac"),b[:-32])):sys.exit("bad MAC")
X=lambda a:a*2^(a>>7)*283
L=[1];exec("L+=[L[-1]^X(L[-1])];"*254);S={0:99}
for k in range(255):q=L[-k];x=q^q*2^q*4^q*8^q*16;S[L[k]]=(x^x>>8)&255^99
r=1;w=list(H(m,b"burnbox/v1/enc"))
for j in range(32,240,4):
 t=w[-4:]
 if j%16<1:t=[S[x]for x in t]
 if j%32<1:t=t[1:]+t[:1];t[0]^=r;r=X(r)
 w+=[x^y for x,y in zip(w[j-32:],t)]
M=lambda a:[x^a[0]^a[1]^a[2]^a[3]^X(x^a[i-3])for i,x in enumerate(a)]
def E(s):
 for r in range(15):
  if r:s=[S[s[i*5%16]]for i in range(16)]
  if r%14:s=sum(map(M,zip(*[iter(s)]*4)),[])
  s=[x^y for x,y in zip(s,w[16*r:])]
 return s
c=b[16:-32];n=int.from_bytes(b[:16],"big")
open(1,"wb").write(bytes(x^y for x,y in zip(c,(y for k in range(len(c))for y in E((n+k).to_bytes(17,"big")[1:])))))' '<key>'
```

The fragment (`<key>`) never reaches the server — browsers don't send
fragments; it is passed to Python as a local argument. The AES is pure
Python (FIPS-197, verified against a NIST known-answer vector in the test
suite) — fast enough for burnbox's 256 KiB cap.

## Crypto contract (v1)

Three independent implementations agree byte-for-byte (browser WebCrypto,
the Go test harness, the terminal recipe):

```
master = 32 random bytes  →  URL-safe base64 (no pad), in the fragment
ek     = HMAC-SHA256(master, "burnbox/v1/enc")     # 32-byte AES key
mk     = HMAC-SHA256(master, "burnbox/v1/mac")     # 32-byte MAC key
iv     = 16 random bytes
ct     = AES-256-CTR(ek, counter=iv, plaintext)    # WebCrypto length:128
tag    = HMAC-SHA256(mk, iv || ct)                 # Encrypt-then-MAC
blob   = base64url_nopad(iv || ct || tag)          # what the server stores
```

See [AGENTS.md](AGENTS.md) for the full design and rationale.

## Security model

- **Server compromise reveals no secrets**: it holds only ciphertext +
  MAC; the key is never present.
- **Browser-delivered E2E caveat**: a malicious server could serve
  tampered JS that exfiltrates the fragment — unsolvable for any
  in-browser crypto app. The **terminal recipe is the mitigation**: it
  can be audited once and run without loading any server JavaScript.
- **Burn is atomic**: the first reader of `/s/<id>` gets the blob and it
  is deleted under lock; everyone else gets a 404.

## Development

```sh
make all     # build + cross-compile + fmt + vet + frontend lint +
             # race tests w/ 100% coverage gate + e2e smoke
make e2e     # just the end-to-end (real curl|python3 round-trip)
```

Stdlib-only server, zero third-party Go dependencies. Go 1.24+.

## License

MIT © Kalin Fetvadjiev
