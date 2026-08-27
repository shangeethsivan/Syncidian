var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

// main.ts
var main_exports = {};
__export(main_exports, {
  default: () => SyncidianPlugin
});
module.exports = __toCommonJS(main_exports);
var import_obsidian2 = require("obsidian");

// codec.ts
var CHUNK = 8192;
function b64encode(bytes) {
  let bin = "";
  for (let i = 0; i < bytes.length; i += CHUNK) {
    const sub = bytes.subarray(i, i + CHUNK);
    let piece = "";
    for (let j = 0; j < sub.length; j++)
      piece += String.fromCharCode(sub[j]);
    bin += piece;
  }
  return btoa(bin);
}
function b64decode(s) {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++)
    out[i] = bin.charCodeAt(i);
  return out;
}
function toArrayBuffer(data) {
  return data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength);
}

// hash.ts
function hex(bytes) {
  let out = "";
  for (let i = 0; i < bytes.length; i++)
    out += bytes[i].toString(16).padStart(2, "0");
  return out;
}
async function sha256(data) {
  var _a;
  try {
    const subtle = (_a = globalThis.crypto) == null ? void 0 : _a.subtle;
    if (subtle) {
      const buf = await subtle.digest("SHA-256", data);
      return hex(new Uint8Array(buf));
    }
  } catch (e) {
  }
  return sha256Sync(new Uint8Array(data));
}
function sha256Sync(bytes) {
  const K = [
    1116352408,
    1899447441,
    3049323471,
    3921009573,
    961987163,
    1508970993,
    2453635748,
    2870763221,
    3624381080,
    310598401,
    607225278,
    1426881987,
    1925078388,
    2162078206,
    2614888103,
    3248222580,
    3835390401,
    4022224774,
    264347078,
    604807628,
    770255983,
    1249150122,
    1555081692,
    1996064986,
    2554220882,
    2821834349,
    2952996808,
    3210313671,
    3336571891,
    3584528711,
    113926993,
    338241895,
    666307205,
    773529912,
    1294757372,
    1396182291,
    1695183700,
    1986661051,
    2177026350,
    2456956037,
    2730485921,
    2820302411,
    3259730800,
    3345764771,
    3516065817,
    3600352804,
    4094571909,
    275423344,
    430227734,
    506948616,
    659060556,
    883997877,
    958139571,
    1322822218,
    1537002063,
    1747873779,
    1955562222,
    2024104815,
    2227730452,
    2361852424,
    2428436474,
    2756734187,
    3204031479,
    3329325298
  ];
  const rr = (x, n) => x >>> n | x << 32 - n;
  const len = bytes.length;
  const bitLenHi = Math.floor(len / 536870912);
  const bitLenLo = len << 3 >>> 0;
  const withPad = len + 1;
  const blockCount = withPad + 8 + 63 >> 6 << 4;
  const w = new Uint32Array(blockCount);
  for (let i = 0; i < len; i++)
    w[i >> 2] |= bytes[i] << 24 - i % 4 * 8;
  w[len >> 2] |= 128 << 24 - len % 4 * 8;
  w[blockCount - 2] = bitLenHi;
  w[blockCount - 1] = bitLenLo;
  let h0 = 1779033703, h1 = 3144134277, h2 = 1013904242, h3 = 2773480762, h4 = 1359893119, h5 = 2600822924, h6 = 528734635, h7 = 1541459225;
  const words = new Uint32Array(64);
  for (let i = 0; i < blockCount; i += 16) {
    for (let t = 0; t < 16; t++)
      words[t] = w[i + t];
    for (let t = 16; t < 64; t++) {
      const s0 = rr(words[t - 15], 7) ^ rr(words[t - 15], 18) ^ words[t - 15] >>> 3;
      const s1 = rr(words[t - 2], 17) ^ rr(words[t - 2], 19) ^ words[t - 2] >>> 10;
      words[t] = words[t - 16] + s0 + words[t - 7] + s1 >>> 0;
    }
    let a = h0, b = h1, c = h2, d = h3, e = h4, f = h5, g = h6, h = h7;
    for (let t = 0; t < 64; t++) {
      const S1 = rr(e, 6) ^ rr(e, 11) ^ rr(e, 25);
      const ch = e & f ^ ~e & g;
      const temp1 = h + S1 + ch + K[t] + words[t] >>> 0;
      const S0 = rr(a, 2) ^ rr(a, 13) ^ rr(a, 22);
      const maj = a & b ^ a & c ^ b & c;
      const temp2 = S0 + maj >>> 0;
      h = g;
      g = f;
      f = e;
      e = d + temp1 >>> 0;
      d = c;
      c = b;
      b = a;
      a = temp1 + temp2 >>> 0;
    }
    h0 = h0 + a >>> 0;
    h1 = h1 + b >>> 0;
    h2 = h2 + c >>> 0;
    h3 = h3 + d >>> 0;
    h4 = h4 + e >>> 0;
    h5 = h5 + f >>> 0;
    h6 = h6 + g >>> 0;
    h7 = h7 + h >>> 0;
  }
  const out = new Uint8Array(32);
  const hs = [h0, h1, h2, h3, h4, h5, h6, h7];
  for (let i = 0; i < 8; i++) {
    out[i * 4] = hs[i] >>> 24;
    out[i * 4 + 1] = hs[i] >>> 16 & 255;
    out[i * 4 + 2] = hs[i] >>> 8 & 255;
    out[i * 4 + 3] = hs[i] & 255;
  }
  return hex(out);
}

