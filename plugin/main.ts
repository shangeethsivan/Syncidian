import {
  App,
  Modal,
  Notice,
  Plugin,
  PluginSettingTab,
  Setting,
  TAbstractFile,
  TFile,
} from "obsidian";

interface SyncidianSettings {
  serverUrl: string;
  token: string;
  deviceName: string;
  deviceId: string;
  hashes: Record<string, string>;
}

const DEFAULT_SETTINGS: SyncidianSettings = {
  serverUrl: "http://localhost:8080",
  token: "",
  deviceName: "Obsidian",
  deviceId: "",
  hashes: {},
};

/** Wait until typing/edits stop, then wait this long before pushing. */
const IDLE_SYNC_MS = 3000;

const IGNORE = [
  /^\.obsidian\/workspace(-mobile)?\.json$/,
  /^\.obsidian\/workspace-/,
  /^\.obsidian\/plugins\/syncidian\//,
  /^\.trash\//,
  /^\.git\//,
  /(^|\/)\.DS_Store$/,
];

function ignored(path: string): boolean {
  return IGNORE.some((re) => re.test(path));
}

async function sha256(data: ArrayBuffer): Promise<string> {
  const buf = await crypto.subtle.digest("SHA-256", data);
  return Array.from(new Uint8Array(buf))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

function b64encode(bytes: Uint8Array): string {
  let bin = "";
  bytes.forEach((b) => (bin += String.fromCharCode(b)));
  return btoa(bin);
}

function b64decode(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function toArrayBuffer(data: Uint8Array): ArrayBuffer {
  return data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength) as ArrayBuffer;
}

export default class SyncidianPlugin extends Plugin {
  settings: SyncidianSettings = DEFAULT_SETTINGS;
  statusEl!: HTMLElement;
  connected = false;
  syncing = false;
  ws: WebSocket | null = null;
  pending = new Map<string, boolean>();
  idleTimer: number | null = null;

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
      callback: () => void this.fullSync(),
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
    this.clearIdleTimer();
    this.ws?.close();
    this.ws = null;
  }

  guessDeviceName(): string {
    const ua = navigator.userAgent;
    if (/Android/i.test(ua)) return "Android";
    if (/iPhone|iPad|iOS/i.test(ua)) return "iOS";
    if (/Mac/i.test(ua)) return "macOS";
    if (/Win/i.test(ua)) return "Windows";
    if (/Linux/i.test(ua)) return "Linux";
    return "Obsidian";
  }

  platform(): string {
    const ua = navigator.userAgent;
    if (/Android/i.test(ua)) return "Android";
    if (/iPhone|iPad|iOS/i.test(ua)) return "iOS";
    if (/Mac/i.test(ua)) return "macOS";
    if (/Win/i.test(ua)) return "Windows";
    if (/Linux/i.test(ua)) return "Linux";
    return "unknown";
  }

  setStatus(kind: "offline" | "connecting" | "syncing" | "pending" | "ok" | "conflict" | "error", extra = "") {
    const labels: Record<string, string> = {
      offline: "Syncidian • offline",
      connecting: "Syncidian • connecting",
      syncing: "Syncidian • syncing",
      pending: "Syncidian • pending",
      ok: "Syncidian • synced",
      conflict: "Syncidian • conflict",
      error: "Syncidian • error",
    };
    this.statusEl.setText(extra ? `${labels[kind]} ${extra}` : labels[kind]);
  }

  apiUrl(path: string): string {
    return this.settings.serverUrl.replace(/\/$/, "") + path;
  }

  /** Normalize URL/token before every request (trim paste noise). */
  normalizeCredentials() {
    this.settings.serverUrl = (this.settings.serverUrl || "").trim().replace(/\/$/, "");
    this.settings.token = (this.settings.token || "").trim().replace(/\s+/g, "");
  }

  async api(path: string, opts: RequestInit = {}): Promise<any> {
    this.normalizeCredentials();
    const headers = new Headers(opts.headers || {});
    headers.set("Authorization", `Bearer ${this.settings.token}`);
    if (opts.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
    let res: Response;
    try {
      res = await fetch(this.apiUrl(path), { ...opts, headers });
    } catch (e) {
      throw new Error(
        `Cannot reach ${this.settings.serverUrl || "(no server URL)"}. Check Server URL. (${(e as Error).message})`
      );
    }
    const text = await res.text();
    let data: any = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch {
      data = { error: text };
    }
    if (!res.ok) {
      if (res.status === 401) {
        throw new Error(
          data?.error ||
            "Invalid or revoked access token. In the dashboard, sign in as a vault user (not admin), open Tokens, create a new sk_sync_ token, and paste it here."
        );
      }
      if (res.status === 403) {
        throw new Error(
          data?.error ||
            "Forbidden. Admins cannot sync a vault — use a non-admin user token from the Tokens page."
        );
      }
      throw new Error(data?.error || res.statusText);
    }
    return data;
  }

  async startup(): Promise<boolean> {
    this.normalizeCredentials();
    if (!this.settings.token || !this.settings.serverUrl) {
      this.setStatus("offline");
      new Notice("Syncidian: set Server URL and access token first");
      return false;
    }
    if (!this.settings.token.startsWith("sk_sync_")) {
      this.setStatus("error");
      new Notice("Syncidian: access token must start with sk_sync_");
      return false;
    }
    try {
      await this.connect();
      const ok = await this.fullSync();
      if (!ok) return false;
      this.openSocket();
      return true;
    } catch (e) {
      console.error(e);
      this.setStatus("error");
      new Notice(`Syncidian: ${(e as Error).message}`);
      return false;
    }
  }

  async connect() {
    this.setStatus("connecting");
    const body = {
      id: this.settings.deviceId || undefined,
      name: this.settings.deviceName,
      platform: this.platform(),
      plugin_version: this.manifest.version,
    };
    const d = await this.api("/api/v1/devices/register", {
      method: "POST",
      body: JSON.stringify(body),
    });
    this.settings.deviceId = d.id;
    await this.saveSettings();
    this.connected = true;
  }

  async fullSync(): Promise<boolean> {
    if (!this.settings.token) return false;
    if (this.syncing) return false;
    this.clearIdleTimer();
    this.pending.clear();
    this.syncing = true;
    this.setStatus("syncing");
    try {
      if (!this.connected) await this.connect();
      const local = await this.localManifest();
      const files: { path: string; hash: string; base_hash: string; deleted?: boolean }[] = Object.keys(local).map(
        (path) => ({
          path,
          hash: local[path],
          base_hash: this.settings.hashes[path] || "",
        })
      );
      for (const path of Object.keys(this.settings.hashes)) {
        if (path in local || ignored(path)) continue;
        files.push({
          path,
          hash: "",
          deleted: true,
          base_hash: this.settings.hashes[path] || "",
        });
      }
      const plan = await this.api("/api/v1/sync/plan", {
        method: "POST",
        body: JSON.stringify({ device_id: this.settings.deviceId, files }),
      });
      for (const path of plan.Pull || []) {
        await this.pullFile(path);
      }
      if ((plan.Delete || []).length) await this.pushPaths(plan.Delete, true);
      const toPush: string[] = plan.Push || [];
      if (toPush.length) await this.pushPaths(toPush, false);
      for (const path of plan.Conflicts || []) {
        await this.raiseConflict(path);
      }
      this.setStatus((plan.Conflicts || []).length ? "conflict" : "ok");
      return true;
    } catch (e) {
      this.setStatus("error");
      new Notice(`Syncidian sync failed: ${(e as Error).message}`);
      return false;
    } finally {
      this.syncing = false;
      if (this.pending.size) this.scheduleFlush();
    }
  }

  async localManifest(): Promise<Record<string, string>> {
    const out: Record<string, string> = {};
    for (const file of this.app.vault.getAllLoadedFiles()) {
      if (!(file instanceof TFile)) continue;
      if (ignored(file.path)) continue;
      const data = await this.app.vault.readBinary(file);
      out[file.path] = await sha256(data);
    }
    return out;
  }

  async pullFile(path: string) {
    const remote = await this.api(`/api/v1/sync/file?path=${encodeURIComponent(path)}`);
    if (remote.deleted) {
      const existing = this.app.vault.getAbstractFileByPath(path);
      if (existing) await this.app.vault.delete(existing);
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
  async writeBinary(path: string, data: ArrayBuffer) {
    const existing = this.app.vault.getAbstractFileByPath(path);
    if (existing instanceof TFile) {
      await this.app.vault.modifyBinary(existing, data);
      return;
    }
    if (existing) {
      throw new Error(`Cannot write file; path is a folder: ${path}`);
    }
    const dir = path.split("/").slice(0, -1).join("/");
    if (dir) await this.ensureFolder(dir);
    const again = this.app.vault.getAbstractFileByPath(path);
    if (again instanceof TFile) {
      await this.app.vault.modifyBinary(again, data);
      return;
    }
    try {
      await this.app.vault.createBinary(path, data);
    } catch (e) {
      const msg = (e as Error).message || String(e);
      if (!/already exists/i.test(msg)) throw e;
      const raced = this.app.vault.getAbstractFileByPath(path);
      if (raced instanceof TFile) {
        await this.app.vault.modifyBinary(raced, data);
        return;
      }
      // On disk but not indexed yet — write through the adapter.
      await this.app.vault.adapter.writeBinary(path, data);
    }
  }

  async ensureFolder(dir: string) {
    const parts = dir.split("/").filter(Boolean);
    let cur = "";
    for (const p of parts) {
      cur = cur ? `${cur}/${p}` : p;
      if (this.app.vault.getAbstractFileByPath(cur)) continue;
      // Vault index can lag the filesystem (existing vault folders, prior syncs).
      try {
        if (await this.app.vault.adapter.exists(cur)) continue;
        await this.app.vault.createFolder(cur);
      } catch (e) {
        const msg = (e as Error).message || String(e);
        if (/already exists/i.test(msg)) continue;
        throw e;
      }
    }
  }

  async pushPaths(paths: string[], deleted: boolean) {
    const files = [];
    for (const path of paths) {
      if (deleted) {
        files.push({
          path,
          hash: "",
          deleted: true,
          base_hash: this.settings.hashes[path] || "",
          mtime: Date.now() / 1000,
          content: "",
        });
        continue;
      }
      const af = this.app.vault.getAbstractFileByPath(path);
      if (!(af instanceof TFile)) continue;
      const data = await this.app.vault.readBinary(af);
      const hash = await sha256(data);
      files.push({
        path,
        hash,
        deleted: false,
        base_hash: this.settings.hashes[path] || "",
        mtime: Math.floor(af.stat.mtime / 1000),
        content: b64encode(new Uint8Array(data)),
      });
    }
    if (!files.length) return;
    const res = await this.api("/api/v1/sync/push", {
      method: "POST",
      body: JSON.stringify({ device_id: this.settings.deviceId, files }),
    });
    for (const p of res.accepted || []) {
      const f = files.find((x) => x.path === p);
      if (f?.deleted) delete this.settings.hashes[p];
      else if (f) this.settings.hashes[p] = f.hash;
      else delete this.settings.hashes[p];
    }
    await this.saveSettings();
    for (const c of res.conflicts || []) {
      new ConflictModal(this.app, this, c.id, c.path).open();
      this.setStatus("conflict");
    }
  }

  async raiseConflict(path: string) {
    new Notice(`Syncidian conflict: ${path}`);
    this.setStatus("conflict", path);
  }

  pathsUnder(prefix: string): string[] {
    const folder = prefix.replace(/\/$/, "");
    const child = folder + "/";
    const out: string[] = [];
    for (const p of Object.keys(this.settings.hashes)) {
      if (p === folder || p.startsWith(child)) out.push(p);
    }
    for (const p of this.pending.keys()) {
      if ((p === folder || p.startsWith(child)) && !out.includes(p)) out.push(p);
    }
    return out;
  }

  queueChange(file: TAbstractFile, deleted: boolean) {
    if (!this.connected) return;
    if (deleted) {
      const paths = file instanceof TFile ? [file.path] : this.pathsUnder(file.path);
      if (file instanceof TFile) {
        if (ignored(file.path)) return;
      } else if (!paths.length && ignored(file.path)) {
        return;
      }
      for (const p of paths.length ? paths : [file.path]) {
        if (ignored(p)) continue;
        this.pending.set(p, true);
      }
    } else {
      if (!(file instanceof TFile) || ignored(file.path)) return;
      this.pending.set(file.path, false);
    }
    this.scheduleFlush();
  }

  queueRename(oldPath: string, file: TAbstractFile) {
    if (!this.connected) return;
    const oldPaths = this.pathsUnder(oldPath);
    for (const p of oldPaths.length ? oldPaths : ignored(oldPath) ? [] : [oldPath]) {
      this.pending.set(p, true);
    }
    if (file instanceof TFile) {
      if (!ignored(file.path)) this.pending.set(file.path, false);
    } else {
      const oldPrefix = oldPath.replace(/\/$/, "") + "/";
      const newPrefix = file.path.replace(/\/$/, "") + "/";
      for (const p of Object.keys(this.settings.hashes)) {
        if (p === oldPath || p.startsWith(oldPrefix)) {
          const np = p === oldPath ? file.path : newPrefix + p.slice(oldPrefix.length);
          if (!ignored(np)) this.pending.set(np, false);
        }
      }
    }
    this.scheduleFlush();
  }

  scheduleFlush() {
    this.clearIdleTimer();
    if (this.connected && !this.syncing) this.setStatus("pending");
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
    if (!this.pending.size) return;
    const snapshot = this.pending;
    this.pending = new Map();
    const toDelete: string[] = [];
    const toPush: string[] = [];
    for (const [p, deleted] of snapshot) {
      if (deleted) toDelete.push(p);
      else toPush.push(p);
    }
    this.syncing = true;
    this.setStatus("syncing");
    try {
      if (toDelete.length) await this.pushPaths(toDelete, true);
      if (toPush.length) await this.pushPaths(toPush, false);
      this.setStatus("ok");
    } catch (e) {
      this.setStatus("error");
      console.error(e);
      new Notice(`Syncidian sync failed: ${(e as Error).message}`);
    } finally {
      this.syncing = false;
      if (this.pending.size) this.scheduleFlush();
    }
  }

  openSocket() {
    this.ws?.close();
    const base = this.settings.serverUrl.replace(/\/$/, "");
    const wsUrl =
      base.replace(/^http/, "ws") +
      `/api/v1/ws?device_id=${encodeURIComponent(this.settings.deviceId)}&token=${encodeURIComponent(this.settings.token)}`;
    try {
      this.ws = new WebSocket(wsUrl);
    } catch {
      return;
    }
    this.ws.onmessage = (ev) => {
      void (async () => {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === "file_changed" && msg.path) {
            if (msg.deleted) {
              const f = this.app.vault.getAbstractFileByPath(msg.path);
              if (f) await this.app.vault.delete(f);
              delete this.settings.hashes[msg.path];
              await this.saveSettings();
            } else {
              await this.pullFile(msg.path);
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
        if (this.connected) this.openSocket();
      }, 4000);
    };
  }

  async loadSettings() {
    this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());
    if (!this.settings.hashes) this.settings.hashes = {};
    this.normalizeCredentials();
  }

  async saveSettings() {
    this.normalizeCredentials();
    await this.saveData(this.settings);
  }
}

class ConflictModal extends Modal {
  constructor(
    app: App,
    private plugin: SyncidianPlugin,
    private id: string,
    private path: string
  ) {
    super(app);
  }

  async onOpen() {
    const { contentEl } = this;
    contentEl.addClass("syncidian-modal");
    contentEl.createEl("h2", { text: "Sync conflict" });
    contentEl.createEl("p", { text: `${this.path} was modified on more than one device.` });
    let data: any = {};
    try {
      data = await this.plugin.api(`/api/v1/conflicts/${this.id}`);
    } catch (e) {
      contentEl.createEl("p", { text: (e as Error).message });
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
    row.createEl("button", { text: "Save merge", cls: "mod-cta" }).onclick = () =>
      void this.resolve("merged", mergeTa.value);
  }

  async resolve(resolution: string, content?: string) {
    await this.plugin.api(`/api/v1/conflicts/${this.id}/resolve`, {
      method: "POST",
      body: JSON.stringify({
        resolution,
        content: content || "",
        device_id: this.plugin.settings.deviceId,
      }),
    });
    await this.plugin.pullFile(this.path);
    new Notice(`Resolved ${this.path}`);
    this.close();
    this.plugin.setStatus("ok");
  }

  onClose() {
    this.contentEl.empty();
  }
}

class SyncidianSettingTab extends PluginSettingTab {
  plugin: SyncidianPlugin;
  constructor(app: App, plugin: SyncidianPlugin) {
    super(app, plugin);
    this.plugin = plugin;
  }
  display() {
    const { containerEl } = this;
    containerEl.empty();
    containerEl.createEl("h2", { text: "Syncidian" });
    containerEl.createEl("p", {
      text: "Point this vault at your Syncidian server. GitHub is configured per user in the dashboard, not here.",
    });
    let urlInput: HTMLInputElement | null = null;
    let tokenInput: HTMLInputElement | null = null;
    let nameInput: HTMLInputElement | null = null;
    new Setting(containerEl)
      .setName("Server URL")
      .setDesc("Example: http://localhost:8080 or https://sync.example.com")
      .addText((t) => {
        urlInput = t.inputEl;
        t.setPlaceholder("http://localhost:8080").setValue(this.plugin.settings.serverUrl).onChange(async (v) => {
          this.plugin.settings.serverUrl = v.trim();
          await this.plugin.saveSettings();
        });
      });
    new Setting(containerEl)
      .setName("Access token")
      .setDesc("Created in the Syncidian dashboard Tokens page (vault user, not admin). Starts with sk_sync_")
      .addText((t) => {
        tokenInput = t.inputEl;
        t.inputEl.type = "password";
        t.inputEl.spellcheck = false;
        t.inputEl.autocomplete = "off";
        t.setPlaceholder("sk_sync_…").setValue(this.plugin.settings.token).onChange(async (v) => {
          this.plugin.settings.token = v.trim().replace(/\s+/g, "");
          await this.plugin.saveSettings();
        });
      });
    new Setting(containerEl)
      .setName("Device name")
      .addText((t) => {
        nameInput = t.inputEl;
        t.setValue(this.plugin.settings.deviceName).onChange(async (v) => {
          this.plugin.settings.deviceName = v.trim() || "Obsidian";
          await this.plugin.saveSettings();
        });
      });
    new Setting(containerEl)
      .setName("Connect")
      .setDesc("Register this device and run an initial sync")
      .addButton((b) =>
        b.setButtonText("Connect").setCta().onClick(async () => {
          // Read inputs directly so Connect works even if the last keystroke/paste
          // has not flushed through onChange yet.
          if (urlInput) this.plugin.settings.serverUrl = urlInput.value.trim();
          if (tokenInput) this.plugin.settings.token = tokenInput.value.trim().replace(/\s+/g, "");
          if (nameInput) this.plugin.settings.deviceName = nameInput.value.trim() || "Obsidian";
          await this.plugin.saveSettings();
          const ok = await this.plugin.startup();
          if (ok) new Notice("Syncidian connected");
        })
      );
  }
}
