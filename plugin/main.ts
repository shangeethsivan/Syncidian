import {
  App,
  Modal,
  Notice,
  Platform,
  Plugin,
  PluginSettingTab,
  Setting,
  TAbstractFile,
  TFile,
  TFolder,
  normalizePath,
  requestUrl,
} from "obsidian";
import { b64decode, b64encode, toArrayBuffer } from "./codec";
import { sha256 } from "./hash";
import {
  devicePlatform,
  guessDeviceName,
  isInsecureHttp,
  isLoopbackUrl,
  isMobileApp,
  pushBatchSize,
} from "./mobile";

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
/** Collapse focus + visibility + online events from one app switch. */
const RESUME_SYNC_MIN_MS = 2000;

interface PendingOp {
  deleted: boolean;
  renamedFrom?: string;
}

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

function listedConflictId(c: { id?: string; ID?: string }): string {
  return c.id || c.ID || "";
}

function listedConflictPath(c: { path?: string; Path?: string }): string {
  return c.path || c.Path || "";
}

export default class SyncidianPlugin extends Plugin {
  settings: SyncidianSettings = DEFAULT_SETTINGS;
  statusEl: HTMLElement | null = null;
  ribbonEl: HTMLElement | null = null;
  connected = false;
  syncing = false;
  booting = false;
  ws: WebSocket | null = null;
  wsConnecting = false;
  pending = new Map<string, PendingOp>();
  idleTimer: number | null = null;
  applyingRemote = 0;
  lastResumeSync = 0;
  conflictQueue: { id: string; path: string }[] = [];
  activeConflictId: string | null = null;

  async onload() {
    await this.loadSettings();
    if (!this.settings.deviceName || this.settings.deviceName === "Obsidian") {
      this.settings.deviceName = guessDeviceName();
    }
    // Status bar is desktop-only; the ribbon is the mobile-visible control.
    this.statusEl = this.addStatusBarItem();
    this.statusEl.addClass("syncidian-status");
    this.statusEl.addClass("mod-clickable");
    this.registerDomEvent(this.statusEl, "click", () => {
      void this.onStatusClick();
    });
    this.ribbonEl = this.addRibbonIcon("sync", "Syncidian: sync now", () => {
      void this.fullSync();
    });
    this.setStatus("offline");
    this.addSettingTab(new SyncidianSettingTab(this.app, this));
    this.addCommand({
      id: "syncidian-sync-now",
      name: "Sync now",
      callback: () => void this.fullSync(),
    });
    this.registerInterval(
      window.setInterval(() => {
        if (this.connected && !this.ws && !this.wsConnecting) void this.openSocket();
      }, 8000)
    );
    this.registerInterval(
      window.setInterval(() => {
        if (this.connected && !this.ws && !this.syncing) void this.pollRemote();
      }, 15000)
    );
    // Desktop and mobile both freeze timers and WebSockets while backgrounded.
    // Polling only when `!this.ws` misses updates if the socket looks open but
    // was asleep. Re-check when the vault window (or phone app) is foregrounded.
    this.registerDomEvent(document, "visibilitychange", () => {
      if (document.visibilityState === "visible") void this.onForeground();
    });
    this.registerDomEvent(window, "focus", () => {
      void this.onForeground();
    });
    this.registerDomEvent(window, "online", () => {
      void this.onForeground();
    });
    const onCapacitorResume = () => void this.onForeground();
    document.addEventListener("resume", onCapacitorResume);
    this.register(() => document.removeEventListener("resume", onCapacitorResume));

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
    this.wsConnecting = false;
  }

