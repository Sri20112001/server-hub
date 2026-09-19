import { useEffect, useMemo, useState } from "react";
import { Activity } from "lucide-react";
import { api } from "../../lib/api";
import type {
  ContainerStat,
  ServerDetail,
  StorageInfo,
  TelemetryPoint,
} from "../../lib/types";
import { fmtRate, fmtUptime } from "../../lib/format";
import { Kicker, Modal } from "../../components/ui";

const RANGES = ["15m", "1h", "6h", "24h"] as const;

type RichPoint = TelemetryPoint & { rx: number; tx: number; rd: number; wr: number };

function fmtTime(ts: number): string {
  const d = new Date(ts * 1000);
  return d.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
}

function fmtGB(gb: number): string {
  if (gb == null || gb < 0) return "—";
  if (gb < 1) return `${Math.round(gb * 1024)} MB`;
  return `${gb.toFixed(1)} GB`;
}

function fmtBytesTotal(b: number): string {
  if (!b || b < 0) return "0 B";
  if (b < 1024) return `${Math.round(b)} B`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`;
  return `${(b / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

function fmtSize(mb: number): string {
  if (mb == null || mb < 0) return "—";

  if (mb < 1) {
    return `${Math.round(mb * 1024)} KB`;
  }

  if (mb < 1024) {
    return `${mb.toFixed(0)} MB`;
  }

  const gb = mb / 1024;

  if (gb < 1024) {
    return `${gb.toFixed(1)} GB`;
  }

  return `${(gb / 1024).toFixed(1)} TB`;
}

function Chart({
  title,
  unit,
  points,
  value,
  value2,
  max,
  stroke,
  fill,
  stroke2,
  fill2,
  label2,
  format,
}: {
  title: string;
  unit: string;
  points: RichPoint[];
  value: (p: RichPoint) => number;
  value2?: (p: RichPoint) => number;
  max: (vals: number[]) => number;
  stroke: string;
  fill: string;
  stroke2?: string;
  fill2?: string;
  label2?: string;
  format: (v: number) => string;
}) {
  const W = 560;
  const H = 140;
  const vals = points.map(value);
  const vals2 = value2 ? points.map(value2) : null;
  const hi = Math.max(max(vals), ...vals, ...(vals2 ?? []), 1);
  const step = points.length > 1 ? W / (points.length - 1) : 0;
  const toXY = (vs: number[]) =>
    vs
      .map((v, i) => {
        const x = Math.round(i * step);
        const y = Math.round(H - 12 - (Math.min(v, hi) / hi) * (H - 28));
        return `${x},${y}`;
      })
      .join(" ");
  const line = toXY(vals);
  const line2 = vals2 ? toXY(vals2) : null;
  const last = vals.length ? vals[vals.length - 1] : 0;
  const last2 = vals2 && vals2.length ? vals2[vals2.length - 1] : 0;
  return (
    <div>
      <div className="flex items-center justify-between gap-4 mb-1.5">
        <Kicker>{title}</Kicker>
        <span className="font-mono text-[13px]">
          {format(last)}{" "}
          {label2 && vals2 ? (
            <span className="text-muted dark:text-fog text-[11px]">
              / {format(last2)} {label2}
            </span>
          ) : (
            <span className="text-muted dark:text-fog text-[11px]">{unit}</span>
          )}
        </span>
      </div>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        className="w-full h-36 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl"
        role="img"
        aria-label={`${title} history chart`}
      >
        {[0.25, 0.5, 0.75].map((f) => (
          <line
            key={f}
            x1="0"
            x2={W}
            y1={H * f}
            y2={H * f}
            className="stroke-line dark:stroke-edge"
            strokeWidth="1"
          />
        ))}
        <polygon points={`0,${H} ${line} ${W},${H}`} fill={fill} opacity="0.25" />
        {line2 && stroke2 && fill2 && (
          <polygon points={`0,${H} ${line2} ${W},${H}`} fill={fill2} opacity="0.2" />
        )}
        <polyline points={line} fill="none" stroke={stroke} strokeWidth="2" strokeLinejoin="round" />
        {line2 && stroke2 && (
          <polyline points={line2} fill="none" stroke={stroke2} strokeWidth="2" strokeLinejoin="round" />
        )}
      </svg>
      <div className="flex items-center justify-between gap-4 mt-1">
        <span className="font-mono text-[11px] text-muted dark:text-fog">
          {points.length ? fmtTime(points[0].ts) : "—"}
        </span>
        <span className="font-mono text-[11px] text-muted dark:text-fog">
          max {format(hi)}{points.length ? ` · ${fmtTime(points[points.length - 1].ts)}` : ""}
        </span>
      </div>
    </div>
  );
}

function rates(points: TelemetryPoint[], key: "netRx" | "netTx" | "diskRead" | "diskWrite"): number[] {
  return points.map((p, i) => {
    if (i === 0) return 0;
    const dt = Math.max(1, p.ts - points[i - 1].ts);
    const dv = Math.max(0, p[key] - points[i - 1][key]);
    return dv / dt / 1024;
  });
}

function Fact({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5">
      <Kicker>{label}</Kicker>
      <div className={`text-[13px] mt-1.5 truncate ${mono === false ? "" : "font-mono"}`}>
        {value}
      </div>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mt-6">
      <div className="mb-2.5">
        <Kicker>{title}</Kicker>
      </div>
      {children}
    </div>
  );
}

function pressureDot(level: string): string {
  if (level === "High") return "bg-brick";
  if (level === "Elevated") return "bg-status-amber";
  return "bg-moss";
}

export function TelemetryModal({ onClose }: { onClose: () => void }) {
  const [range, setRange] = useState<(typeof RANGES)[number]>("1h");
  const [points, setPoints] = useState<TelemetryPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [detail, setDetail] = useState<ServerDetail | null>(null);
  const [storage, setStorage] = useState<StorageInfo | null>(null);
  const [cstats, setCstats] = useState<ContainerStat[] | null>(null);

  useEffect(() => {
    let alive = true;
    setLoading(true);
    api
      .telemetry(range)
      .then((r) => {
        if (alive) setPoints(r.points);
      })
      .catch(() => {
        if (alive) setPoints([]);
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [range]);

  useEffect(() => {
    let alive = true;
    api
      .serverDetail()
      .then((d) => {
        if (alive) setDetail(d);
      })
      .catch(() => {});
    api
      .storage()
      .then((s) => {
        if (alive) setStorage(s);
      })
      .catch(() => {});
    api
      .containerStats()
      .then((c) => {
        if (alive) setCstats(c);
      })
      .catch(() => {
        if (alive) setCstats([]);
      });
    return () => {
      alive = false;
    };
  }, []);

  const rich: RichPoint[] = useMemo(() => {
    const rx = rates(points, "netRx");
    const tx = rates(points, "netTx");
    const rd = rates(points, "diskRead");
    const wr = rates(points, "diskWrite");
    return points.map((p, i) => ({
      ...p,
      rx: rx[i] ?? 0,
      tx: tx[i] ?? 0,
      rd: rd[i] ?? 0,
      wr: wr[i] ?? 0,
    }));
  }, [points]);

  return (
    <Modal onClose={onClose}>
      <div className="flex items-center gap-2 flex-wrap mb-1">
        <Activity size={16} />
        <h3 className="text-[18px]">Host telemetry</h3>
      </div>
      <p className="text-muted dark:text-fog text-[13px] mb-4">
        One sample per minute, kept for 7 days. Ranges switch the charts.
      </p>
      <div className="grid grid-cols-4 gap-1 p-1 rounded-xl bg-paper dark:bg-abyss border border-line dark:border-edge mb-4">
        {RANGES.map((r) => (
          <button
            key={r}
            type="button"
            onClick={() => setRange(r)}
            aria-pressed={range === r}
            className={`px-3 py-2 rounded-lg font-mono text-[12px] cursor-pointer border transition-colors duration-150 ${
              range === r
                ? "bg-white dark:bg-panel text-ink dark:text-bone border-line dark:border-edge shadow-sm"
                : "bg-transparent text-muted dark:text-fog border-transparent hover:text-ink dark:hover:text-bone"
            }`}
          >
            {r}
          </button>
        ))}
      </div>

      <div className="flex flex-col gap-5 max-h-[62vh] overflow-y-auto pr-1">
        {loading ? (
          <div className="text-muted dark:text-fog text-[13px]">Tuning the dials…</div>
        ) : points.length === 0 ? (
          <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-8 text-center">
            <div className="font-head font-semibold mb-1">No readings yet</div>
            <div className="text-muted dark:text-fog text-[13px]">
              The sampler records every minute — check back shortly.
            </div>
          </div>
        ) : (
          <>
            <Chart
              title="CPU"
              unit="load"
              points={rich}
              value={(p) => p.cpu}
              max={(v) => Math.max(100, ...v)}
              stroke="#EA580C"
              fill="#EA580C"
              format={(v) => `${v.toFixed(1)}%`}
            />
            <Chart
              title="Memory"
              unit="used"
              points={rich}
              value={(p) => p.memPct}
              max={(v) => Math.max(100, ...v)}
              stroke="#16A34A"
              fill="#16A34A"
              format={(v) => `${v.toFixed(1)}%`}
            />
            <Chart
              title="Disk"
              unit="used"
              points={rich}
              value={(p) => p.diskPct}
              max={(v) => Math.max(100, ...v)}
              stroke="#D97706"
              fill="#D97706"
              format={(v) => `${v.toFixed(1)}%`}
            />
            <Chart
              title="Network"
              unit=""
              points={rich}
              value={(p) => p.rx}
              value2={(p) => p.tx}
              max={(v) => Math.max(...v, 1)}
              stroke="#0E7490"
              fill="#0E7490"
              stroke2="#7C3AED"
              fill2="#7C3AED"
              label2="out"
              format={fmtRate}
            />
            <Chart
              title="Disk I/O"
              unit=""
              points={rich}
              value={(p) => p.rd}
              value2={(p) => p.wr}
              max={(v) => Math.max(...v, 1)}
              stroke="#2563EB"
              fill="#2563EB"
              stroke2="#DB2777"
              fill2="#DB2777"
              label2="write"
              format={fmtRate}
            />
          </>
        )}

        {detail && (
          <>
            <Section title="System pressure">
              <div className="grid grid-cols-3 gap-4">
                {[
                  ["CPU", detail.pressure.cpu],
                  ["Memory", detail.pressure.memory],
                  ["Disk", detail.pressure.disk],
                ].map(([label, level]) => (
                  <div
                    key={label}
                    className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5 flex items-center gap-2.5"
                  >
                    <span className={`w-2 h-2 rounded-full ${pressureDot(level)}`} />
                    <div>
                      <Kicker>{label}</Kicker>
                      <div className="text-[13px] font-medium">{level}</div>
                    </div>
                  </div>
                ))}
              </div>
            </Section>

            <Section title="Host">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <Fact label="Hostname" value={detail.host.hostname || "—"} />
                <Fact
                  label="OS"
                  value={`${detail.host.platform || detail.host.os || "—"} · ${detail.host.arch || ""}`}
                />
                <Fact label="Kernel" value={detail.host.kernel || "—"} />
                <Fact label="Uptime" value={fmtUptime(detail.host.uptimeSec)} />
                <Fact
                  label="CPU"
                  value={`${detail.cpu.logicalCores || "?"} cores · ${detail.cpu.physicalCores || "?"} physical · ${(detail.cpu.mhz / 1000 || 0).toFixed(1)} GHz`}
                />
                <Fact
                  label="Load average"
                  value={`${detail.cpu.load1.toFixed(2)} / ${detail.cpu.load5.toFixed(2)} / ${detail.cpu.load15.toFixed(2)}`}
                />
              </div>
              <div className="font-mono text-[11px] text-muted dark:text-fog mt-2 truncate">
                {detail.cpu.model || "unknown processor"}
              </div>
            </Section>

            <Section title="Memory">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <Fact
                  label="Used"
                  value={`${fmtSize(detail.memory.usedMB)} / ${fmtSize(detail.memory.totalMB)} · ${detail.memory.usedPct.toFixed(1)}%`}
                />
                <Fact label="Available" value={fmtSize(detail.memory.availableMB)} />
                <Fact label="Free" value={fmtSize(detail.memory.freeMB)} />
                <Fact
                  label="Cached + buffers"
                  value={`${fmtSize(detail.memory.cachedMB + detail.memory.buffersMB)}`}
                />
                <Fact
                  label="Swap"
                  value={
                    detail.memory.swapTotalMB > 0
                      ? `${fmtSize(detail.memory.swapUsedMB)} / ${fmtSize(detail.memory.swapTotalMB)} · ${detail.memory.swapPct.toFixed(0)}%`
                      : "no swap configured"
                  }
                />
                <Fact label="Total" value={fmtSize(detail.memory.totalMB)} />
              </div>
            </Section>

            <Section title="Processes">
              <div className="flex items-center gap-2 flex-wrap mb-2.5 font-mono text-[12px]">
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                  {detail.processes.total} total
                </span>
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                  {detail.processes.running} running
                </span>
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                  {detail.processes.sleeping} sleeping
                </span>
                {detail.processes.zombie > 0 && (
                  <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">
                    {detail.processes.zombie} zombie
                  </span>
                )}
              </div>
              <div className="overflow-x-auto bg-white dark:bg-panel border border-line dark:border-edge rounded-xl">
                <table className="w-full border-collapse text-[13px]">
                  <thead>
                    <tr>
                      <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Process</th>
                      <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">PID</th>
                      <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">State</th>
                      <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss last:text-right">Memory</th>
                    </tr>
                  </thead>
                  <tbody>
                    {detail.processes.top.map((p) => (
                      <tr key={p.pid} className="group">
                        <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle font-mono max-w-[220px] truncate">
                          {p.name}
                        </td>
                        <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle font-mono text-muted dark:text-fog">
                          {p.pid}
                        </td>
                        <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle font-mono text-muted dark:text-fog">
                          {p.status}
                        </td>
                        <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss font-mono">
                          {fmtSize(p.memMB)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Section>
          </>
        )}

        {storage && (
          <Section title="Storage">
            <div className="overflow-x-auto bg-white dark:bg-panel border border-line dark:border-edge rounded-xl mb-4">
              <table className="w-full border-collapse text-[13px]">
                <thead>
                  <tr>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Filesystem</th>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Used</th>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss last:text-right">Total</th>
                  </tr>
                </thead>
                <tbody>
                  {storage.filesystems.map((f) => (
                    <tr key={f.mount} className="group">
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle">
                        <div className="font-mono">{f.mount}</div>
                        <div className="font-mono text-muted dark:text-fog text-[11px]">
                          {f.device} · {f.fstype}
                        </div>
                      </td>
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle font-mono">
                        {fmtGB(f.usedGB)} · {f.usedPct.toFixed(0)}%
                      </td>
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss font-mono">
                        {fmtGB(f.totalGB)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {storage.apps.length > 0 && (
              <div className="mb-4">
                <div className="font-mono text-[11px] text-muted dark:text-fog mb-1.5">
                  {storage.appsRoot || "applications"}
                  {storage.appsPartial ? " · partial scan" : ""}
                </div>
                <div className="flex flex-col gap-1.5">
                  {(() => {
                    const top = Math.max(...storage.apps.map((a) => a.bytesMB), 1);
                    return storage.apps.slice(0, 10).map((a) => {
                      const filled = Math.max(
                        a.bytesMB > 0 ? 1 : 0,
                        Math.round((a.bytesMB / top) * 10),
                      );
                      return (
                        <div key={a.path} className="flex items-center gap-2.5">
                          <span className="font-mono text-[12px] w-32 truncate shrink-0">{a.name}</span>
                          <span className="flex gap-[3px] flex-1" aria-hidden="true">
                            {Array.from({ length: 10 }, (_, i) => (
                              <span
                                key={i}
                                className={`h-1.5 flex-1 rounded-full ${
                                  i < filled
                                    ? "bg-accent dark:bg-ember"
                                    : "bg-paper dark:bg-abyss border border-line dark:border-edge"
                                }`}
                              />
                            ))}
                          </span>
                          <span className="font-mono text-[11px] text-muted dark:text-fog w-20 text-right shrink-0">
                            {fmtSize(a.bytesMB)}
                          </span>
                        </div>
                      );
                    });
                  })()}
                </div>
              </div>
            )}

            {storage.docker && (
              <div>
                <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 mb-2.5">
                  <Fact label="Images" value={fmtSize(storage.docker.imagesMB)} />
                  <Fact label="Containers" value={fmtSize(storage.docker.containersMB)} />
                  <Fact label="Volumes" value={fmtSize(storage.docker.volumesMB)} />
                  <Fact label="Build cache" value={fmtSize(storage.docker.buildCacheMB)} />
                </div>
                {storage.docker.imagesTop.length > 0 && (
                  <div className="flex flex-col gap-1.5">
                    {storage.docker.imagesTop.slice(0, 6).map((im) => (
                      <div key={im.name} className="flex items-center gap-2.5">
                        <span className="font-mono text-[12px] flex-1 truncate">{im.name}</span>
                        <span className="font-mono text-[11px] text-muted dark:text-fog shrink-0">
                          {fmtSize(im.sizeMB)}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </Section>
        )}

        {detail && detail.network.length > 0 && (
          <Section title="Network interfaces">
            <div className="overflow-x-auto bg-white dark:bg-panel border border-line dark:border-edge rounded-xl">
              <table className="w-full border-collapse text-[13px]">
                <thead>
                  <tr>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Interface</th>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Received</th>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss last:text-right">Transmitted</th>
                  </tr>
                </thead>
                <tbody>
                  {detail.network.map((n) => (
                    <tr key={n.name} className="group">
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle font-mono">
                        {n.name}
                      </td>
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle font-mono">
                        {fmtBytesTotal(n.rxBytes)}
                      </td>
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss font-mono">
                        {fmtBytesTotal(n.txBytes)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Section>
        )}

        {detail && detail.diskIO.length > 0 && (
          <Section title="Disk I/O totals">
            <div className="overflow-x-auto bg-white dark:bg-panel border border-line dark:border-edge rounded-xl">
              <table className="w-full border-collapse text-[13px]">
                <thead>
                  <tr>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Device</th>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Read</th>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss last:text-right">Written</th>
                  </tr>
                </thead>
                <tbody>
                  {detail.diskIO.map((d) => (
                    <tr key={d.device} className="group">
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle font-mono">
                        {d.device}
                      </td>
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle font-mono">
                        {fmtSize(d.readMB)}
                      </td>
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss font-mono">
                        {fmtSize(d.writeMB)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Section>
        )}

        {cstats !== null && cstats.length > 0 && (
          <Section title="Container resources">
            <div className="overflow-x-auto bg-white dark:bg-panel border border-line dark:border-edge rounded-xl">
              <table className="w-full border-collapse text-[13px]">
                <thead>
                  <tr>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Container</th>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">CPU</th>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss">Memory</th>
                    <th className="text-left text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog px-4 py-2.5 bg-paper dark:bg-abyss last:text-right">Net ↓↑</th>
                  </tr>
                </thead>
                <tbody>
                  {cstats.map((c) => (
                    <tr key={c.id} className="group">
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle">
                        <div className="font-mono">{c.name}</div>
                        <div className="font-mono text-muted dark:text-fog text-[11px] truncate max-w-[220px]">
                          {c.image}
                        </div>
                      </td>
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle font-mono">
                        {c.cpuPercent.toFixed(1)}%
                      </td>
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle font-mono">
                        {fmtSize(c.memMB)}
                        {c.memLimitMB > 0 ? ` / ${fmtSize(c.memLimitMB)}` : ""}
                      </td>
                      <td className="px-4 py-2.5 border-t border-line dark:border-edge align-middle last:text-right group-hover:bg-paper dark:group-hover:bg-emboss font-mono">
                        {fmtSize(c.netRxMB)} / {fmtSize(c.netTxMB)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Section>
        )}
      </div>

      <div className="flex items-center gap-2 flex-wrap justify-end mt-4">
        <button
          className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss"
          onClick={onClose}
        >
          Close
        </button>
      </div>
    </Modal>
  );
}
