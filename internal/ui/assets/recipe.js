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
    "import sys,os,base64,hmac,hashlib,subprocess\n" +
    "def u(s): return base64.urlsafe_b64decode(s+\"=\"*(-len(s)%4))\n" +
    "b=u(sys.stdin.read().strip()); iv,ct,tag=b[:16],b[16:-32],b[-32:]\n" +
    "m=u(os.environ[\"KEY\"])\n" +
    "ek=hmac.new(m,b\"burnbox/v1/enc\",hashlib.sha256).digest()\n" +
    "mk=hmac.new(m,b\"burnbox/v1/mac\",hashlib.sha256).digest()\n" +
    "assert hmac.compare_digest(tag,hmac.new(mk,iv+ct,hashlib.sha256).digest()),\"bad MAC\"\n" +
    "sys.stdout.buffer.write(subprocess.run([\"openssl\",\"enc\",\"-aes-256-ctr\",\"-d\",\"-K\",ek.hex(),\"-iv\",iv.hex()],input=ct,capture_output=True,check=True).stdout)";
  var full = "KEY='" + key + "' curl -s " + new URL("../s/" + id, location.href).href +
    " | python3 -c '" + py + "'";
  cmd.textContent = full;
  document.getElementById("copy").onclick = function () { navigator.clipboard.writeText(full); };
})();
