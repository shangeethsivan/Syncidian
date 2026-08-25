import {
  App,
  Modal,
  Notice,
  Plugin,
  PluginSettingTab,
  Setting,
  TAbstractFile,
  TFile,
  debounce,
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

export default class SyncidianPlugin extends Plugin {
  settings: SyncidianSettings = DEFAULT_SETTINGS;
  statusEl!: HTMLElement;
  connected = false;
  syncing = false;
  ws: WebSocket | null = null;

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

    this.registerEvent(this.app.vault.on("create", (f) => this.onFileEvent(f, false)));
    this.registerEvent(this.app.vault.on("modify", (f) => this.onFileEvent(f, false)));
    this.registerEvent(this.app.vault.on("delete", (f) => this.onFileEvent(f, true)));
    this.registerEvent(
      this.app.vault.on("rename", (f, old) => {
        void this.onRename(old, f);
      })
    );

    this.app.workspace.onLayoutReady(() => {
      void this.startup();
    });
  }

  onunload() {
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

  setStatus(kind: "offline" | "connecting" | "syncing" | "ok" | "conflict" | "error", extra = "") {
    const labels: Record<string, string> = {
      offline: "Syncidian • offline",
      connecting: "Syncidian • connecting",
      syncing: "Syncidian • syncing",
      ok: "Syncidian • synced",
      conflict: "Syncidian • conflict",
      error: "Syncidian • error",
    };
    this.statusEl.setText(extra ? `${labels[kind]} ${extra}` : labels[kind]);
  }

  apiUrl(path: string): string {
    return this.settings.serverUrl.replace(/\/$/, "") + path;
  }

  async api(path: string, opts: RequestInit = {}): Promise<any> {
    const headers = new Headers(opts.headers || {});
    headers.set("Authorization", `Bearer ${this.settings.token}`);
    if (opts.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
    const res = await fetch(this.apiUrl(path), { ...opts, headers });
    const text = await res.text();
    let data: any = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch {
      data = { error: text };
    }
    if (!res.ok) throw new Error(data?.error || res.statusText);
    return data;
  }

  async startup(): Promise<boolean> {
    if (!this.settings.token || !this.settings.serverUrl) {
      this.setStatus("offline");
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
    this.syncing = true;
    this.setStatus("syncing");
    try {
      if (!this.connected) await this.connect();
      const local = await this.localManifest();
      const files = Object.keys(local).map((path) => ({
        path,
        hash: local[path],
        base_hash: this.settings.hashes[path] || "",
      }));
      const plan = await this.api("/api/v1/sync/plan", {
        method: "POST",
        body: JSON.stringify({ device_id: this.settings.deviceId, files }),
      });
      for (const path of plan.Pull || []) {
        await this.pullFile(path);
      }
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
    const bytes = b64decode(remote.content || "");
    const existing = this.app.vault.getAbstractFileByPath(path);
    if (existing instanceof TFile) {
      await this.app.vault.modifyBinary(existing, bytes);
    } else {
      const dir = path.split("/").slice(0, -1).join("/");
      if (dir) await this.ensureFolder(dir);
      // Recheck: another pull or Obsidian may have created the file while we prepared the folder.
      const again = this.app.vault.getAbstractFileByPath(path);
      if (again instanceof TFile) {
        await this.app.vault.modifyBinary(again, bytes);
      } else {
        await this.app.vault.createBinary(path, bytes);
      }
    }
    this.settings.hashes[path] = remote.hash;
    await this.saveSettings();
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

  onFileEvent = debounce(
    (file: TAbstractFile, deleted: boolean) => {
      void this.handleChange(file, deleted);
    },
    400,
    true
  );

  async handleChange(file: TAbstractFile, deleted: boolean) {
    if (!(file instanceof TFile) && !deleted) return;
    const path = file.path;
    if (ignored(path) || !this.connected) return;
    this.setStatus("syncing");
    try {
      await this.pushPaths([path], deleted);
      this.setStatus("ok");
    } catch (e) {
      this.setStatus("error");
      console.error(e);
    }
  }

  async onRename(oldPath: string, file: TAbstractFile) {
    if (ignored(oldPath) && ignored(file.path)) return;
    try {
      await this.pushPaths([oldPath], true);
      if (file instanceof TFile && !ignored(file.path)) await this.pushPaths([file.path], false);
    } catch (e) {
      console.error(e);
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
  }

  async saveSettings() {
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
    new Setting(containerEl)
      .setName("Server URL")
      .setDesc("Example: http://localhost:8080 or https://sync.example.com")
      .addText((t) =>
        t.setPlaceholder("http://localhost:8080").setValue(this.plugin.settings.serverUrl).onChange(async (v) => {
          this.plugin.settings.serverUrl = v.trim();
          await this.plugin.saveSettings();
        })
      );
    new Setting(containerEl)
      .setName("Access token")
      .setDesc("Created in the Syncidian dashboard. Starts with sk_sync_")
      .addText((t) => {
        t.inputEl.type = "password";
        t.setPlaceholder("sk_sync_…").setValue(this.plugin.settings.token).onChange(async (v) => {
          this.plugin.settings.token = v.trim();
          await this.plugin.saveSettings();
        });
      });
    new Setting(containerEl)
      .setName("Device name")
      .addText((t) =>
        t.setValue(this.plugin.settings.deviceName).onChange(async (v) => {
          this.plugin.settings.deviceName = v.trim() || "Obsidian";
          await this.plugin.saveSettings();
        })
      );
    new Setting(containerEl)
      .setName("Connect")
      .setDesc("Register this device and run an initial sync")
      .addButton((b) =>
        b.setButtonText("Connect").setCta().onClick(async () => {
          const ok = await this.plugin.startup();
          if (ok) new Notice("Syncidian connected");
        })
      );
  }
}
