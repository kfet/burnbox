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
    "import sys,base64,hmac,hashlib,subprocess\n" +
    "def u(s): return base64.urlsafe_b64decode(s+\"=\"*(-len(s)%4))\n" +
    "b=u(sys.stdin.read().strip()); iv,ct,tag=b[:16],b[16:-32],b[-32:]\n" +
    "m=u(sys.argv[1])\n" +
    "ek=hmac.new(m,b\"burnbox/v1/enc\",hashlib.sha256).digest()\n" +
    "mk=hmac.new(m,b\"burnbox/v1/mac\",hashlib.sha256).digest()\n" +
    "assert hmac.compare_digest(tag,hmac.new(mk,iv+ct,hashlib.sha256).digest()),\"bad MAC\"\n" +
    "sys.stdout.buffer.write(subprocess.run([\"openssl\",\"enc\",\"-aes-256-ctr\",\"-d\",\"-K\",ek.hex(),\"-iv\",iv.hex()],input=ct,capture_output=True,check=True).stdout)";
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
