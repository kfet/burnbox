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
    "import sys,base64\n" +
    "from hmac import*\n" +
    "u=lambda s:base64.b64decode(s+\"==\",\"-_\")\n" +
    "b=u(input());m=u(sys.argv[1]);H=lambda k,x:digest(k,x,\"sha256\")\n" +
    "if not compare_digest(b[-32:],H(H(m,b\"burnbox/v1/mac\"),b[:-32])):sys.exit(\"bad MAC\")\n" +
    "O=int.__xor__;X=lambda a:a*2^(a>>7)*283\n" +
    "L=[p:=1]+[p:=p^X(p)for _ in\" \"*255];S={0:99}\n" +
    "for p,q in zip(L,L[::-1]):x=q^q*2^q*4^q*8^q*16;S[p]=x%256^x>>8^99\n" +
    "w=list(H(m,b\"burnbox/v1/enc\"))\n" +
    "for j in range(8,60):\n" +
    " t=w[-4:]\n" +
    " if j%4<1:f=j%8<1;t=[S[x]for x in t[f:]+t[:f]];t[0]^=f<<j//8>>1\n" +
    " w+=map(O,w[-32:],t)\n" +
    "def E(s):\n" +
    " for r in range(15):\n" +
    "  if r:s=[S[s[i*5%16]]for i in range(16)]\n" +
    "  if r%14:s=[X(x^s[i&12|i+1&3])^s[i^1]^s[i^2]^s[i^3]for i,x in enumerate(s)]\n" +
    "  s=[*map(O,s,w[16*r:])]\n" +
    " return s\n" +
    "open(1,\"wb\").write(bytes(map(O,b[16:-32],(y for k in range(len(b))for y in E((int(b[:16].hex(),16)+k).to_bytes(17,\"big\")[1:])))))";
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