  platform(): string {
    return devicePlatform();
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
    const text = extra ? `${labels[kind]} ${extra}` : labels[kind];
    this.statusEl?.removeClass(
      "is-offline",
      "is-connecting",
      "is-syncing",
      "is-pending",
      "is-ok",
      "is-conflict",
      "is-error"
    );
    this.statusEl?.addClass(`is-${kind}`);
    this.statusEl?.setText(text);
    this.ribbonEl?.setAttribute("aria-label", text);
    this.ribbonEl?.setAttribute("title", `${text} — tap to sync now`);
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
    const headers: Record<string, string> = {
      Authorization: `Bearer ${this.settings.token}`,
      "X-Syncidian-Client": `obsidian-plugin/${this.manifest.version}`,
    };
    const body = typeof opts.body === "string" ? opts.body : undefined;
    if (body) headers["Content-Type"] = "application/json";
    const method = (opts.method || "GET").toString().toUpperCase();
    let status = 0;
    let text = "";
    try {
      // requestUrl bypasses CORS and mixed-content blocks in the Android/iOS WebView.
      // Plain fetch() is what makes community plugins like Git fail or get blocked on mobile.
      const res = await requestUrl({
        url: this.apiUrl(path),
        method,
        headers,
        contentType: body ? "application/json" : undefined,
        body,
        throw: false,
      });
      status = res.status;
      text = res.text;
    } catch (e) {
      throw new Error(
        `Cannot reach ${this.settings.serverUrl || "(no server URL)"}. On a phone, use a public HTTPS URL, not localhost. (${(e as Error).message})`
      );
    }
    let data: any = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch {
      data = { error: text };
    }
    if (status < 200 || status >= 300) {
      if (status === 401) {
        throw new Error(
          data?.error ||
            "Invalid or revoked access token. In the dashboard, sign in as a vault user (not admin), open Tokens, create a new sk_sync_ token, and paste it here."
        );
      }
      if (status === 403) {
        throw new Error(
          data?.error ||
            "Forbidden. Admins cannot sync a vault — use a non-admin user token from the Tokens page."
        );
      }
      throw new Error(data?.error || `HTTP ${status}`);
    }
    return data;
  }

  mobileUrlError(): string | null {
    if (!isMobileApp()) return null;
    if (isLoopbackUrl(this.settings.serverUrl)) {
      return "On Android and iOS, localhost is this device, not your computer. Set Server URL to your public HTTPS address (for example your Railway domain).";
    }
    return null;
  }

