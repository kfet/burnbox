// recipe page client: builds the bare-OS decryption command for a
// one-time secret. The decryption key comes from this page's URL
// fragment and is never sent to the server.
"use strict";
(function () {
  var parts = location.pathname.split("/");
  var id = parts[parts.length - 1];
  var key = location.hash.replace(/^#/, "");
  var cmd = document.getElementById("cmd");
  var err = document.getElementById("err");
  if (!key) {
    err.textContent = "No key in the URL fragment — this recipe link is incomplete.";
    return;
  }
  var py =
    "import sys,base64,hmac,hashlib\n" +
    "def u(s): return base64.urlsafe_b64decode(s+\"=\"*(-len(s)%4))\n" +
    "b=u(sys.stdin.read().strip()); iv,ct,tag=b[:16],b[16:-32],b[-32:]\n" +
    "m=u(sys.argv[1])\n" +
    "ek=hmac.new(m,b\"burnbox/v1/enc\",hashlib.sha256).digest()\n" +
    "mk=hmac.new(m,b\"burnbox/v1/mac\",hashlib.sha256).digest()\n" +
    "assert hmac.compare_digest(tag,hmac.new(mk,iv+ct,hashlib.sha256).digest()),\"bad MAC\"\n" +
    "X=lambda a:(a<<1^(27 if a&128 else 0))&255\n" +
    "S=[99]*256;p=q=1\n" +
    "while 1:\n" +
    " p=p^X(p);q^=q<<1;q^=q<<2;q^=q<<4;q&=255;q^=9 if q&128 else 0\n" +
    " S[p]=99^q^(q<<1|q>>7)&255^(q<<2|q>>6)&255^(q<<3|q>>5)&255^(q<<4|q>>4)&255\n" +
    " if p==1: break\n" +
    "w=[list(ek[i:i+4]) for i in range(0,32,4)];r=1\n" +
    "for i in range(8,60):\n" +
    " t=w[-1]\n" +
    " if i%8==0: t=[S[x] for x in t[1:]+t[:1]];t[0]^=r;r=X(r)\n" +
    " elif i%8==4: t=[S[x] for x in t]\n" +
    " w.append([x^y for x,y in zip(w[i-8],t)])\n" +
    "K=[sum(w[j:j+4],[]) for j in range(0,60,4)]\n" +
    "def M(a):\n" +
    " t=a[0]^a[1]^a[2]^a[3];return [a[i]^t^X(a[i]^a[(i+1)%4]) for i in range(4)]\n" +
    "def E(s):\n" +
    " s=[x^y for x,y in zip(s,K[0])]\n" +
    " for r in range(1,15):\n" +
    "  s=[S[s[(i+4*i)%16]] for i in range(16)]\n" +
    "  if r<14: s=M(s[0:4])+M(s[4:8])+M(s[8:12])+M(s[12:16])\n" +
    "  s=[x^y for x,y in zip(s,K[r])]\n" +
    " return s\n" +
    "n=int.from_bytes(iv,\"big\");o=bytearray()\n" +
    "for i in range(0,len(ct),16): o+=bytes(x^y for x,y in zip(ct[i:i+16],E(((n+i//16)%2**128).to_bytes(16,\"big\"))))\n" +
    "sys.stdout.buffer.write(bytes(o))";
  var full = "curl -s " + new URL("../s/" + id, location.href).href +
    " | python3 -c '" + py + "' '" + key + "'";

  // AI-agent variant: same decrypt, but the secret is written to a 0600
  // file (never the console) and the command is prefixed with prompt
  // comments telling an AI how to handle a burn-after-read secret safely.
  // Selected with ?ai on the recipe URL so it can be shared as a link.
  var aiMode = new URLSearchParams(location.search).has("ai");
  var aiCmd =
    "# burnbox one-time secret — instructions for the AI agent running this:\n" +
    "#  - The output is a SECRET. Do NOT print, echo, or log it, and do NOT\n" +
    "#    inspect it with cat or any file-reading tool — anything on the\n" +
    "#    console or in your transcript may be captured in logs.\n" +
    "#  - Do NOT open this URL with a web-fetch/browser tool and do NOT curl\n" +
    "#    it first to 'preview' it: the secret BURNS on the first read, so you\n" +
    "#    get exactly ONE fetch.\n" +
    "#  - Pipe it to a TEMPORARY file — delete it the moment you no\n" +
    "#    longer need it\n" +
    "( umask 077; " + full + " )";

  var shown = aiMode ? aiCmd : full;
  cmd.textContent = shown;

  // Cross-link the two variants, preserving the key in the fragment.
  var ailink = document.getElementById("ailink");
  if (aiMode) {
    document.getElementById("title").innerHTML =
      '<span class="b">burn</span>box — AI agent recipe';
    document.getElementById("intro").innerHTML =
      "Annotated recipe for an AI agent. The comments tell it to keep the " +
      "secret out of logs, not to preview the URL (it burns on first read), " +
      "and to pipe the output to a temporary file it deletes when done.";
    ailink.innerHTML = '<a href="' + location.pathname + location.hash +
      '">← human terminal version</a>';
  } else {
    ailink.innerHTML = 'Handing this to an AI agent? Use the ' +
      '<a href="' + location.pathname + "?ai" + location.hash +
      '">AI-annotated version</a> — it tells the agent to keep the secret out ' +
      'of logs and not to burn it by previewing the link.';
  }

  var copyBtn = document.getElementById("copy");
  copyBtn.onclick = function () {
    var flash = function (msg, ok) {
      copyBtn.textContent = msg;
      if (ok) copyBtn.className = "ok";
      clearTimeout(copyBtn._t);
      copyBtn._t = setTimeout(function () {
        copyBtn.textContent = "Copy command";
        copyBtn.className = "";
      }, 1600);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(shown)
        .then(function () { flash("Copied \u2713", true); })
        .catch(function () { flash("Press \u2318/Ctrl-C to copy", false); });
    } else {
      flash("Press \u2318/Ctrl-C to copy", false);
    }
  };
})();
