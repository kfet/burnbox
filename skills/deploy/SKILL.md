---
name: deploy
description: Deploy burnbox to a host behind Tailscale Funnel, supervised by launchd (macOS) or systemd (Linux). Verify on a throwaway test unit on an alt port, then enable the prod unit.
---

# Deploy Skill

Deploy `burnbox` to a host fronted by `tailscale funnel`. The server
listens on loopback; funnel terminates TLS and forwards.

burnbox is **stateless** — secrets live in memory and are burned on
read. There is no data dir, no credentials, no init step, and nothing
to back up. A restart simply drops any un-read secrets, which is the
correct fail-safe behaviour. This makes deploy far simpler than a
stateful service: ship the binary, supervise it, funnel the port.

The flow is **test-then-prod**: start a *test* unit on an alternate
port, smoke-test it (incl. the bare-OS recipient path) through funnel,
then enable the prod unit on the canonical port.

## Confirm with the user before acting

1. **Host** — ssh target (`user@host`), or `localhost` for this Mac.
   Linux (Pi) or macOS.
2. **Funnel layout**:
   - **(a) Dedicated host** — funnel `127.0.0.1:<port>` on `/`. Public
     URL `https://<host>.<tailnet>.ts.net/`.
   - **(b) Prefix** — funnel on `/<prefix>` (e.g. `/secret`). The SPA
     uses `location.origin` + relative paths and reads the key from the
     URL fragment, so a path prefix is transparent **as long as funnel
     does not strip it** (burnbox has no prefix-rewrite support; prefer
     a dedicated host or a non-stripping mapping).
3. **Port** — prod `127.0.0.1:8087`, test `127.0.0.1:8089` by
   convention.

## Steps

### 1. Ship the binary

**macOS (this Mac or a remote Mac):**

```bash
# from the burnbox tree:
make build
cp burnbox ~/.local/bin/burnbox        # local
# or, remote:
make build-darwin-arm64 V=1            # then scp the artefact
scp burnbox <host>:~/.local/bin/burnbox
ssh <host> '~/.local/bin/burnbox version'
```

**Linux (Pi):**

```bash
# cross-build from the dev tree and copy over:
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
  -ldflags='-s -w -X github.com/kfet/burnbox.Version='"$(cat VERSION)" \
  -o /tmp/burnbox-linux-arm64 ./cmd/burnbox
scp /tmp/burnbox-linux-arm64 <host>:~/.local/bin/burnbox
ssh <host> 'chmod +x ~/.local/bin/burnbox && ~/.local/bin/burnbox version'
```

(armv6/armv7 Pis: `GOARCH=arm GOARM=6/7`.)

### 2. Supervise

#### macOS: launchd user agent

Render `packaging/launchd/dev.user.burnbox.plist.tmpl`, substituting
`__USER__` and `__PORT__`, to
`~/Library/LaunchAgents/dev.<user>.burnbox.plist`:

```bash
sed -e "s/__USER__/$USER/g" -e "s/__PORT__/8089/g" \
  packaging/launchd/dev.user.burnbox.plist.tmpl \
  > ~/Library/LaunchAgents/dev.$USER.burnbox-test.plist
# fix the Label to match the filename's -test suffix before loading:
/usr/libexec/PlistBuddy -c "Set :Label dev.$USER.burnbox-test" \
  ~/Library/LaunchAgents/dev.$USER.burnbox-test.plist

plutil -lint ~/Library/LaunchAgents/dev.$USER.burnbox-test.plist
launchctl bootout   gui/$UID/dev.$USER.burnbox-test 2>/dev/null
launchctl bootstrap gui/$UID ~/Library/LaunchAgents/dev.$USER.burnbox-test.plist
launchctl print     gui/$UID/dev.$USER.burnbox-test | grep -E 'state =|pid ='
```

#### Linux: systemd user unit

```bash
mkdir -p ~/.config/systemd/user
sed 's/8087/8089/' packaging/systemd/burnbox.service \
  > ~/.config/systemd/user/burnbox-test.service
systemctl --user daemon-reload
systemctl --user enable --now burnbox-test
systemctl --user status burnbox-test --no-pager
```

### 3. Smoke-test the test unit (loopback first)

```bash
curl -s http://127.0.0.1:8089/healthz        # -> ok
# headers: CSP + no-referrer present
curl -sI http://127.0.0.1:8089/ | grep -i 'content-security\|referrer'
```

Full crypto round-trip + burn + auto-restart — see the verification
script pattern in the repo's localhost test (encrypt with the v1
contract, `POST /s`, decrypt via `curl | python3 | openssl`, assert the
second read is 404, `kill -9` the pid and confirm the supervisor
respawns a healthy process).

### 4. Funnel and verify end-to-end

```bash
ssh <host> 'sudo tailscale funnel --bg --set-path=/secret-test 127.0.0.1:8089'
ssh <host> 'tailscale serve status'
```

From your workstation:

```bash
curl -sL --max-time 20 -o /dev/null \
  -w 'final=%{url_effective} code=%{http_code}\n' \
  https://<host>.<tailnet>.ts.net/secret-test/
```

Connection refused → the unit didn't bind to 127.0.0.1; check logs
(`tail -f ~/Library/Logs/burnbox.err.log` /
`journalctl --user -u burnbox-test -f`).

### 5. Promote to prod, retire test

Repeat step 2 with no `-test` suffix, port `8087`. On Linux also run
`loginctl enable-linger "$USER"` so the unit survives logout/reboot.

```bash
# switch funnel from test to prod
ssh <host> 'sudo tailscale funnel --https=443 --set-path=/secret-test off'
ssh <host> 'sudo tailscale funnel --bg --set-path=/secret 127.0.0.1:8087'

# tear down the test unit
launchctl bootout gui/$UID/dev.$USER.burnbox-test        # macOS
systemctl --user disable --now burnbox-test              # Linux
```

## Teardown / update

- **Update**: `make build` → copy binary → `launchctl kickstart -k
  gui/$UID/dev.<user>.burnbox` (macOS) / `systemctl --user restart
  burnbox` (Linux). KeepAlive/Restart brings it back on the new binary.
- **Remove**: `launchctl bootout gui/$UID/dev.<user>.burnbox` and delete
  the plist; or `systemctl --user disable --now burnbox`.