  async startup(): Promise<boolean> {
    if (this.booting) return false;
    this.booting = true;
    try {
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
      const mobileErr = this.mobileUrlError();
      if (mobileErr) {
        this.setStatus("error");
        new Notice(`Syncidian: ${mobileErr}`);
        return false;
      }
      if (isMobileApp() && isInsecureHttp(this.settings.serverUrl) && Platform.isIosApp) {
        new Notice("Syncidian: iOS often blocks plain HTTP. Prefer an https:// server URL.");
      }
      try {
        await this.connect();
        const ok = await this.fullSync();
        if (!ok) return false;
        await this.openSocket();
        return true;
      } catch (e) {
        console.error(e);
        this.setStatus("error");
        new Notice(`Syncidian: ${(e as Error).message}`);
        return false;
      }
    } finally {
      this.booting = false;
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
      let githubLayout = false;
      try {
        await this.api("/api/v1/github/sync", { method: "POST", body: "{}" });
        githubLayout = true;
      } catch (e) {
        const msg = (e as Error).message || String(e);
        if (!/GitHub is not configured/i.test(msg)) {
          throw e;
        }
      }
      if (githubLayout) {
        await this.dropLocalNotOnServer();
      }
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
      if ((plan.Delete || []).length || (plan.Push || []).length) {
        await this.pushBatch(plan.Delete || [], plan.Push || []);
      }
      await this.resolvePlanConflicts(plan.Conflicts || []);
      await this.pruneEmptyFolders();
      if (!this.activeConflictId && !this.conflictQueue.length) this.setStatus("ok");
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
    // getFiles() is complete on mobile; getAllLoadedFiles() can miss vault files that are not in the editor cache.
    for (const file of this.app.vault.getFiles()) {
      if (ignored(file.path)) continue;
      const data = await this.app.vault.readBinary(file);
      out[file.path] = await sha256(data);
    }
    return out;
  }

  async pullFile(path: string) {
    const remote = await this.api(`/api/v1/sync/file?path=${encodeURIComponent(path)}`);
    if (remote.deleted) {
      await this.removeLocalPath(path);
      delete this.settings.hashes[path];
      await this.saveSettings();
      return;
    }
    const bytes = toArrayBuffer(b64decode(remote.content || ""));
    await this.writeBinary(path, bytes);
    this.settings.hashes[path] = remote.hash;
    await this.saveSettings();
  }

  async removeLocalPath(path: string) {
    path = normalizePath(path);
    const existing = this.app.vault.getAbstractFileByPath(path);
    if (existing) await this.app.vault.delete(existing);
    await this.pruneEmptyParents(path);
  }

  async pruneEmptyParents(filePath: string) {
    const parts = normalizePath(filePath).split("/").filter(Boolean);
    parts.pop();
    while (parts.length) {
      const dir = parts.join("/");
      const af = this.app.vault.getAbstractFileByPath(dir);
      if (!af || af instanceof TFile) return;
      const folder = af as TFolder;
      if (folder.children && folder.children.length > 0) return;
      try {
        await this.app.vault.delete(folder);
      } catch {
        return;
      }
      parts.pop();
    }
  }

  keepFolder(path: string): boolean {
    path = normalizePath(path);
    return (
      path === ".obsidian" ||
      path.startsWith(".obsidian/") ||
      path === ".obsidian-mobile" ||
      path.startsWith(".obsidian-mobile/") ||
      path === ".trash" ||
      path.startsWith(".trash/") ||
      path === ".git" ||
      path.startsWith(".git/")
    );
  }

  /** After GitHub import, delete vault files that are not in the live server tree. */
  async dropLocalNotOnServer() {
    const man = await this.api("/api/v1/sync/manifest");
    const remote = new Set<string>();
    for (const item of man.files || []) {
      const path = normalizePath(item.path || "");
      if (path) remote.add(path);
    }
    for (const file of this.app.vault.getFiles()) {
      if (ignored(file.path) || remote.has(normalizePath(file.path))) continue;
      await this.removeLocalPath(file.path);
      delete this.settings.hashes[file.path];
    }
    await this.pruneEmptyFolders();
    await this.saveSettings();
  }

  async pruneEmptyFolders() {
    const folders: TFolder[] = [];
    const walk = (folder: TFolder) => {
      for (const child of folder.children) {
        if (child instanceof TFolder) {
          walk(child);
          folders.push(child);
        }
      }
    };
    walk(this.app.vault.getRoot());
    folders.sort((a, b) => b.path.length - a.path.length);
    for (const folder of folders) {
      if (this.keepFolder(folder.path)) continue;
      if (folder.children.length > 0) continue;
      try {
        await this.app.vault.delete(folder);
      } catch {
        /* folder may already be gone */
      }
    }
  }

  /** Write bytes, tolerating vault-index lag vs files already on disk. */
  async writeBinary(path: string, data: ArrayBuffer) {
    path = normalizePath(path);
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
    if (deleted) await this.pushBatch(paths, []);
    else await this.pushBatch([], paths);
  }

  async pushBatch(deletes: string[], upserts: string[], renamedFrom: Record<string, string> = {}) {
    const files = [];
    const movedAway = new Set(Object.values(renamedFrom).filter(Boolean));
    for (const path of deletes) {
      if (movedAway.has(path)) continue;
      files.push({
        path,
        hash: "",
        deleted: true,
        renamed_from: "",
        base_hash: this.settings.hashes[path] || "",
        mtime: Math.floor(Date.now() / 1000),
        content: "",
      });
    }
    for (const path of upserts) {
      const af = this.app.vault.getAbstractFileByPath(path);
      if (!(af instanceof TFile)) continue;
      const data = await this.app.vault.readBinary(af);
      const hash = await sha256(data);
      const from = renamedFrom[path] || "";
      files.push({
        path,
        hash,
        deleted: false,
        renamed_from: from,
        base_hash: this.settings.hashes[from] || this.settings.hashes[path] || "",
        mtime: Math.floor(af.stat.mtime / 1000),
        content: b64encode(new Uint8Array(data)),
      });
    }
    if (!files.length) return;
    const chunk = pushBatchSize();
    for (let i = 0; i < files.length; i += chunk) {
      const slice = files.slice(i, i + chunk);
      const last = i + chunk >= files.length;
      await this.sendPush(slice, last ? movedAway : new Set());
    }
  }

  async sendPush(
    files: {
      path: string;
      hash: string;
      deleted: boolean;
      renamed_from: string;
      base_hash: string;
      mtime: number;
      content: string;
    }[],
    movedAway: Set<string>
  ) {
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
    for (const old of movedAway) delete this.settings.hashes[old];
    await this.saveSettings();
    for (const c of res.conflicts || []) {
      this.enqueueConflict(c.id, c.path);
    }
  }

  async onStatusClick() {
    if (await this.openStoredConflicts()) return;
    void this.fullSync();
  }

  async listOpenConflicts(): Promise<{ id?: string; ID?: string; path?: string; Path?: string }[]> {
    try {
      const res = await this.api("/api/v1/conflicts");
      return Array.isArray(res) ? res : [];
    } catch {
      return [];
    }
  }

  async openStoredConflicts(): Promise<boolean> {
    const items = await this.listOpenConflicts();
    if (!items.length) return false;
    for (const c of items) {
      const id = listedConflictId(c);
      if (id) this.enqueueConflict(id, listedConflictPath(c));
    }
    return true;
  }

  /** Full sync reports conflict paths without ids; open a stored conflict or push to create one. */
  async resolvePlanConflicts(paths: string[]) {
    if (!paths.length) return;
    const stored = await this.listOpenConflicts();
    const remaining: string[] = [];
    for (const path of paths) {
      const hit = stored.find((c) => listedConflictPath(c) === path);
      const id = hit ? listedConflictId(hit) : "";
      if (id) this.enqueueConflict(id, path);
      else remaining.push(path);
    }
    if (!remaining.length) return;
    const deletes: string[] = [];
    const upserts: string[] = [];
    for (const path of remaining) {
      if (this.app.vault.getAbstractFileByPath(path) instanceof TFile) upserts.push(path);
      else deletes.push(path);
    }
    await this.pushBatch(deletes, upserts);
  }

  enqueueConflict(id: string, path: string) {
    if (!id) return;
    if (this.activeConflictId === id) return;
    if (this.conflictQueue.some((c) => c.id === id)) return;
    if (this.activeConflictId) {
      this.conflictQueue.push({ id, path });
      return;
    }
    this.presentConflict(id, path);
  }

  presentConflict(id: string, path: string) {
    this.activeConflictId = id;
    this.setStatus("conflict", path);
    new ConflictModal(this.app, this, id, path).open();
  }

  onConflictResolved() {
    this.activeConflictId = null;
    const next = this.conflictQueue.shift();
    if (next) this.presentConflict(next.id, next.path);
    else this.setStatus("ok");
  }

  onConflictModalClosed(id: string) {
    if (this.activeConflictId !== id) return;
    this.activeConflictId = null;
    const next = this.conflictQueue.shift();
    if (next) this.presentConflict(next.id, next.path);
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
    if (!this.connected || this.syncing || this.applyingRemote) return;
    if (deleted) {
      const paths = file instanceof TFile ? [file.path] : this.pathsUnder(file.path);
      if (file instanceof TFile) {
        if (ignored(file.path)) return;
      } else if (!paths.length && ignored(file.path)) {
        return;
      }
      for (const p of paths.length ? paths : [file.path]) {
        if (ignored(p)) continue;
        this.pending.set(p, { deleted: true });
      }
    } else {
      if (!(file instanceof TFile) || ignored(file.path)) return;
      const prev = this.pending.get(file.path);
      this.pending.set(file.path, { deleted: false, renamedFrom: prev?.renamedFrom });
    }
    this.scheduleFlush();
  }

  queueRename(oldPath: string, file: TAbstractFile) {
    if (!this.connected || this.syncing || this.applyingRemote) return;
    if (file instanceof TFile) {
      if (!ignored(oldPath)) this.pending.set(oldPath, { deleted: true });
      if (!ignored(file.path)) {
        this.pending.set(file.path, { deleted: false, renamedFrom: oldPath });
      }
    } else {
      this.queueFolderRename(oldPath, file.path);
    }
    this.scheduleFlush();
  }

  queueFolderRename(oldPath: string, newPath: string) {
    const oldFolder = oldPath.replace(/\/$/, "");
    const newFolder = newPath.replace(/\/$/, "");
    const remap = (p: string): string | null => {
      if (p === oldFolder) return newFolder;
      if (p.startsWith(oldFolder + "/")) return newFolder + p.slice(oldFolder.length);
      return null;
    };
    for (const [p, op] of [...this.pending.entries()]) {
      const np = remap(p);
      if (np == null) continue;
      this.pending.delete(p);
      if (!ignored(p)) this.pending.set(p, { deleted: true });
      if (!ignored(np)) {
        this.pending.set(np, { deleted: op.deleted, renamedFrom: op.renamedFrom || p });
      }
    }
    for (const p of Object.keys(this.settings.hashes)) {
      const np = remap(p);
      if (np == null || ignored(np)) continue;
      this.pending.set(p, { deleted: true });
      this.pending.set(np, { deleted: false, renamedFrom: p });
    }
    for (const f of this.app.vault.getFiles()) {
      if (ignored(f.path)) continue;
      if (f.path !== newFolder && !f.path.startsWith(newFolder + "/")) continue;
      const rel = f.path === newFolder ? "" : f.path.slice(newFolder.length + 1);
      const oldP = rel ? `${oldFolder}/${rel}` : oldFolder;
      if (!ignored(oldP)) this.pending.set(oldP, { deleted: true });
      this.pending.set(f.path, { deleted: false, renamedFrom: oldP });
    }
    if (!this.pathsUnder(oldFolder).length && !ignored(oldFolder)) {
      this.pending.set(oldFolder, { deleted: true });
    }
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
    const renamedFrom: Record<string, string> = {};
    for (const [p, op] of snapshot) {
      if (op.deleted) toDelete.push(p);
      else {
        toPush.push(p);
        if (op.renamedFrom) renamedFrom[p] = op.renamedFrom;
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
      new Notice(`Syncidian sync failed: ${(e as Error).message}`);
    } finally {
      this.syncing = false;
      if (this.pending.size) this.scheduleFlush();
    }
  }

  async openSocket() {
    if (this.wsConnecting || this.ws) return;
    this.wsConnecting = true;
    this.normalizeCredentials();
    const base = this.settings.serverUrl.replace(/\/$/, "");
    let ticket = "";
    try {
      const t = await this.api("/api/v1/ws/ticket", { method: "POST", body: "{}" });
      ticket = t?.ticket || "";
    } catch {
      this.wsConnecting = false;
      return;
    }
    if (!ticket) {
      this.wsConnecting = false;
      return;
    }
    const wsUrl =
      base.replace(/^http/, "ws") +
      `/api/v1/ws?device_id=${encodeURIComponent(this.settings.deviceId)}&ticket=${encodeURIComponent(ticket)}`;
    try {
      this.ws = new WebSocket(wsUrl);
    } catch {
      this.wsConnecting = false;
      return;
    }
    this.ws.onopen = () => {
      this.wsConnecting = false;
    };
    this.ws.onerror = () => {
      this.wsConnecting = false;
      try {
        this.ws?.close();
      } catch {
        /* ignore */
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
                await this.removeLocalPath(msg.path);
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

  /**
   * Pull remote changes after the user returns to Obsidian (desktop window
   * focus, or Android/iOS coming out of the background). A live WebSocket is
   * not enough: Electron and mobile WebViews often keep a stale OPEN socket
   * that delivered no file_changed events while the app was asleep.
   */
  async onForeground() {
    const now = Date.now();
    if (now - this.lastResumeSync < RESUME_SYNC_MIN_MS) return;
    this.lastResumeSync = now;
    this.normalizeCredentials();
    if (!this.settings.token || !this.settings.serverUrl) return;
    if (this.mobileUrlError()) return;
    if (!this.connected) {
      if (!this.booting) void this.startup();
      return;
    }
    this.reconnectSocketIfNeeded();
    if (!this.syncing && !this.booting) void this.pollRemote();
  }

  reconnectSocketIfNeeded() {
    const ws = this.ws;
    if (ws && ws.readyState === WebSocket.OPEN) return;
    if (ws) {
      try {
        ws.close();
      } catch {
        /* ignore */
      }
      this.ws = null;
    }
    this.wsConnecting = false;
    void this.openSocket();
  }

  /** HTTP fallback when the mobile WebView blocks WebSockets (cleartext, ATS, captive Wi-Fi). */
  async pollRemote() {
    try {
      const man = await this.api("/api/v1/sync/manifest");
      const remote = new Map<string, string>();
      for (const f of man.files || []) remote.set(f.path, f.hash);
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
      if (need) await this.fullSync();
    } catch (e) {
      console.error(e);
    }
  }

  async loadSettings() {
    this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());
    if (!this.settings.hashes) this.settings.hashes = {};
    this.normalizeCredentials();
    if (isMobileApp() && isLoopbackUrl(this.settings.serverUrl)) {
      this.settings.serverUrl = "";
    }
  }

  async saveSettings() {
    this.normalizeCredentials();
    await this.saveData(this.settings);
  }
}

class ConflictModal extends Modal {
  private resolved = false;

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
    if (isMobileApp()) contentEl.addClass("syncidian-modal-mobile");
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
    this.resolved = true;
    this.plugin.onConflictResolved();
    this.close();
  }

  onClose() {
    this.contentEl.empty();
    if (!this.resolved) this.plugin.onConflictModalClosed(this.id);
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
    containerEl.createEl("p", {
      text: "Install from Community plugins (or BRAT). Point this vault at your Syncidian server. GitHub is configured per user in the dashboard, not here. Unlike Git plugins, this one uses only the Obsidian API and works on Android and iOS.",
    });
    if (isMobileApp()) {
      new Setting(containerEl)
        .setName("Mobile")
        .setDesc(
          "Use a public HTTPS server URL. localhost is this phone or tablet, not your computer. iOS often blocks plain http://."
        );
    }
    let urlInput: HTMLInputElement | null = null;
    let tokenInput: HTMLInputElement | null = null;
    let nameInput: HTMLInputElement | null = null;
    new Setting(containerEl)
      .setName("Server URL")
      .setDesc(
        isMobileApp()
          ? "HTTPS address of your Syncidian server, for example https://sync.example.com"
          : "Example: http://localhost:8080 or https://sync.example.com"
      )
      .addText((t) => {
        urlInput = t.inputEl;
        t.setPlaceholder(isMobileApp() ? "https://sync.example.com" : "http://localhost:8080")
          .setValue(this.plugin.settings.serverUrl)
          .onChange(async (v) => {
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
          const mobileErr = this.plugin.mobileUrlError();
          if (mobileErr) {
            new Notice(`Syncidian: ${mobileErr}`);
            return;
          }
          const ok = await this.plugin.startup();
          if (ok) new Notice("Syncidian connected");
        })
      );
  }
}
