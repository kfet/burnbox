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
  cmd.textContent = full;

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
      navigator.clipboard.writeText(full)
        .then(function () { flash("Copied \u2713", true); })
        .catch(function () { flash("Press \u2318/Ctrl-C to copy", false); });
    } else {
      flash("Press \u2318/Ctrl-C to copy", false);
    }
  };
})();
