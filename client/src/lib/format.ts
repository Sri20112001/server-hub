/** Parse SQLite `YYYY-MM-DD HH:MM:SS` (UTC) or RFC3339 into a Date. */
export function parseTime(s: string | undefined): Date | null {
  if (!s) return null;
  const iso = s.includes("T") ? s : s.replace(" ", "T") + "Z";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? null : d;
}

export function timeAgo(s: string | undefined): string {
  const d = parseTime(s);
  if (!d) return "—";
  const secs = Math.max(0, Math.floor((Date.now() - d.getTime()) / 1000));
  if (secs < 60) return `${secs}s ago`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins} min${mins === 1 ? "" : "s"} ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs} hour${hrs === 1 ? "" : "s"} ago`;
  const days = Math.floor(hrs / 24);
  return `${days} day${days === 1 ? "" : "s"} ago`;
}

export function fmtDuration(sec: number | undefined): string {
  if (sec === undefined || sec === null) return "—";
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  return `${m}m ${String(sec % 60).padStart(2, "0")}s`;
}

export function fmtUptime(totalSec: number): string {
  const d = Math.floor(totalSec / 86400);
  const h = Math.floor((totalSec % 86400) / 3600);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${Math.floor((totalSec % 3600) / 60)}m`;
  return `${Math.floor(totalSec / 60)}m`;
}

export function greeting(): string {
  const h = new Date().getHours();
  if (h < 12) return "Good morning";
  if (h < 18) return "Good afternoon";
  return "Good evening";
}

export function todayLong(): string {
  return new Date().toLocaleDateString("en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
  });
}

export function shortSha(sha: string | undefined): string {
  if (!sha) return "—";
  return sha.length > 8 ? sha.slice(0, 7) : sha;
}

export function shortId(id: string | undefined): string {
  if (!id) return "—";
  return id.length > 12 ? id.slice(0, 12) : id;
}

export function fmtBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return "0 B";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function fmtRate(kbs: number): string {
  if (!kbs || kbs < 0) return "0 B/s";
  if (kbs < 1) return `${Math.round(kbs * 1024)} B/s`;
  if (kbs < 1024) return `${kbs.toFixed(1)} KB/s`;
  return `${(kbs / 1024).toFixed(2)} MB/s`;
}

export function fmtGBFromMB(mb: number): string {
  if (mb == null || mb < 0) return "—";
  if (mb < 1024) return `${mb.toFixed(0)} MB`;
  return `${(mb / 1024).toFixed(1)} GB`;
}
