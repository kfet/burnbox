#!/bin/bash
# live round-trip + burn: live.sh https://host
set -eu; H=$1; D=$(dirname "$0")
PY=$(curl -fsS "$H/recipe.js" | python3 -c '
import sys,re,json;s=sys.stdin.read();s=s[s.index("var py =")+8:];s=re.split(r"\"\s*;",s)[0]+"\""
print("".join(json.loads(m) for m in re.findall(r"\"(?:[^\"\\]|\\.)*\"",s)),end="")')
MSG="live-check $(date +%s) $RANDOM"
OUT=$(printf %s "$MSG" | go run "$D/enc.go"); BLOB=$(sed -n 1p <<<"$OUT"); KEY=$(sed -n 2p <<<"$OUT")
ID=$(curl -fsS -X POST --data-binary "$BLOB" "$H/s" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
GOT=$(curl -s "$H/s/$ID" | python3 -c "$PY" "$KEY")
[ "$GOT" = "$MSG" ] && echo "round-trip OK ($ID)" || { echo "MISMATCH: $GOT"; exit 1; }
C=$(curl -s -o /dev/null -w %{http_code} "$H/s/$ID"); [ "$C" = 404 ] && echo "burn OK (2nd read 404)" || { echo "burn FAIL $C"; exit 1; }
