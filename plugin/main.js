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
var import_obsidian = require("obsidian");
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
async function sha256(data) {
  const buf = await crypto.subtle.digest("SHA-256", data);
  return Array.from(new Uint8Array(buf)).map((b) => b.toString(16).padStart(2, "0")).join("");
}
function b64encode(bytes) {
  let bin = "";
  bytes.forEach((b) => bin += String.fromCharCode(b));
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
var SyncidianPlugin = class extends import_obsidian.Plugin {
  constructor() {
    super(...arguments);
    this.settings = DEFAULT_SETTINGS;
    this.connected = false;
    this.syncing = false;
    this.ws = null;
    this.pending = /* @__PURE__ */ new Map();
    this.idleTimer = null;
    this.applyingRemote = 0;
  }
  async onload() {
    await this.loadSettings();
    if (!this.settings.deviceName || this.settings.deviceName === "Obsidian") {
      this.settings.deviceName = this.guessDeviceName();
    }
    this.statusEl = this.addStatusBarItem();
    this.setStatus("offline");
    this.addSettingTab(new SyncidianSettingTab(this.app, this));
    this.addRibbonIcon("sync", "Syncidian: sync now", () => {
      void this.fullSync();
    });
    this.addCommand({
      id: "syncidian-sync-now",
      name: "Sync now",
      callback: () => void this.fullSync()
    });
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
  }
  guessDeviceName() {
    const ua = navigator.userAgent;
    if (/Android/i.test(ua))
      return "Android";
    if (/iPhone|iPad|iOS/i.test(ua))
      return "iOS";
    if (/Mac/i.test(ua))
      return "macOS";
    if (/Win/i.test(ua))
      return "Windows";
    if (/Linux/i.test(ua))
      return "Linux";
    return "Obsidian";
  }
  platform() {
    const ua = navigator.userAgent;
    if (/Android/i.test(ua))
      return "Android";
    if (/iPhone|iPad|iOS/i.test(ua))
      return "iOS";
    if (/Mac/i.test(ua))
      return "macOS";
    if (/Win/i.test(ua))
      return "Windows";
    if (/Linux/i.test(ua))
      return "Linux";
    return "unknown";
  }
  setStatus(kind, extra = "") {
    const labels = {
      offline: "Syncidian \u2022 offline",
      connecting: "Syncidian \u2022 connecting",
      syncing: "Syncidian \u2022 syncing",
      pending: "Syncidian \u2022 pending",
      ok: "Syncidian \u2022 synced",
      conflict: "Syncidian \u2022 conflict",
      error: "Syncidian \u2022 error"
    };
    this.statusEl.setText(extra ? `${labels[kind]} ${extra}` : labels[kind]);
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
    const headers = new Headers(opts.headers || {});
    headers.set("Authorization", `Bearer ${this.settings.token}`);
    if (opts.body && !headers.has("Content-Type"))
      headers.set("Content-Type", "application/json");
    let res;
    try {
      res = await fetch(this.apiUrl(path), {
        ...opts,
        method: opts.method || "GET",
        headers,
        credentials: "omit",
        mode: "cors",
        cache: "no-store"
      });
    } catch (e) {
      throw new Error(
        `Cannot reach ${this.settings.serverUrl || "(no server URL)"}. Check Server URL. (${e.message})`
      );
    }
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch (e) {
      data = { error: text };
    }
    if (!res.ok) {
      if (res.status === 401) {
        throw new Error(
          (data == null ? void 0 : data.error) || "Invalid or revoked access token. In the dashboard, sign in as a vault user (not admin), open Tokens, create a new sk_sync_ token, and paste it here."
        );
      }
      if (res.status === 403) {
        throw new Error(
          (data == null ? void 0 : data.error) || "Forbidden. Admins cannot sync a vault \u2014 use a non-admin user token from the Tokens page."
        );
      }
      throw new Error((data == null ? void 0 : data.error) || res.statusText);
    }
    return data;
  }
  async startup() {
    this.normalizeCredentials();
    if (!this.settings.token || !this.settings.serverUrl) {
      this.setStatus("offline");
      new import_obsidian.Notice("Syncidian: set Server URL and access token first");
      return false;
    }
    if (!this.settings.token.startsWith("sk_sync_")) {
      this.setStatus("error");
      new import_obsidian.Notice("Syncidian: access token must start with sk_sync_");
      return false;
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
      new import_obsidian.Notice(`Syncidian: ${e.message}`);
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
      new import_obsidian.Notice(`Syncidian sync failed: ${e.message}`);
      return false;
    } finally {
      this.syncing = false;
      if (this.pending.size)
        this.scheduleFlush();
    }
  }
  async localManifest() {
    const out = {};
    for (const file of this.app.vault.getAllLoadedFiles()) {
      if (!(file instanceof import_obsidian.TFile))
        continue;
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
    const existing = this.app.vault.getAbstractFileByPath(path);
    if (existing instanceof import_obsidian.TFile) {
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
    if (again instanceof import_obsidian.TFile) {
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
      if (raced instanceof import_obsidian.TFile) {
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
      if (!(af instanceof import_obsidian.TFile))
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
    new import_obsidian.Notice(`Syncidian conflict: ${path}`);
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
      const paths = file instanceof import_obsidian.TFile ? [file.path] : this.pathsUnder(file.path);
      if (file instanceof import_obsidian.TFile) {
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
      if (!(file instanceof import_obsidian.TFile) || ignored(file.path))
        return;
      const prev = this.pending.get(file.path);
      this.pending.set(file.path, { deleted: false, renamedFrom: prev == null ? void 0 : prev.renamedFrom });
    }
    this.scheduleFlush();
  }
  queueRename(oldPath, file) {
    if (!this.connected || this.syncing || this.applyingRemote)
      return;
    if (file instanceof import_obsidian.TFile) {
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
    for (const f of this.app.vault.getAllLoadedFiles()) {
      if (!(f instanceof import_obsidian.TFile) || ignored(f.path))
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
      new import_obsidian.Notice(`Syncidian sync failed: ${e.message}`);
    } finally {
      this.syncing = false;
      if (this.pending.size)
        this.scheduleFlush();
    }
  }
  async openSocket() {
    var _a;
    (_a = this.ws) == null ? void 0 : _a.close();
    this.normalizeCredentials();
    const base = this.settings.serverUrl.replace(/\/$/, "");
    let ticket = "";
    try {
      const t = await this.api("/api/v1/ws/ticket", { method: "POST", body: "{}" });
      ticket = (t == null ? void 0 : t.ticket) || "";
    } catch (e) {
      ticket = "";
    }
    const q = ticket ? `ticket=${encodeURIComponent(ticket)}` : `token=${encodeURIComponent(this.settings.token)}`;
    const wsUrl = base.replace(/^http/, "ws") + `/api/v1/ws?device_id=${encodeURIComponent(this.settings.deviceId)}&${q}`;
    try {
      this.ws = new WebSocket(wsUrl);
    } catch (e) {
      return;
    }
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
      window.setTimeout(() => {
        if (this.connected)
          void this.openSocket();
      }, 4e3);
    };
  }
  async loadSettings() {
    this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());
    if (!this.settings.hashes)
      this.settings.hashes = {};
    this.normalizeCredentials();
  }
  async saveSettings() {
    this.normalizeCredentials();
    await this.saveData(this.settings);
  }
};
var ConflictModal = class extends import_obsidian.Modal {
  constructor(app, plugin, id, path) {
    super(app);
    this.plugin = plugin;
    this.id = id;
    this.path = path;
  }
  async onOpen() {
    const { contentEl } = this;
    contentEl.addClass("syncidian-modal");
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
    new import_obsidian.Notice(`Resolved ${this.path}`);
    this.close();
    this.plugin.setStatus("ok");
  }
  onClose() {
    this.contentEl.empty();
  }
};
var SyncidianSettingTab = class extends import_obsidian.PluginSettingTab {
  constructor(app, plugin) {
    super(app, plugin);
    this.plugin = plugin;
  }
  display() {
    const { containerEl } = this;
    containerEl.empty();
    containerEl.createEl("h2", { text: "Syncidian" });
    containerEl.createEl("p", {
      text: "Point this vault at your Syncidian server. GitHub is configured per user in the dashboard, not here."
    });
    let urlInput = null;
    let tokenInput = null;
    let nameInput = null;
    new import_obsidian.Setting(containerEl).setName("Server URL").setDesc("Example: http://localhost:8080 or https://sync.example.com").addText((t) => {
      urlInput = t.inputEl;
      t.setPlaceholder("http://localhost:8080").setValue(this.plugin.settings.serverUrl).onChange(async (v) => {
        this.plugin.settings.serverUrl = v.trim();
        await this.plugin.saveSettings();
      });
    });
    new import_obsidian.Setting(containerEl).setName("Access token").setDesc("Created in the Syncidian dashboard Tokens page (vault user, not admin). Starts with sk_sync_").addText((t) => {
      tokenInput = t.inputEl;
      t.inputEl.type = "password";
      t.inputEl.spellcheck = false;
      t.inputEl.autocomplete = "off";
      t.setPlaceholder("sk_sync_\u2026").setValue(this.plugin.settings.token).onChange(async (v) => {
        this.plugin.settings.token = v.trim().replace(/\s+/g, "");
        await this.plugin.saveSettings();
      });
    });
    new import_obsidian.Setting(containerEl).setName("Device name").addText((t) => {
      nameInput = t.inputEl;
      t.setValue(this.plugin.settings.deviceName).onChange(async (v) => {
        this.plugin.settings.deviceName = v.trim() || "Obsidian";
        await this.plugin.saveSettings();
      });
    });
    new import_obsidian.Setting(containerEl).setName("Connect").setDesc("Register this device and run an initial sync").addButton(
      (b) => b.setButtonText("Connect").setCta().onClick(async () => {
        if (urlInput)
          this.plugin.settings.serverUrl = urlInput.value.trim();
        if (tokenInput)
          this.plugin.settings.token = tokenInput.value.trim().replace(/\s+/g, "");
        if (nameInput)
          this.plugin.settings.deviceName = nameInput.value.trim() || "Obsidian";
        await this.plugin.saveSettings();
        const ok = await this.plugin.startup();
        if (ok)
          new import_obsidian.Notice("Syncidian connected");
      })
    );
  }
};
