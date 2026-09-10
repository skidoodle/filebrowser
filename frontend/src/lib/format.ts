const BYTE_UNITS = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"] as const;

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "–";
  let value = n;
  let unit = 0;
  while (value >= 1024 && unit < BYTE_UNITS.length - 1) {
    value /= 1024;
    unit++;
  }
  const digits = unit === 0 || value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(digits)} ${BYTE_UNITS[unit]}`;
}

export function parseHumanBytes(str: string, fallback: number): number {
  const s = str.trim().toLowerCase();
  if (!s) return fallback;
  const units: [string, number][] = [
    ["tib", 1024 ** 4],
    ["tb", 1024 ** 4],
    ["gib", 1024 ** 3],
    ["gb", 1024 ** 3],
    ["mib", 1024 ** 2],
    ["mb", 1024 ** 2],
    ["kib", 1024],
    ["kb", 1024],
    ["b", 1],
  ];
  for (const [suffix, mult] of units) {
    if (s.endsWith(suffix)) {
      const val = parseFloat(s.slice(0, -suffix.length).trim());
      if (!Number.isNaN(val) && val > 0) {
        return Math.round(val * mult);
      }
    }
  }
  const val = parseFloat(s);
  return !Number.isNaN(val) && val > 0 ? Math.round(val) : fallback;
}

export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "0s";
  if (seconds < 60) return `${Math.floor(seconds)}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${Math.floor(seconds % 60)}s`;
  const hours = Math.floor(minutes / 60);
  const remMinutes = minutes % 60;
  if (hours < 24) return `${hours}h ${remMinutes}m`;
  const days = Math.floor(hours / 24);
  const remHours = hours % 24;
  return `${days}d ${remHours}h ${remMinutes}m`;
}

const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });

export function formatRelative(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "–";
  const diffSeconds = Math.round((then - Date.now()) / 1000);
  const abs = Math.abs(diffSeconds);
  if (abs < 60) return "just now";
  if (abs < 3600) return rtf.format(Math.round(diffSeconds / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diffSeconds / 3600), "hour");
  if (abs < 86400 * 30) return rtf.format(Math.round(diffSeconds / 86400), "day");
  if (abs < 86400 * 365) return rtf.format(Math.round(diffSeconds / (86400 * 30)), "month");
  return rtf.format(Math.round(diffSeconds / (86400 * 365)), "year");
}