// mobile.ts
var import_obsidian = require("obsidian");
function isMobileApp() {
  return import_obsidian.Platform.isMobile || import_obsidian.Platform.isAndroidApp || import_obsidian.Platform.isIosApp;
}
function devicePlatform() {
  if (import_obsidian.Platform.isAndroidApp)
    return "Android";
  if (import_obsidian.Platform.isIosApp)
    return "iOS";
  if (import_obsidian.Platform.isMacOS)
    return "macOS";
  if (import_obsidian.Platform.isWin)
    return "Windows";
  if (import_obsidian.Platform.isLinux)
    return "Linux";
  return "unknown";
}
function guessDeviceName() {
  if (import_obsidian.Platform.isAndroidApp)
    return import_obsidian.Platform.isTablet ? "Android tablet" : "Android";
  if (import_obsidian.Platform.isIosApp)
    return import_obsidian.Platform.isTablet ? "iPad" : "iPhone";
  if (import_obsidian.Platform.isMacOS)
    return "macOS";
  if (import_obsidian.Platform.isWin)
    return "Windows";
  if (import_obsidian.Platform.isLinux)
    return "Linux";
  return "Obsidian";
}
function isLoopbackUrl(url) {
  try {
    const host = new URL(url).hostname.toLowerCase();
    return host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "[::1]" || host === "0.0.0.0";
  } catch (e) {
    return /localhost|127\.0\.0\.1/i.test(url);
  }
}
function isInsecureHttp(url) {
  try {
    return new URL(url).protocol === "http:";
  } catch (e) {
    return url.startsWith("http://");
  }
}
function pushBatchSize() {
  return isMobileApp() ? 6 : 40;
}

