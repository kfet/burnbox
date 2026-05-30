# burnbox

Server-blind, one-time secret sharing. Paste a secret, get a single-use
link, share it. The recipient opens it **once** and the ciphertext is
destroyed. The server never sees the plaintext or the key.

It is the only such service that satisfies three constraints at once:

1. **Zero install on the recipient.** Decrypt with only what ships on a
   bare macOS / Linux box — `curl`, `python3` (stdlib), `openssl`. No
   `gpg`, no `node`, no `age`, no `pip install`.
2. **True end-to-end.** The server stores opaque ciphertext only. The
   key lives in the URL fragment (`#…`) and is never transmitted.
3. **Web-hosted.** A single static page does the in-browser crypto; a
   tiny stdlib Go server stores and burns blobs.

The trick that makes (1) and (2) coexist is the cipher choice:
**AES-256-CTR + HMAC-SHA256 (Encrypt-then-MAC)** instead of AES-GCM. EtM
is an authenticated-encryption construction of equivalent strength, but
unlike GCM both primitives are exposed by the `openssl` CLI and Python's
`hmac`/`hashlib` stdlib — so the recipient needs no crypto library.
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

## Decrypting in a terminal (zero install)

Every link has a companion recipe page at `/r/<id>#<key>` that prints a
ready-to-paste command. It looks like this:

```sh
KEY='<key>' curl -s https://host/s/<id> | python3 -c '
import sys,os,base64,hmac,hashlib,subprocess
def u(s): return base64.urlsafe_b64decode(s+"="*(-len(s)%4))
b=u(sys.stdin.read().strip()); iv,ct,tag=b[:16],b[16:-32],b[-32:]
m=u(os.environ["KEY"])
ek=hmac.new(m,b"burnbox/v1/enc",hashlib.sha256).digest()
mk=hmac.new(m,b"burnbox/v1/mac",hashlib.sha256).digest()
assert hmac.compare_digest(tag,hmac.new(mk,iv+ct,hashlib.sha256).digest()),"bad MAC"
sys.stdout.buffer.write(subprocess.run(
    ["openssl","enc","-aes-256-ctr","-d","-K",ek.hex(),"-iv",iv.hex()],
    input=ct,capture_output=True,check=True).stdout)'
```

The fragment (`<key>`) never reaches the server — browsers don't send
fragments, and the command takes it from `KEY=` locally.

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
make e2e     # just the end-to-end (real curl|python3|openssl round-trip)
```

Stdlib-only server, zero third-party Go dependencies. Go 1.22+.

## License

MIT © Kalin Fetvadjiev
