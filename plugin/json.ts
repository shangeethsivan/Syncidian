/** Narrow JSON and API payloads without using `any`. */

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

export function mergeSettings(
  raw: unknown,
  defaults: {
    serverUrl: string;
    token: string;
    deviceName: string;
    deviceId: string;
    hashes: Record<string, string>;
  }
): {
  serverUrl: string;
  token: string;
  deviceName: string;
  deviceId: string;
  hashes: Record<string, string>;
} {
  const next = {
    serverUrl: defaults.serverUrl,
    token: defaults.token,
    deviceName: defaults.deviceName,
    deviceId: defaults.deviceId,
    hashes: { ...defaults.hashes },
  };
  if (!isRecord(raw)) return next;
  const serverUrl = stringField(raw, "serverUrl");
  const token = stringField(raw, "token");
  const deviceName = stringField(raw, "deviceName");
  const deviceId = stringField(raw, "deviceId");
  if ("serverUrl" in raw) next.serverUrl = serverUrl;
  if ("token" in raw) next.token = token;
  if ("deviceName" in raw) next.deviceName = deviceName;
  if ("deviceId" in raw) next.deviceId = deviceId;
  if (isRecord(raw.hashes)) {
    const hashes: Record<string, string> = {};
    for (const path of Object.keys(raw.hashes)) {
      const hash = raw.hashes[path];
      if (typeof hash === "string") hashes[path] = hash;
    }
    next.hashes = hashes;
  }
  return next;
}