// main.ts
var DEFAULT_SETTINGS = {
  serverUrl: "http://localhost:8080",
  token: "",
  deviceName: "Obsidian",
  deviceId: "",
  hashes: {}
};
var IDLE_SYNC_MS = 3e3;
var IGNORE = [
  /^\.obsidian\/workspace(-mobile)?\.json$/,
  /^\.obsidian\/workspace-/,
  /^\.obsidian\/plugins\/syncidian\//,
  /^\.trash\//,
  /^\.git\//,
  /(^|\/)\.DS_Store$/
];
function ignored(path) {
  return IGNORE.some((re) => re.test(path));
}
var SyncidianPlugin = class extends import_obsidian2.Plugin {
  constructor() {
    super(...arguments);
    this.settings = DEFAULT_SETTINGS;
    this.statusEl = null;
    this.ribbonEl = null;
    this.connected = false;
    this.syncing = false;
    this.ws = null;
    this.wsConnecting = false;
    this.pending = /* @__PURE__ */ new Map();
    this.idleTimer = null;
    this.applyingRemote = 0;
  }
  async onload() {
    await this.loadSettings();
    if (!this.settings.deviceName || this.settings.deviceName === "Obsidian") {
      this.settings.deviceName = guessDeviceName();
    }
    this.statusEl = this.addStatusBarItem();
    this.statusEl.addClass("syncidian-status");
    this.ribbonEl = this.addRibbonIcon("sync", "Syncidian: sync now", () => {
      void this.fullSync();
    });
    this.setStatus("offline");
    this.addSettingTab(new SyncidianSettingTab(this.app, this));
    this.addCommand({
      id: "syncidian-sync-now",
      name: "Sync now",
      callback: () => void this.fullSync()
    });
    this.registerInterval(
      window.setInterval(() => {
        if (this.connected && !this.ws && !this.wsConnecting)
          void this.openSocket();
      }, 8e3)
    );
    this.registerInterval(
      window.setInterval(() => {
        if (this.connected && !this.ws && !this.syncing)
          void this.pollRemote();
      }, 15e3)
    );
    this.registerEvent(this.app.vault.on("create", (f) => this.queueChange(f, false)));
    this.registerEvent(this.app.vault.on("modify", (f) => this.queueChange(f, false)));
    this.registerEvent(this.app.vault.on("delete", (f) => this.queueChange(f, true)));
    this.registerEvent(
      this.app.vault.on("rename", (f, old) => {
        this.queueRename(old, f);
      })
    );
    this.app.workspace.onLayoutReady(() => {
      void this.startup();
    });
  }
  onunload() {
    var _a;
    this.clearIdleTimer();
    (_a = this.ws) == null ? void 0 : _a.close();
    this.ws = null;
    this.wsConnecting = false;
  }
  platform() {
    return devicePlatform();
  }
  setStatus(kind, extra = "") {
    var _a, _b, _c;
    const labels = {
      offline: "Syncidian \u2022 offline",
      connecting: "Syncidian \u2022 connecting",
      syncing: "Syncidian \u2022 syncing",
      pending: "Syncidian \u2022 pending",
      ok: "Syncidian \u2022 synced",
      conflict: "Syncidian \u2022 conflict",
      error: "Syncidian \u2022 error"
    };
    const text = extra ? `${labels[kind]} ${extra}` : labels[kind];
    (_a = this.statusEl) == null ? void 0 : _a.setText(text);
    (_b = this.ribbonEl) == null ? void 0 : _b.setAttribute("aria-label", text);
    (_c = this.ribbonEl) == null ? void 0 : _c.setAttribute("title", `${text} \u2014 tap to sync now`);
  }
  apiUrl(path) {
    return this.settings.serverUrl.replace(/\/$/, "") + path;
  }
  /** Normalize URL/token before every request (trim paste noise). */
  normalizeCredentials() {
    this.settings.serverUrl = (this.settings.serverUrl || "").trim().replace(/\/$/, "");
    this.settings.token = (this.settings.token || "").trim().replace(/\s+/g, "");
  }
  async api(path, opts = {}) {
    this.normalizeCredentials();
    const headers = {
      Authorization: `Bearer ${this.settings.token}`,
      "X-Syncidian-Client": `obsidian-plugin/${this.manifest.version}`
    };
    const body = typeof opts.body === "string" ? opts.body : void 0;
    if (body)
      headers["Content-Type"] = "application/json";
    const method = (opts.method || "GET").toString().toUpperCase();
    let status = 0;
    let text = "";
    try {
      const res = await (0, import_obsidian2.requestUrl)({
        url: this.apiUrl(path),
        method,
        headers,
        contentType: body ? "application/json" : void 0,
        body,
        throw: false
      });
      status = res.status;
      text = res.text;
    } catch (e) {
      throw new Error(
        `Cannot reach ${this.settings.serverUrl || "(no server URL)"}. On a phone, use a public HTTPS URL, not localhost. (${e.message})`
      );
    }
    let data = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch (e) {
      data = { error: text };
    }
    if (status < 200 || status >= 300) {
      if (status === 401) {
        throw new Error(
          (data == null ? void 0 : data.error) || "Invalid or revoked access token. In the dashboard, sign in as a vault user (not admin), open Tokens, create a new sk_sync_ token, and paste it here."
        );
      }
      if (status === 403) {
        throw new Error(
          (data == null ? void 0 : data.error) || "Forbidden. Admins cannot sync a vault \u2014 use a non-admin user token from the Tokens page."
        );
      }
      throw new Error((data == null ? void 0 : data.error) || `HTTP ${status}`);
    }
    return data;
  }
  mobileUrlError() {
    if (!isMobileApp())
      return null;
    if (isLoopbackUrl(this.settings.serverUrl)) {
      return "On Android and iOS, localhost is this device, not your computer. Set Server URL to your public HTTPS address (for example your Railway domain).";
    }
    return null;
  }
  async startup() {
    this.normalizeCredentials();
    if (!this.settings.token || !this.settings.serverUrl) {
      this.setStatus("offline");
      new import_obsidian2.Notice("Syncidian: set Server URL and access token first");
      return false;
    }
    if (!this.settings.token.startsWith("sk_sync_")) {
      this.setStatus("error");
      new import_obsidian2.Notice("Syncidian: access token must start with sk_sync_");
      return false;
    }
    const mobileErr = this.mobileUrlError();
    if (mobileErr) {
      this.setStatus("error");
      new import_obsidian2.Notice(`Syncidian: ${mobileErr}`);
      return false;
    }
    if (isMobileApp() && isInsecureHttp(this.settings.serverUrl) && import_obsidian2.Platform.isIosApp) {
      new import_obsidian2.Notice("Syncidian: iOS often blocks plain HTTP. Prefer an https:// server URL.");
    }
    try {
      await this.connect();
      const ok = await this.fullSync();
      if (!ok)
        return false;
      await this.openSocket();
      return true;
    } catch (e) {
      console.error(e);
      this.setStatus("error");
      new import_obsidian2.Notice(`Syncidian: ${e.message}`);
      return false;
    }
  }
  async connect() {
    this.setStatus("connecting");
    const body = {
      id: this.settings.deviceId || void 0,
      name: this.settings.deviceName,
      platform: this.platform(),
      plugin_version: this.manifest.version
    };
    const d = await this.api("/api/v1/devices/register", {
      method: "POST",
      body: JSON.stringify(body)
    });
    this.settings.deviceId = d.id;
    await this.saveSettings();
    this.connected = true;
  }
  async fullSync() {
    if (!this.settings.token)
      return false;
    if (this.syncing)
      return false;
    this.clearIdleTimer();
    this.pending.clear();
    this.syncing = true;
    this.setStatus("syncing");
    try {
      if (!this.connected)
        await this.connect();
      const local = await this.localManifest();
      const files = Object.keys(local).map(
        (path) => ({
          path,
          hash: local[path],
          base_hash: this.settings.hashes[path] || ""
        })
      );
      for (const path of Object.keys(this.settings.hashes)) {
        if (path in local || ignored(path))
          continue;
        files.push({
          path,
          hash: "",
          deleted: true,
          base_hash: this.settings.hashes[path] || ""
        });
      }
      const plan = await this.api("/api/v1/sync/plan", {
        method: "POST",
        body: JSON.stringify({ device_id: this.settings.deviceId, files })
      });
      for (const path of plan.Pull || []) {
        await this.pullFile(path);
      }
      if ((plan.Delete || []).length || (plan.Push || []).length) {
        await this.pushBatch(plan.Delete || [], plan.Push || []);
      }
      for (const path of plan.Conflicts || []) {
        await this.raiseConflict(path);
      }
      this.setStatus((plan.Conflicts || []).length ? "conflict" : "ok");
      return true;
    } catch (e) {
      this.setStatus("error");
      new import_obsidian2.Notice(`Syncidian sync failed: ${e.message}`);
      return false;
    } finally {
      this.syncing = false;
      if (this.pending.size)
        this.scheduleFlush();
    }
  }
  async localManifest() {
    const out = {};
    for (const file of this.app.vault.getFiles()) {
      if (ignored(file.path))
        continue;
      const data = await this.app.vault.readBinary(file);
      out[file.path] = await sha256(data);
    }
    return out;
  }
  async pullFile(path) {
    const remote = await this.api(`/api/v1/sync/file?path=${encodeURIComponent(path)}`);
    if (remote.deleted) {
      const existing = this.app.vault.getAbstractFileByPath(path);
      if (existing)
        await this.app.vault.delete(existing);
      delete this.settings.hashes[path];
      await this.saveSettings();
      return;
    }
    const bytes = toArrayBuffer(b64decode(remote.content || ""));
    await this.writeBinary(path, bytes);
    this.settings.hashes[path] = remote.hash;
    await this.saveSettings();
  }
  /** Write bytes, tolerating vault-index lag vs files already on disk. */
  async writeBinary(path, data) {
    path = (0, import_obsidian2.normalizePath)(path);
    const existing = this.app.vault.getAbstractFileByPath(path);
    if (existing instanceof import_obsidian2.TFile) {
      await this.app.vault.modifyBinary(existing, data);
      return;
    }
    if (existing) {
      throw new Error(`Cannot write file; path is a folder: ${path}`);
    }
    const dir = path.split("/").slice(0, -1).join("/");
    if (dir)
      await this.ensureFolder(dir);
    const again = this.app.vault.getAbstractFileByPath(path);
    if (again instanceof import_obsidian2.TFile) {
      await this.app.vault.modifyBinary(again, data);
      return;
    }
    try {
      await this.app.vault.createBinary(path, data);
    } catch (e) {
      const msg = e.message || String(e);
      if (!/already exists/i.test(msg))
        throw e;
      const raced = this.app.vault.getAbstractFileByPath(path);
      if (raced instanceof import_obsidian2.TFile) {
        await this.app.vault.modifyBinary(raced, data);
        return;
      }
      await this.app.vault.adapter.writeBinary(path, data);
    }
  }
  async ensureFolder(dir) {
    const parts = dir.split("/").filter(Boolean);
    let cur = "";
    for (const p of parts) {
      cur = cur ? `${cur}/${p}` : p;
      if (this.app.vault.getAbstractFileByPath(cur))
        continue;
      try {
        if (await this.app.vault.adapter.exists(cur))
          continue;
        await this.app.vault.createFolder(cur);
      } catch (e) {
        const msg = e.message || String(e);
        if (/already exists/i.test(msg))
          continue;
        throw e;
      }
    }
  }
  async pushPaths(paths, deleted) {
    if (deleted)
      await this.pushBatch(paths, []);
    else
      await this.pushBatch([], paths);
  }
  async pushBatch(deletes, upserts, renamedFrom = {}) {
    const files = [];
    const movedAway = new Set(Object.values(renamedFrom).filter(Boolean));
    for (const path of deletes) {
      if (movedAway.has(path))
        continue;
      files.push({
        path,
        hash: "",
        deleted: true,
        renamed_from: "",
        base_hash: this.settings.hashes[path] || "",
        mtime: Math.floor(Date.now() / 1e3),
        content: ""
      });
    }
    for (const path of upserts) {
      const af = this.app.vault.getAbstractFileByPath(path);
      if (!(af instanceof import_obsidian2.TFile))
        continue;
      const data = await this.app.vault.readBinary(af);
      const hash = await sha256(data);
      const from = renamedFrom[path] || "";
      files.push({
        path,
        hash,
        deleted: false,
        renamed_from: from,
        base_hash: this.settings.hashes[from] || this.settings.hashes[path] || "",
        mtime: Math.floor(af.stat.mtime / 1e3),
        content: b64encode(new Uint8Array(data))
      });
    }
    if (!files.length)
      return;
    const chunk = pushBatchSize();
    for (let i = 0; i < files.length; i += chunk) {
      const slice = files.slice(i, i + chunk);
      const last = i + chunk >= files.length;
      await this.sendPush(slice, last ? movedAway : /* @__PURE__ */ new Set());
    }
  }
  async sendPush(files, movedAway) {
    const res = await this.api("/api/v1/sync/push", {
      method: "POST",
      body: JSON.stringify({ device_id: this.settings.deviceId, files })
    });
    for (const p of res.accepted || []) {
      const f = files.find((x) => x.path === p);
      if (f == null ? void 0 : f.deleted)
        delete this.settings.hashes[p];
      else if (f)
        this.settings.hashes[p] = f.hash;
      else
        delete this.settings.hashes[p];
    }
    for (const old of movedAway)
      delete this.settings.hashes[old];
    await this.saveSettings();
    for (const c of res.conflicts || []) {
      new ConflictModal(this.app, this, c.id, c.path).open();
      this.setStatus("conflict");
    }
  }
  async raiseConflict(path) {
    new import_obsidian2.Notice(`Syncidian conflict: ${path}`);
    this.setStatus("conflict", path);
  }
  pathsUnder(prefix) {
    const folder = prefix.replace(/\/$/, "");
    const child = folder + "/";
    const out = [];
    for (const p of Object.keys(this.settings.hashes)) {
      if (p === folder || p.startsWith(child))
        out.push(p);
    }
    for (const p of this.pending.keys()) {
      if ((p === folder || p.startsWith(child)) && !out.includes(p))
        out.push(p);
    }
    return out;
  }
  queueChange(file, deleted) {
    if (!this.connected || this.syncing || this.applyingRemote)
      return;
    if (deleted) {
      const paths = file instanceof import_obsidian2.TFile ? [file.path] : this.pathsUnder(file.path);
      if (file instanceof import_obsidian2.TFile) {
        if (ignored(file.path))
          return;
      } else if (!paths.length && ignored(file.path)) {
        return;
      }
      for (const p of paths.length ? paths : [file.path]) {
        if (ignored(p))
          continue;
        this.pending.set(p, { deleted: true });
      }
    } else {
      if (!(file instanceof import_obsidian2.TFile) || ignored(file.path))
        return;
      const prev = this.pending.get(file.path);
      this.pending.set(file.path, { deleted: false, renamedFrom: prev == null ? void 0 : prev.renamedFrom });
    }
    this.scheduleFlush();
  }
  queueRename(oldPath, file) {
    if (!this.connected || this.syncing || this.applyingRemote)
      return;
    if (file instanceof import_obsidian2.TFile) {
      if (!ignored(oldPath))
        this.pending.set(oldPath, { deleted: true });
      if (!ignored(file.path)) {
        this.pending.set(file.path, { deleted: false, renamedFrom: oldPath });
      }
    } else {
      this.queueFolderRename(oldPath, file.path);
    }
    this.scheduleFlush();
  }
  queueFolderRename(oldPath, newPath) {
    const oldFolder = oldPath.replace(/\/$/, "");
    const newFolder = newPath.replace(/\/$/, "");
    const remap = (p) => {
      if (p === oldFolder)
        return newFolder;
      if (p.startsWith(oldFolder + "/"))
        return newFolder + p.slice(oldFolder.length);
      return null;
    };
    for (const [p, op] of [...this.pending.entries()]) {
      const np = remap(p);
      if (np == null)
        continue;
      this.pending.delete(p);
      if (!ignored(p))
        this.pending.set(p, { deleted: true });
      if (!ignored(np)) {
        this.pending.set(np, { deleted: op.deleted, renamedFrom: op.renamedFrom || p });
      }
    }
    for (const p of Object.keys(this.settings.hashes)) {
      const np = remap(p);
      if (np == null || ignored(np))
        continue;
      this.pending.set(p, { deleted: true });
      this.pending.set(np, { deleted: false, renamedFrom: p });
    }
    for (const f of this.app.vault.getFiles()) {
      if (ignored(f.path))
        continue;
      if (f.path !== newFolder && !f.path.startsWith(newFolder + "/"))
        continue;
      const rel = f.path === newFolder ? "" : f.path.slice(newFolder.length + 1);
      const oldP = rel ? `${oldFolder}/${rel}` : oldFolder;
      if (!ignored(oldP))
        this.pending.set(oldP, { deleted: true });
      this.pending.set(f.path, { deleted: false, renamedFrom: oldP });
    }
    if (!this.pathsUnder(oldFolder).length && !ignored(oldFolder)) {
      this.pending.set(oldFolder, { deleted: true });
    }
  }
  scheduleFlush() {
    this.clearIdleTimer();
    if (this.connected && !this.syncing)
      this.setStatus("pending");
    this.idleTimer = window.setTimeout(() => {
      this.idleTimer = null;
      void this.flushPending();
    }, IDLE_SYNC_MS);
  }
  clearIdleTimer() {
    if (this.idleTimer != null) {
      window.clearTimeout(this.idleTimer);
      this.idleTimer = null;
    }
  }
  async flushPending() {
    if (this.syncing) {
      this.scheduleFlush();
      return;
    }
    if (!this.pending.size)
      return;
    const snapshot = this.pending;
    this.pending = /* @__PURE__ */ new Map();
    const toDelete = [];
    const toPush = [];
    const renamedFrom = {};
    for (const [p, op] of snapshot) {
      if (op.deleted)
        toDelete.push(p);
      else {
        toPush.push(p);
        if (op.renamedFrom)
          renamedFrom[p] = op.renamedFrom;
      }
    }
    this.syncing = true;
    this.setStatus("syncing");
    try {
      await this.pushBatch(toDelete, toPush, renamedFrom);
      this.setStatus("ok");
    } catch (e) {
      this.setStatus("error");
      console.error(e);
      new import_obsidian2.Notice(`Syncidian sync failed: ${e.message}`);
    } finally {
      this.syncing = false;
      if (this.pending.size)
        this.scheduleFlush();
    }
  }
  async openSocket() {
    if (this.wsConnecting || this.ws)
      return;
    this.wsConnecting = true;
    this.normalizeCredentials();
    const base = this.settings.serverUrl.replace(/\/$/, "");
    let ticket = "";
    try {
      const t = await this.api("/api/v1/ws/ticket", { method: "POST", body: "{}" });
      ticket = (t == null ? void 0 : t.ticket) || "";
    } catch (e) {
      this.wsConnecting = false;
      return;
    }
    if (!ticket) {
      this.wsConnecting = false;
      return;
    }
    const wsUrl = base.replace(/^http/, "ws") + `/api/v1/ws?device_id=${encodeURIComponent(this.settings.deviceId)}&ticket=${encodeURIComponent(ticket)}`;
    try {
      this.ws = new WebSocket(wsUrl);
    } catch (e) {
      this.wsConnecting = false;
      return;
    }
    this.ws.onopen = () => {
      this.wsConnecting = false;
    };
    this.ws.onerror = () => {
      var _a;
      this.wsConnecting = false;
      try {
        (_a = this.ws) == null ? void 0 : _a.close();
      } catch (e) {
      }
    };
    this.ws.onmessage = (ev) => {
      void (async () => {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === "file_changed" && msg.path) {
            this.applyingRemote++;
            try {
              if (msg.deleted) {
                const f = this.app.vault.getAbstractFileByPath(msg.path);
                if (f)
                  await this.app.vault.delete(f);
                delete this.settings.hashes[msg.path];
                await this.saveSettings();
              } else if (msg.content) {
                const bytes = toArrayBuffer(b64decode(msg.content));
                await this.writeBinary(msg.path, bytes);
                if (msg.hash) this.settings.hashes[msg.path] = msg.hash;
                await this.saveSettings();
              } else {
                await this.pullFile(msg.path);
              }
            } finally {
              this.applyingRemote--;
            }
          }
          if (msg.type === "github_synced") {
            await this.fullSync();
          }
        } catch (e) {
          console.error(e);
        }
      })();
    };
    this.ws.onclose = () => {
      this.ws = null;
      this.wsConnecting = false;
    };
  }
  /** HTTP fallback when the mobile WebView blocks WebSockets (cleartext, ATS, captive Wi-Fi). */
  async pollRemote() {
    try {
      const man = await this.api("/api/v1/sync/manifest");
      const remote = /* @__PURE__ */ new Map();
      for (const f of man.files || [])
        remote.set(f.path, f.hash);
      let need = false;
      for (const [path, hash] of remote) {
        if (this.settings.hashes[path] !== hash) {
          need = true;
          break;
        }
      }
      if (!need) {
        for (const path of Object.keys(this.settings.hashes)) {
          if (!remote.has(path)) {
            need = true;
            break;
          }
        }
      }
      if (need)
        await this.fullSync();
    } catch (e) {
      console.error(e);
    }
  }
  async loadSettings() {
    this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());
    if (!this.settings.hashes)
      this.settings.hashes = {};
    this.normalizeCredentials();
    if (isMobileApp() && isLoopbackUrl(this.settings.serverUrl)) {
      this.settings.serverUrl = "";
    }
  }
  async saveSettings() {
    this.normalizeCredentials();
    await this.saveData(this.settings);
  }
};
var ConflictModal = class extends import_obsidian2.Modal {
  constructor(app, plugin, id, path) {
    super(app);
    this.plugin = plugin;
    this.id = id;
    this.path = path;
  }
  async onOpen() {
    const { contentEl } = this;
    contentEl.addClass("syncidian-modal");
    if (isMobileApp())
      contentEl.addClass("syncidian-modal-mobile");
    contentEl.createEl("h2", { text: "Sync conflict" });
    contentEl.createEl("p", { text: `${this.path} was modified on more than one device.` });
    let data = {};
    try {
      data = await this.plugin.api(`/api/v1/conflicts/${this.id}`);
    } catch (e) {
      contentEl.createEl("p", { text: e.message });
      return;
    }
    const cols = contentEl.createDiv({ cls: "cols" });
    const left = cols.createDiv();
    left.createEl("h3", { text: "This device" });
    const localTa = left.createEl("textarea");
    localTa.value = data.local_content || "";
    const right = cols.createDiv();
    right.createEl("h3", { text: "Server / other device" });
    const remoteTa = right.createEl("textarea");
    remoteTa.value = data.remote_content || "";
    contentEl.createEl("h3", { text: "Merged result" });
    const mergeTa = contentEl.createEl("textarea");
    mergeTa.value = localTa.value;
    const row = contentEl.createDiv({ cls: "setting-item-control" });
    row.createEl("button", { text: "Keep local" }).onclick = () => void this.resolve("local");
    row.createEl("button", { text: "Keep remote" }).onclick = () => void this.resolve("remote");
    row.createEl("button", { text: "Save merge", cls: "mod-cta" }).onclick = () => void this.resolve("merged", mergeTa.value);
  }
  async resolve(resolution, content) {
    await this.plugin.api(`/api/v1/conflicts/${this.id}/resolve`, {
      method: "POST",
      body: JSON.stringify({
        resolution,
        content: content || "",
        device_id: this.plugin.settings.deviceId
      })
    });
    await this.plugin.pullFile(this.path);
    new import_obsidian2.Notice(`Resolved ${this.path}`);
    this.close();
    this.plugin.setStatus("ok");
  }
  onClose() {
    this.contentEl.empty();
  }
};
var SyncidianSettingTab = class extends import_obsidian2.PluginSettingTab {
  constructor(app, plugin) {
    super(app, plugin);
    this.plugin = plugin;
  }
  display() {
    const { containerEl } = this;
    containerEl.empty();
    containerEl.createEl("p", {
      text: "Install from Community plugins (or BRAT). Point this vault at your Syncidian server. GitHub is configured per user in the dashboard, not here. Unlike Git plugins, this one uses only the Obsidian API and works on Android and iOS."
    });
    if (isMobileApp()) {
      new import_obsidian2.Setting(containerEl).setName("Mobile").setDesc(
        "Use a public HTTPS server URL. localhost is this phone or tablet, not your computer. iOS often blocks plain http://."
      );
    }
    let urlInput = null;
    let tokenInput = null;
    let nameInput = null;
    new import_obsidian2.Setting(containerEl).setName("Server URL").setDesc(
      isMobileApp() ? "HTTPS address of your Syncidian server, for example https://sync.example.com" : "Example: http://localhost:8080 or https://sync.example.com"
    ).addText((t) => {
      urlInput = t.inputEl;
      t.setPlaceholder(isMobileApp() ? "https://sync.example.com" : "http://localhost:8080").setValue(this.plugin.settings.serverUrl).onChange(async (v) => {
        this.plugin.settings.serverUrl = v.trim();
        await this.plugin.saveSettings();
      });
    });
    new import_obsidian2.Setting(containerEl).setName("Access token").setDesc("Created in the Syncidian dashboard Tokens page (vault user, not admin). Starts with sk_sync_").addText((t) => {
      tokenInput = t.inputEl;
      t.inputEl.type = "password";
      t.inputEl.spellcheck = false;
      t.inputEl.autocomplete = "off";
      t.setPlaceholder("sk_sync_\u2026").setValue(this.plugin.settings.token).onChange(async (v) => {
        this.plugin.settings.token = v.trim().replace(/\s+/g, "");
        await this.plugin.saveSettings();
      });
    });
    new import_obsidian2.Setting(containerEl).setName("Device name").addText((t) => {
      nameInput = t.inputEl;
      t.setValue(this.plugin.settings.deviceName).onChange(async (v) => {
        this.plugin.settings.deviceName = v.trim() || "Obsidian";
        await this.plugin.saveSettings();
      });
    });
    new import_obsidian2.Setting(containerEl).setName("Connect").setDesc("Register this device and run an initial sync").addButton(
      (b) => b.setButtonText("Connect").setCta().onClick(async () => {
        if (urlInput)
          this.plugin.settings.serverUrl = urlInput.value.trim();
        if (tokenInput)
          this.plugin.settings.token = tokenInput.value.trim().replace(/\s+/g, "");
        if (nameInput)
          this.plugin.settings.deviceName = nameInput.value.trim() || "Obsidian";
        await this.plugin.saveSettings();
        const mobileErr = this.plugin.mobileUrlError();
        if (mobileErr) {
          new import_obsidian2.Notice(`Syncidian: ${mobileErr}`);
          return;
        }
        const ok = await this.plugin.startup();
        if (ok)
          new import_obsidian2.Notice("Syncidian connected");
      })
    );
  }
};
