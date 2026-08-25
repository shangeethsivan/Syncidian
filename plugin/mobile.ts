import { Platform } from "obsidian";

export function isMobileApp(): boolean {
  return Platform.isMobile || Platform.isAndroidApp || Platform.isIosApp;
}

export function devicePlatform(): string {
  if (Platform.isAndroidApp) return "Android";
  if (Platform.isIosApp) return "iOS";
  if (Platform.isMacOS) return "macOS";
  if (Platform.isWin) return "Windows";
  if (Platform.isLinux) return "Linux";
  return "unknown";
}

export function guessDeviceName(): string {
  if (Platform.isAndroidApp) return Platform.isTablet ? "Android tablet" : "Android";
  if (Platform.isIosApp) return Platform.isTablet ? "iPad" : "iPhone";
  if (Platform.isMacOS) return "macOS";
  if (Platform.isWin) return "Windows";
  if (Platform.isLinux) return "Linux";
  return "Obsidian";
}

export function isLoopbackUrl(url: string): boolean {
  try {
    const host = new URL(url).hostname.toLowerCase();
    return host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "[::1]" || host === "0.0.0.0";
  } catch {
    return /localhost|127\.0\.0\.1/i.test(url);
  }
}

export function isInsecureHttp(url: string): boolean {
  try {
    return new URL(url).protocol === "http:";
  } catch {
    return url.startsWith("http://");
  }
}

export function pushBatchSize(): number {
  return isMobileApp() ? 6 : 40;
}
