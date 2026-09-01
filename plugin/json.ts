/** Narrow JSON and API payloads without using `any`. */

export const HOSTED_SERVER_URL = "https://syncidian.com";
const LEGACY_DEFAULT_SERVER_URL = "http://localhost:8080";

export type PluginSettings = {
  serverUrl: string;
  useCustomDomain: boolean;
  customServerUrl: string;
  token: string;
  deviceName: string;
  deviceId: string;
  hashes: Record<string, string>;
};

export function parseJson(text: string): unknown {
  return JSON.parse(text) as unknown;
}

export function isUnknownArray(value: unknown): value is unknown[] {
  return Array.isArray(value);
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function stringField(value: unknown, key: string): string {
  if (!isRecord(value)) return "";
  const field = value[key];
  return typeof field === "string" ? field : "";
}

export function boolField(value: unknown, key: string): boolean {
  if (!isRecord(value)) return false;
  return value[key] === true;
}

export function stringArrayField(value: unknown, key: string): string[] {
  if (!isRecord(value)) return [];
  const field = value[key];
  if (!Array.isArray(field)) return [];
  return field.filter((item): item is string => typeof item === "string");
}

export function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return String(err);
}

export function normalizeServerUrl(url: string): string {
  return (url || "").trim().replace(/\/$/, "");
}

export function isHostedServerUrl(url: string): boolean {
  const raw = normalizeServerUrl(url);
  if (!raw) return false;
  try {
    const host = new URL(raw.includes("://") ? raw : `https://${raw}`).hostname.toLowerCase();
    return host === "syncidian.com" || host === "www.syncidian.com";
  } catch {
    const n = raw.toLowerCase();
    return n === HOSTED_SERVER_URL || n === "https://www.syncidian.com";
  }
}

function isLegacyDefaultServerUrl(url: string): boolean {
  return normalizeServerUrl(url).toLowerCase() === LEGACY_DEFAULT_SERVER_URL;
}

/** Older installs stored only serverUrl. Infer Custom Domain from that. */
export function inferUseCustomDomain(serverUrl: string, token: string): boolean {
  const url = normalizeServerUrl(serverUrl);
  if (!url || isHostedServerUrl(url)) return false;
  if (isLegacyDefaultServerUrl(url) && !token) return false;
  return true;
}

export function mergeSettings(raw: unknown, defaults: PluginSettings): PluginSettings {
  const next: PluginSettings = {
    serverUrl: defaults.serverUrl,
    useCustomDomain: defaults.useCustomDomain,
    customServerUrl: defaults.customServerUrl,
    token: defaults.token,
    deviceName: defaults.deviceName,
    deviceId: defaults.deviceId,
    hashes: { ...defaults.hashes },
  };
  if (!isRecord(raw)) return applyServerMode(next);
  const serverUrl = stringField(raw, "serverUrl");
  const customServerUrl = stringField(raw, "customServerUrl");
  const token = stringField(raw, "token");
  const deviceName = stringField(raw, "deviceName");
  const deviceId = stringField(raw, "deviceId");
  if ("serverUrl" in raw) next.serverUrl = serverUrl;
  if ("customServerUrl" in raw) next.customServerUrl = customServerUrl;
  if ("token" in raw) next.token = token;
  if ("deviceName" in raw) next.deviceName = deviceName;
  if ("deviceId" in raw) next.deviceId = deviceId;
  if ("useCustomDomain" in raw) {
    next.useCustomDomain = raw.useCustomDomain === true;
  } else {
    next.useCustomDomain = inferUseCustomDomain(next.serverUrl, next.token);
  }
  if (!next.customServerUrl && next.useCustomDomain) {
    next.customServerUrl = next.serverUrl;
  }
  if (isRecord(raw.hashes)) {
    const hashes: Record<string, string> = {};
    for (const path of Object.keys(raw.hashes)) {
      const hash = raw.hashes[path];
      if (typeof hash === "string") hashes[path] = hash;
    }
    next.hashes = hashes;
  }
  return applyServerMode(next);
}

export function applyServerMode(settings: PluginSettings): PluginSettings {
  settings.customServerUrl = normalizeServerUrl(settings.customServerUrl);
  if (settings.useCustomDomain) {
    settings.serverUrl = normalizeServerUrl(settings.customServerUrl || settings.serverUrl);
    settings.customServerUrl = settings.serverUrl;
  } else {
    settings.serverUrl = HOSTED_SERVER_URL;
  }
  return settings;
}
