// burnbox single-page client. Implements the frozen v1 crypto contract
// (see AGENTS.md) entirely in the browser via WebCrypto. The key is a
// random 32-byte master, carried only in the URL fragment.
//
// Contract:
//   ek  = HMAC-SHA256(master, "burnbox/v1/enc")
//   mk  = HMAC-SHA256(master, "burnbox/v1/mac")
//   iv  = 16 random bytes
//   ct  = AES-256-CTR(ek, counter=iv, length=128, plaintext)
//   tag = HMAC-SHA256(mk, iv || ct)        (Encrypt-then-MAC)
//   blob = base64url_nopad(iv || ct || tag)
"use strict";

const enc = new TextEncoder();
const dec = new TextDecoder();

function b64uEncode(bytes) {
  const u8 = new Uint8Array(bytes);
  // Build the binary string in chunks: spreading a 256 KiB array into
  // String.fromCharCode(...) arguments overflows the call stack.
  let s = "";
  const chunk = 0x8000;
  for (let i = 0; i < u8.length; i += chunk) {
    s += String.fromCharCode.apply(null, u8.subarray(i, i + chunk));
  }
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
function b64uDecode(str) {
  str = str.replace(/-/g, "+").replace(/_/g, "/");
  while (str.length % 4) str += "=";
  const bin = atob(str);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

async function hmac(keyBytes, msgBytes) {
  const k = await crypto.subtle.importKey(
    "raw", keyBytes, { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  return new Uint8Array(await crypto.subtle.sign("HMAC", k, msgBytes));
}

async function subkeys(master) {
  const ek = await hmac(master, enc.encode("burnbox/v1/enc"));
  const mk = await hmac(master, enc.encode("burnbox/v1/mac"));
  return { ek, mk };
}

function concat(...arrs) {
  let n = 0;
  for (const a of arrs) n += a.length;
  const out = new Uint8Array(n);
  let o = 0;
  for (const a of arrs) { out.set(a, o); o += a.length; }
  return out;
}

async function encryptSecret(plaintext) {
  const master = crypto.getRandomValues(new Uint8Array(32));
  const iv = crypto.getRandomValues(new Uint8Array(16));
  const { ek, mk } = await subkeys(master);
  const aesKey = await crypto.subtle.importKey(
    "raw", ek, { name: "AES-CTR" }, false, ["encrypt"]);
  const ct = new Uint8Array(await crypto.subtle.encrypt(
    { name: "AES-CTR", counter: iv, length: 128 }, aesKey, enc.encode(plaintext)));
  const tag = await hmac(mk, concat(iv, ct));
  const blob = b64uEncode(concat(iv, ct, tag));
  return { blob, key: b64uEncode(master) };
}

async function decryptBlob(blobStr, keyStr) {
  const raw = b64uDecode(blobStr);
  if (raw.length < 16 + 32) throw new Error("blob too short");
  const iv = raw.slice(0, 16);
  const ct = raw.slice(16, raw.length - 32);
  const tag = raw.slice(raw.length - 32);
  const master = b64uDecode(keyStr);
  const { ek, mk } = await subkeys(master);
  const expect = await hmac(mk, concat(iv, ct));
  if (!timingSafeEqual(tag, expect)) throw new Error("bad MAC — wrong key or corrupted");
  const aesKey = await crypto.subtle.importKey(
    "raw", ek, { name: "AES-CTR" }, false, ["decrypt"]);
  const pt = await crypto.subtle.decrypt(
    { name: "AES-CTR", counter: iv, length: 128 }, aesKey, ct);
  return dec.decode(pt);
}

function timingSafeEqual(a, b) {
  if (a.length !== b.length) return false;
  let v = 0;
  for (let i = 0; i < a.length; i++) v |= a[i] ^ b[i];
  return v === 0;
}

// ---- UI wiring ----

const $ = (id) => document.getElementById(id);

// baseURL returns the directory URL of the current page (with trailing
// slash, hash/query stripped). Used to build absolute share/recipe links
// that respect whatever path prefix burnbox is mounted under (e.g. a
// Tailscale `--set-path=/secret` mount). All same-origin fetches use
// plain relative paths so they resolve against this base automatically.
function baseURL() {
  return new URL(".", location.href).href;
}

async function doCreate() {
  $("cerr").textContent = "";
  const secret = $("secret").value;
  if (!secret) { $("cerr").textContent = "Nothing to encrypt."; return; }
  const hours = Math.max(1, parseInt($("ttl").value, 10) || 24);
  $("enc").disabled = true;
  try {
    const { blob, key } = await encryptSecret(secret);
    const res = await fetch("s?ttl=" + (hours * 3600), {
      method: "POST",
      headers: { "Content-Type": "application/octet-stream" },
      body: blob,
    });
    if (!res.ok) throw new Error("server error " + res.status);
    const { id } = await res.json();
    const base = baseURL();
    const url = base + "#" + id + "." + key;
    const recipe = base + "r/" + id + "#" + key;
    $("link").textContent = url;
    $("rlink").innerHTML = '<a href="' + recipe + '">terminal recipe</a>';
    $("copy").onclick = () => navigator.clipboard.writeText(url);
    $("create").classList.add("hide");
    $("result").classList.remove("hide");
    $("secret").value = "";
  } catch (e) {
    $("cerr").textContent = String(e.message || e);
  } finally {
    $("enc").disabled = false;
  }
}

async function doView(id, key) {
  $("create").classList.add("hide");
  $("view").classList.remove("hide");
  try {
    const res = await fetch("s/" + id);
    if (res.status === 404) throw new Error("This secret has already been viewed or has expired.");
    if (!res.ok) throw new Error("server error " + res.status);
    const blobStr = await res.text();
    $("plain").textContent = await decryptBlob(blobStr, key);
  } catch (e) {
    $("plain").textContent = "";
    $("verr").textContent = String(e.message || e);
  }
}

function init() {
  const h = location.hash.replace(/^#/, "");
  const dot = h.indexOf(".");
  if (dot > 0) {
    doView(h.slice(0, dot), h.slice(dot + 1));
    return;
  }
  $("enc").onclick = doCreate;
  $("again").onclick = () => location.assign(baseURL());
}

document.addEventListener("DOMContentLoaded", init);
